/*
 * Copyright (c) 2013 Matt Jibson <matt.jibson@gmail.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package goapp

import (
	"appengine/datastore"
	"appengine/taskqueue"
	"appengine/urlfetch"
	"appengine/user"
	"code.google.com/p/goauth2/oauth"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/mjibson/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var router = new(mux.Router)
var templates *template.Template

func init() {
	var err error

	if templates, err = template.New("").Funcs(funcs).
		ParseFiles(
		"templates/base.html",
	); err != nil {
		log.Fatal(err)
	}

	router.Handle("/", mpg.NewHandler(Main)).Name("main")
	router.Handle("/login/google", mpg.NewHandler(LoginGoogle)).Name("login-google")
	router.Handle("/logout", mpg.NewHandler(Logout)).Name("logout")
	router.Handle("/oauth2callback", mpg.NewHandler(Oauth2Callback)).Name("oauth2callback")
	router.Handle("/tasks/import-reader", mpg.NewHandler(ImportReaderTask)).Name("import-reader-task")
	router.Handle("/tasks/import-opml", mpg.NewHandler(ImportOpmlTask)).Name("import-opml-task")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	router.Handle("/user/get-contents", mpg.NewHandler(GetContents)).Name("get-contents")
	router.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	router.Handle("/user/import/reader", mpg.NewHandler(ImportReader)).Name("import-reader")
	router.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	router.Handle("/user/mark-all-read", mpg.NewHandler(MarkAllRead)).Name("mark-all-read")
	router.Handle("/user/mark-read", mpg.NewHandler(MarkRead)).Name("mark-read")
	http.Handle("/", router)

	miniprofiler.ShowControls = true
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "base.html", includes(c)); err != nil {
		serveError(w, err)
	}
}

func LoginGoogle(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if u := user.Current(c); u != nil {
		gn := goon.FromContext(c)
		user := User{}
		if ue, err := gn.GetById(&user, u.ID, 0, nil); err == nil && ue.NotFound {
			user.Email = u.Email
			user.Read = time.Now()
			gn.Put(ue)
		}
	}

	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func Logout(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if u, err := user.LogoutURL(c, routeUrl("main")); err == nil {
		http.Redirect(w, r, u, http.StatusFound)
	} else {
		http.Redirect(w, r, routeUrl("main"), http.StatusFound)
	}
}

func ImportOpml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{}
	ue, _ := gn.GetById(&u, cu.ID, 0, nil)
	if ue.NotFound {
		return
	}

	if file, _, err := r.FormFile("file"); err == nil {
		if fdata, err := ioutil.ReadAll(file); err == nil {
			task := taskqueue.NewPOSTTask(routeUrl("import-opml-task"), url.Values{
				"data": {string(fdata)},
				"user": {cu.ID},
			})
			taskqueue.Add(c, task, "import-reader")
		}
	}
}

func ImportOpmlTask(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	type outline struct {
		Outline []outline `xml:"outline"`
		Title   string    `xml:"title,attr"`
		XmlUrl  string    `xml:"xmlUrl,attr"`
	}

	type Body struct {
		Outline []outline `xml:"outline"`
	}

	gn := goon.FromContext(c)
	userid := r.FormValue("user")
	data := r.FormValue("data")

	var skip int
	if s, err := strconv.Atoi(r.FormValue("skip")); err == nil {
		skip = s
	}
	c.Debugf("reader import for %v, skip %v", userid, skip)

	var ufs []*UserFeed
	sortid := 1 + skip
	remaining := skip

	var proc func(label string, outlines []outline)
	proc = func(label string, outlines []outline) {
		for _, o := range outlines {
			if o.XmlUrl != "" {
				if remaining > 0 {
					remaining--
				} else if len(ufs) < IMPORT_LIMIT {
					ufs = append(ufs, &UserFeed{
						Label:  label,
						Url:    o.XmlUrl,
						Title:  o.Title,
						Sortid: strconv.Itoa(sortid * 1000),
					})
					sortid++
				}
			}

			if o.Title != "" && len(o.Outline) > 0 {
				proc(o.Title, o.Outline)
			}
		}
	}

	idx0 := strings.Index(data, "<body>")
	idx1 := strings.LastIndex(data, "</body>")
	data = data[idx0 : idx1+7]
	feed := Body{}
	if err := xml.Unmarshal([]byte(data), &feed); err != nil {
		return
	}
	proc("", feed.Outline)

	// todo: refactor below with similar from ImportReaderTask
	wg := sync.WaitGroup{}
	wg.Add(len(ufs))
	for i := range ufs {
		go func(i int) {
			if err := addFeed(c, userid, ufs[i]); err != nil {
				c.Errorf("opml import error: %v", err.Error())
				// todo: do something here?
			}
			c.Debugf("opml import: %s, %s", ufs[i].Title, ufs[i].Url)
			wg.Done()
		}(i)
	}
	wg.Wait()

	ud := UserData{}
	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		ude, _ := gn.GetById(&ud, "data", 0, datastore.NewKey(c, goon.Kind(&User{}), userid, 0, nil))
		addUserFeed(&ud, ufs...)
		return gn.Put(ude)
	}, nil); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		c.Errorf("ude update error: %v", err.Error())
		return
	}

	if len(ufs) == IMPORT_LIMIT {
		task := taskqueue.NewPOSTTask(routeUrl("import-opml-task"), url.Values{
			"data": {data},
			"user": {userid},
			"skip": {strconv.Itoa(skip + IMPORT_LIMIT)},
		})
		taskqueue.Add(c, task, "import-reader")
	}
}

const RECENT = -time.Hour * 24 * 3

func addFeed(c mpg.Context, userid string, uf *UserFeed) error {
	gn := goon.FromContext(c)
	c.Infof("adding feed %s to user %s", uf.Url, userid)

	f := Feed{}
	fe, err := gn.GetById(&f, uf.Url, 0, nil)
	if err != nil {
		return err
	}
	recentDate := time.Now().Add(RECENT)
	var recent []*datastore.Key
	if fe.NotFound {
		if feed, stories := fetchFeed(c, uf.Url); feed == nil {
			return errors.New(fmt.Sprintf("could not add feed %s", uf.Url))
		} else {
			f = *feed
			f.Updated = time.Time{}
			f.Checked = f.Updated
			f.NextUpdate = f.Updated
			gn.Put(fe)
			if err := updateFeed(c, uf.Url, feed, stories); err != nil {
				return err
			}

			uf.Link = feed.Link
			if uf.Title == "" {
				uf.Title = feed.Title
			}

			for _, s := range stories {
				if recentDate.Before(s.Updated) {
					recent = append(recent, datastore.NewKey(c, goon.Kind(&Story{}), s.Id, 0, fe.Key))
				}
			}
		}
	} else {
		uf.Link = f.Link
		if uf.Title == "" {
			uf.Title = f.Title
		}
		q := datastore.NewQuery(goon.Kind(&Story{})).Ancestor(fe.Key).KeysOnly()
		q = q.Filter("u >=", recentDate)
		es, _ := gn.GetAll(q, nil)
		for _, e := range es {
			recent = append(recent, e.Key)
		}
	}

	return nil
}

func addUserFeed(ud *UserData, ufs ...*UserFeed) {
	var fs Feeds
	json.Unmarshal(ud.Feeds, &fs)
	for _, uf := range ufs {
		found := false
		for _, f := range fs {
			if f.Url == uf.Url {
				found = true
				break
			}
		}
		if !found {
			fs = append(fs, uf)
		}
	}
	ud.Feeds, _ = json.Marshal(&fs)
}

func AddSubscription(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	url := r.FormValue("url")
	uf := &UserFeed{Url: url}
	if err := addFeed(c, cu.ID, uf); err != nil {
		c.Errorf("add sub error (%s): %s", url, err.Error())
		serveError(w, err)
		return
	}

	gn := goon.FromContext(c)
	ud := UserData{}
	ude, _ := gn.GetById(&ud, "data", 0, datastore.NewKey(c, goon.Kind(&User{}), cu.ID, 0, nil))
	addUserFeed(&ud, uf)
	gn.Put(ude)
}

func ImportReader(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, oauth_conf.AuthCodeURL(""), http.StatusFound)
}

func Oauth2Callback(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{}
	ue, _ := gn.GetById(&u, cu.ID, 0, nil)
	if ue.NotFound {
		return
	}
	u.Messages = append(u.Messages,
		"Reader import is happening. It can take a minute.",
		"Refresh at will - you'll continue to see this page until it's done.",
	)
	gn.Put(ue)

	t := &oauth.Transport{
		Config:    oauth_conf,
		Transport: &urlfetch.Transport{Context: c},
	}
	t.Exchange(r.FormValue("code"))
	cl := t.Client()
	resp, err := cl.Get("https://www.google.com/reader/api/0/subscription/list?output=json")
	if err != nil {
		serveError(w, err)
		return
	}
	b, _ := ioutil.ReadAll(resp.Body)
	task := taskqueue.NewPOSTTask(routeUrl("import-reader-task"), url.Values{
		"data": {string(b)},
		"user": {cu.ID},
	})
	taskqueue.Add(c, task, "import-reader")
	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

const IMPORT_LIMIT = 20

func ImportReaderTask(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	userid := r.FormValue("user")
	data := r.FormValue("data")

	var skip int
	if s, err := strconv.Atoi(r.FormValue("skip")); err == nil {
		skip = s
	}

	v := struct {
		Subscriptions []struct {
			Id         string `json:"id"`
			Title      string `json:"title"`
			HtmlUrl    string `json:"htmlUrl"`
			Sortid     string `json:"sortid"`
			Categories []struct {
				Id    string `json:"id"`
				Label string `json:"label"`
			} `json:"categories"`
		} `json:"subscriptions"`
	}{}
	json.Unmarshal([]byte(data), &v)
	c.Debugf("reader import for %v, skip %v, len %v", userid, skip, len(v.Subscriptions))

	end := skip + IMPORT_LIMIT
	if end > len(v.Subscriptions) {
		end = len(v.Subscriptions)
	}

	wg := sync.WaitGroup{}
	wg.Add(end - skip)
	ufs := make([]*UserFeed, end-skip)

	for i := range v.Subscriptions[skip:end] {
		go func(i int) {
			sub := v.Subscriptions[skip+i]
			var label string
			if len(sub.Categories) > 0 {
				label = sub.Categories[0].Label
			}
			uf := &UserFeed{
				Label:  label,
				Url:    sub.Id[5:],
				Title:  sub.Title,
				Sortid: sub.Sortid,
			}
			ufs[i] = uf
			if err := addFeed(c, userid, uf); err != nil {
				c.Errorf("reader import error: %v", err.Error())
				// todo: do something here?
			}
			c.Debugf("reader import: %s, %s", sub.Title, sub.Id)
			wg.Done()
		}(i)
	}
	wg.Wait()

	ud := UserData{}
	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		ude, _ := gn.GetById(&ud, "data", 0, datastore.NewKey(c, goon.Kind(&User{}), userid, 0, nil))
		addUserFeed(&ud, ufs...)
		return gn.Put(ude)
	}, nil); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		c.Errorf("ude update error: %v", err.Error())
		return
	}

	if end < len(v.Subscriptions) {
		task := taskqueue.NewPOSTTask(routeUrl("import-reader-task"), url.Values{
			"data": {data},
			"user": {userid},
			"skip": {strconv.Itoa(skip + IMPORT_LIMIT)},
		})
		taskqueue.Add(c, task, "import-reader")
	}
}

func UpdateFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(goon.Kind(&Feed{})).KeysOnly()
	q = q.Filter("n <=", time.Now())
	es, _ := gn.GetAll(q, nil)
	wg := sync.WaitGroup{}
	wg.Add(len(es))
	for _, e := range es {
		t := taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {e.Key.StringID()},
		})
		if _, err := taskqueue.Add(c, t, "update-feed"); err != nil {
			c.Errorf("taskqueue error: %v", err.Error())
		}
	}
	c.Infof("updating %d feeds", len(es))
}

func fetchFeed(c mpg.Context, url string) (*Feed, []*Story) {
	cl := urlfetch.Client(c)
	if resp, err := cl.Get(url); err == nil && resp.StatusCode == http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		return ParseFeed(c, b)
	} else if err != nil {
		c.Errorf("fetch feed error: %s", err.Error())
	} else {
		c.Errorf("fetch feed error: status code: %s", resp.Status)
	}
	return nil, nil
}

func updateFeed(c mpg.Context, url string, feed *Feed, stories []*Story) error {
	gn := goon.FromContext(c)
	f := Feed{}
	fe, _ := gn.GetById(&f, url, 0, nil)
	if fe.NotFound {
		return errors.New(fmt.Sprintf("feed not found: %s", url))
	}

	hasUpdated := !feed.Updated.IsZero()
	isFeedUpdated := f.Updated == feed.Updated

	var storyDate time.Time
	if hasUpdated {
		storyDate = f.Updated
	} else {
		storyDate = f.Checked
	}
	c.Debugf("hasUpdate: %v, isFeedUpdated: %v, storyDate: %v", hasUpdated, isFeedUpdated, storyDate)

	var datedStories, undatedStories []*Story
	for _, s := range stories {
		if s.Updated.IsZero() {
			undatedStories = append(undatedStories, s)
		} else if storyDate.Before(s.Updated) {
			datedStories = append(datedStories, s)
		}
	}
	c.Debugf("%v undated stories, %v dated stories to update", len(undatedStories), len(datedStories))

	f = *feed

	if hasUpdated && isFeedUpdated {
		c.Infof("feed %s already updated to %v, putting", url, feed.Updated)
		gn.Put(fe)
		return nil
	}

	puts := []*goon.Entity{fe}
	var updateStories []*Story

	// find non existant undated stories
	ses := make([]*goon.Entity, len(undatedStories))
	for i, s := range undatedStories {
		ses[i], _ = gn.NewEntityById(s.Id, 0, fe.Key, s)
	}
	gn.GetMulti(ses)
	for i, e := range ses {
		if e.NotFound {
			updateStories = append(updateStories, undatedStories[i])
		}
	}
	c.Debugf("%v new undated stories", len(updateStories))

	updateStories = append(updateStories, datedStories...)
	ses = make([]*goon.Entity, len(updateStories))
	scs := make([]StoryContent, len(updateStories))
	sces := make([]*goon.Entity, len(updateStories))
	for i, s := range updateStories {
		ses[i], _ = gn.NewEntityById(s.Id, 0, fe.Key, s)
		scs[i].Content = s.content
		sces[i], _ = gn.NewEntityById("", 1, ses[i].Key, &scs[i])
	}
	puts = append(puts, ses...)
	puts = append(puts, sces...)
	c.Debugf("putting %v entities", len(puts))

	if !hasUpdated && len(puts) > 1 {
		f.Updated = time.Now()
	}
	gn.PutMulti(puts)

	return nil
}

func UpdateFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	url := r.FormValue("feed")
	c.Debugf("update feed %s", url)
	f := Feed{}
	fe, _ := gn.GetById(&f, url, 0, nil)
	if fe.NotFound {
		return
	} else if time.Now().Sub(f.Updated) < UpdateTime {
		c.Infof("already updated %s", url)
		return
	}
	if feed, stories := fetchFeed(c, url); feed != nil {
		updateFeed(c, url, feed, stories)
	}
}

func ListFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{}
	ud := UserData{}
	ue, _ := gn.GetById(&u, cu.ID, 0, nil)
	gn.GetById(&ud, "data", 0, ue.Key)

	read := make(Read)
	json.Unmarshal(ud.Read, &read)
	var uf Feeds
	json.Unmarshal(ud.Feeds, &uf)
	fdc := make(chan *FeedData)
	q := datastore.NewQuery(goon.Kind(&Story{}))
	for _, f := range uf {
		go func(f *UserFeed) {
			fd := FeedData{
				Feed: f,
			}

			feed := Feed{}
			feede, _ := gn.GetById(&feed, f.Url, 0, nil)
			if u.Read.Before(feed.Updated) {
				sq := q.Ancestor(feede.Key).Filter("u >=", u.Read)
				var stories []*Story
				ses, _ := gn.GetAll(sq, &stories)
				for i, se := range ses {
					stories[i].Id = se.Key.StringID()
					found := false
					for _, s := range read[f.Url] {
						if s == stories[i].Id {
							found = true
							break
						}
					}
					if !found {
						fd.Stories = append(fd.Stories, stories[i])
					}
				}
			}
			fdc <- &fd
		}(f)
	}

	fl := make(FeedList)
	for i := 0; i < len(uf); i++ {
		fd := <-fdc
		fl[fd.Feed.Url] = fd
	}

	b, _ := json.Marshal(&fl)
	w.Write(b)
}

func MarkRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	ud := UserData{}
	read := make(Read)
	feed := r.FormValue("feed")
	story := r.FormValue("story")
	gn.RunInTransaction(func(gn *goon.Goon) error {
		ude, _ := gn.GetById(&ud, "data", 0, datastore.NewKey(c, goon.Kind(&User{}), cu.ID, 0, nil))
		json.Unmarshal(ud.Read, &read)
		read[feed] = append(read[feed], story)
		b, _ := json.Marshal(&read)
		ud.Read = b
		return gn.Put(ude)
	}, nil)
}

func MarkAllRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{}
	ud := UserData{}
	gn.RunInTransaction(func(gn *goon.Goon) error {
		ue, _ := gn.GetById(&u, cu.ID, 0, nil)
		ude, _ := gn.GetById(&ud, "data", 0, ue.Key)
		u.Read = time.Now()
		ud.Read = nil
		return gn.PutMany(ue, ude)
	}, nil)
}

func GetContents(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	var reqs []struct {
		Feed  string
		Story string
	}
	b, _ := ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(b, &reqs); err != nil {
		serveError(w, err)
		return
	}
	scs := make([]StoryContent, len(reqs))
	sces := make([]*goon.Entity, len(reqs))
	gn := goon.FromContext(c)
	for i, r := range reqs {
		fk := datastore.NewKey(c, goon.Kind(&Feed{}), r.Feed, 0, nil)
		sk := datastore.NewKey(c, goon.Kind(&Story{}), r.Story, 0, fk)
		sces[i], _ = gn.NewEntityById("", 1, sk, &scs[i])
	}
	gn.GetMulti(sces)
	ret := make([]string, len(reqs))
	for i, sc := range scs {
		ret[i] = sc.Content
	}
	b, _ = json.Marshal(&ret)
	w.Write(b)
}
