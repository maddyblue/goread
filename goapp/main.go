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
	"github.com/gorilla/mux"
	"github.com/mjibson/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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
	router.Handle("/tasks/add-feed", mpg.NewHandler(AddFeed)).Name("add-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	router.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	router.Handle("/user/import/reader", mpg.NewHandler(ImportReader)).Name("import-reader")
	router.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	router.Handle("/user/mark-all-read", mpg.NewHandler(MarkAllRead)).Name("mark-all-read")
	router.Handle("/user/mark-read", mpg.NewHandler(MarkRead)).Name("mark-read")
	router.Handle("/user/get-contents", mpg.NewHandler(GetContents)).Name("get-contents")
	http.Handle("/", router)

	miniprofiler.ShowControls = false
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	_ = goon.FromContext(c)
	if err := templates.ExecuteTemplate(w, "base.html", includes(c)); err != nil {
		serveError(w, err)
	}
}

func LoginGoogle(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if u := user.Current(c); u != nil {
		gn := goon.FromContext(c)
		user := User{}
		if e, err := gn.GetById(&user, u.ID, 0, nil); err == nil && e.NotFound {
			user.Email = u.Email
			gn.Put(e)
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
	type outline struct {
		Outline []outline `xml:"outline"`
		Title   string    `xml:"title,attr"`
		XmlUrl  string    `xml:"xmlUrl,attr"`
	}

	type Body struct {
		Outline []outline `xml:"outline"`
	}

	user := user.Current(c)

	var ts []*taskqueue.Task
	var proc func(label string, outlines []outline)
	proc = func(label string, outlines []outline) {
		for _, o := range outlines {
			if o.XmlUrl != "" {
				ts = append(ts, taskqueue.NewPOSTTask(routeUrl("add-feed"), url.Values{
					"user":  {user.ID},
					"label": {label},
					"feed":  {o.XmlUrl},
					"title": {o.Title},
				}))
			}

			if o.Title != "" && len(o.Outline) > 0 {
				proc(o.Title, o.Outline)
			}
		}
	}

	if file, _, err := r.FormFile("file"); err == nil {
		if fdata, err := ioutil.ReadAll(file); err == nil {
			fs := string(fdata)
			idx0 := strings.Index(fs, "<body>")
			idx1 := strings.LastIndex(fs, "</body>")
			fs = fs[idx0 : idx1+7]
			feed := Body{}
			if err = xml.Unmarshal([]byte(fs), &feed); err != nil {
				return
			}
			proc("", feed.Outline)
			taskqueue.AddMulti(c, ts, "add-feed")
		}
	}
}

func AddFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if err := addFeed(c, r.FormValue("user"), r.FormValue("feed"), r.FormValue("title"), r.FormValue("label"), r.FormValue("sortid")); err != nil {
		serveError(w, err)
	}
}

func addFeed(c mpg.Context, userid, feedurl, title, label, sortid string) error {
	gn := goon.FromContext(c)
	c.Infof("adding feed %s to user %s", feedurl, userid)

	u := User{}
	ue, _ := gn.GetById(&u, userid, 0, nil)
	if ue.NotFound {
		c.Errorf("user not found %s", userid)
		return nil
	}

	f := Feed{}
	updateFeed := false
	fe, err := gn.GetById(&f, feedurl, 0, nil)
	if err != nil {
		return err
	}
	if fe.NotFound {
		cl := urlfetch.Client(c)
		if r, err := cl.Get(feedurl); err != nil {
			c.Errorf("fetch error %s: %s", feedurl, err.Error())
			return err
		} else if r.StatusCode == http.StatusOK {
			b, _ := ioutil.ReadAll(r.Body)
			if feed, _ := ParseFeed(c, b); feed != nil {
				f = *feed
				// force this to be updated
				f.Updated = time.Time{}
				gn.Put(fe)
				updateFeed = true
			} else {
				c.Errorf("could not parse feed %s", feedurl)
				return errors.New("Could not parse feed")
			}
		}
	}

	if title == "" && f.Title != "" {
		title = f.Title
	}

	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		ud := UserData{}
		ude, _ := gn.GetById(&ud, "data", 0, ue.Key)
		var fg Feeds

		if len(ud.Feeds) > 0 {
			if json.Unmarshal(ud.Feeds, &fg) != nil {
				c.Errorf("unmarshal error of userdata %s", userid)
				return nil
			}
		}

		found := false
		for _, fd := range fg {
			if fd.Url == feedurl {
				found = true
				break
			}
		}
		if !found {
			fg = append(fg, &UserFeed{
				Url:    feedurl,
				Title:  title,
				Link:   f.Link,
				Label:  label,
				Sortid: sortid,
			})
			b, _ := json.Marshal(fg)
			ud.Feeds = b
			gn.Put(ude)
		}

		fi := FeedIndex{}
		fie, _ := gn.GetById(&fi, "", 1, fe.Key)
		found = false
		for _, fu := range fi.Users {
			if fu == userid {
				found = true
				break
			}
		}
		if !found {
			fi.Users = append(fi.Users, userid)
			gn.Put(fie)
		}

		return nil
	}, &datastore.TransactionOptions{XG: true}); err != nil {
		c.Errorf("transaction error %s", err.Error())
		return err
	}

	if updateFeed {
		t := taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {feedurl},
		})
		if _, err := taskqueue.Add(c, t, "update-feed"); err != nil {
			return err
		}
	}

	return nil
}

func AddSubscription(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	u := user.Current(c)
	url := r.FormValue("url")
	if err := addFeed(c, u.ID, url, "", "", ""); err != nil {
		c.Debugf("add sub error (%s): %s", url, err.Error())
		serveError(w, err)
	}
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
	json.Unmarshal(b, &v)

	wg := sync.WaitGroup{}
	wg.Add(len(v.Subscriptions))
	for i := range v.Subscriptions {
		go func(i int) {
			sub := v.Subscriptions[i]
			var label []string
			if len(sub.Categories) > 0 {
				label = append(label, sub.Categories[0].Label)
			}
			t := taskqueue.NewPOSTTask(routeUrl("add-feed"), url.Values{
				"user":   {cu.ID},
				"label":  label,
				"feed":   {sub.Id[5:]},
				"title":  {sub.Title},
				"sortid": {sub.Sortid},
			})
			c.Debugf("reader import: %s, %s", sub.Title, sub.Id)
			if _, err := taskqueue.Add(c, t, "add-feed"); err != nil {
				c.Errorf("import reader tq add error %s", err.Error())
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

const UpdateTime = time.Hour

func UpdateFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(goon.Kind(&Feed{})).KeysOnly()
	q = q.Filter("u <=", time.Now().Add(-UpdateTime))
	es, _ := gn.GetAll(q, nil)
	ts := make([]*taskqueue.Task, len(es))
	for i, e := range es {
		ts[i] = taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {e.Key.StringID()},
		})
	}
	c.Infof("updating %d feeds", len(es))
	taskqueue.AddMulti(c, ts, "update-feed")
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
	cl := urlfetch.Client(c)
	if resp, err := cl.Get(url); err == nil && resp.StatusCode == http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		if feed, stories := ParseFeed(c, b); feed != nil {
			hasUpdated := !feed.Updated.IsZero()
			isFeedUpdated := feed.Updated == f.Updated

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
			f.Checked = time.Now()

			if hasUpdated && isFeedUpdated {
				c.Infof("feed %s already updated to %v", url, feed.Updated)
				gn.Put(fe)
				return
			}

			puts := []*goon.Entity { fe }
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
			sis := make([]StoryIndex, len(updateStories))
			sies := make([]*goon.Entity, len(updateStories))
			scs := make([]StoryContent, len(updateStories))
			sces := make([]*goon.Entity, len(updateStories))
			for i, s := range updateStories {
				ses[i], _ = gn.NewEntityById(s.Id, 0, fe.Key, s)
				sies[i], _ = gn.NewEntityById("", 1, ses[i].Key, &sis[i])
				scs[i].Content = s.content
				sces[i], _ = gn.NewEntityById("", 1, ses[i].Key, &scs[i])
			}
			puts = append(puts, ses...)
			puts = append(puts, sces...)
			c.Debugf("putting %v entities", len(puts))
			gn.PutMulti(puts)

			fi := FeedIndex{}
			updateTime := time.Now().Add(-time.Hour * 24 * 7)

			gn.RunInTransaction(func(gn *goon.Goon) error {
				gn.GetById(&fi, "", 1, fe.Key)
				gn.GetMulti(sies)

				var puts []*goon.Entity
				for i, sie := range sies {
					if sie.NotFound && updateStories[i].Updated.Sub(updateTime) > 0 {
						sis[i].Users = fi.Users
						puts = append(puts, sie)
					}
				}
				gn.PutMulti(puts)
				return nil
			}, nil)
		}
	} else if err != nil {
		c.Errorf("update feed error: %s", err.Error())
	} else {
		c.Errorf("update feed error: status code: %s", resp.Status)
	}
}

func ListFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	ud := UserData{}
	gn.GetById(&ud, "data", 0, datastore.NewKey(c, goon.Kind(&User{}), cu.ID, 0, nil))

	q := datastore.NewQuery(goon.Kind(&StoryIndex{})).KeysOnly()
	q = q.Filter("u =", cu.ID)
	es, _ := gn.GetAll(q, nil)
	stories := make([]Story, len(es))
	for i, e := range es {
		es[i] = goon.NewEntity(e.Key.Parent(), &stories[i])
	}
	gn.GetMulti(es)

	fl := make(FeedList)

	var uf Feeds
	json.Unmarshal(ud.Feeds, &uf)
	for _, f := range uf {
		fl[f.Url] = &FeedData{
			Feed: f,
		}
	}

	for i, se := range es {
		k := se.Key.Parent().StringID()
		if _, present := fl[k]; !present {
			c.Errorf("Missing parent feed: %s", k)
			continue
		}
		stories[i].Id = se.Key.StringID()
		fl[k].Stories = append(fl[k].Stories, &stories[i])
	}

	b, _ := json.Marshal(fl)
	w.Write(b)
}

func MarkRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	si := StoryIndex{}
	fk := datastore.NewKey(c, goon.Kind(&Feed{}), r.FormValue("feed"), 0, nil)
	sk := datastore.NewKey(c, goon.Kind(&Story{}), r.FormValue("story"), 0, fk)
	c.Debugf(sk.String())
	gn.RunInTransaction(func(gn *goon.Goon) error {
		if sie, _ := gn.GetById(&si, "", 1, sk); !sie.NotFound {
			c.Debugf("searching")
			for i, v := range si.Users {
				if v == cu.ID {
					c.Debugf("marking %s read for %s", sk.StringID(), cu.ID)
					si.Users = append(si.Users[:i], si.Users[i+1:]...)
					gn.Put(sie)
					break
				}
			}
		}
		return nil
	}, nil)
}

func MarkAllRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	q := datastore.NewQuery(goon.Kind(&StoryIndex{}))
	q = q.Filter("u =", cu.ID)
	var sis []*StoryIndex
	sies, _ := gn.GetAll(q, &sis)

	feeds := make(map[string][]*goon.Entity)
	for _, e := range sies {
		fk := e.Key.Parent().Parent().StringID()
		feeds[fk] = append(feeds[fk], e)
	}

	wg := sync.WaitGroup{}
	wg.Add(len(feeds))

	for k, v := range feeds {
		go func(fid string, sies []*goon.Entity) {
			gn.RunInTransaction(func(gn *goon.Goon) error {
				for i := range sies {
					sies[i].Src = &StoryIndex{}
				}
				gn.GetMulti(sies)
				for _, sie := range sies {
					s := sie.Src.(*StoryIndex)
					for i, v := range s.Users {
						if v == cu.ID {
							c.Debugf("marking %s read for %s", sie.Key.Parent().StringID(), cu.ID)
							s.Users = append(s.Users[:i], s.Users[i+1:]...)
							break
						}
					}
				}
				gn.PutMulti(sies)
				return nil
			}, nil)
			wg.Done()
		}(k, v)
	}

	wg.Wait()
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
	ret := make(map[string]string)
	for i, sc := range scs {
		ret[reqs[i].Story] = sc.Content
	}
	b, _ = json.Marshal(&ret)
	w.Write(b)
}
