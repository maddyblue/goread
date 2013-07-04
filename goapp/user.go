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
	"appengine"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"appengine/blobstore"
	"appengine/datastore"
	"appengine/taskqueue"
	"appengine/urlfetch"
	"appengine/user"
	"code.google.com/p/goauth2/oauth"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
)

func LoginGoogle(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if cu := user.Current(c); cu != nil {
		gn := goon.FromContext(c)
		u := &User{Id: cu.ID}
		if err := gn.Get(u); err == datastore.ErrNoSuchEntity {
			u.Email = cu.Email
			u.Read = time.Now().Add(-time.Hour * 24)
			gn.Put(u)
		}
	}

	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func Logout(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if appengine.IsDevAppServer() {
		if u, err := user.LogoutURL(c, routeUrl("main")); err == nil {
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
	} else {
		http.SetCookie(w, &http.Cookie{
			Name:    "ACSID",
			Value:   "",
			Expires: time.Time{},
		})
	}
	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func ImportOpml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}

	if file, _, err := r.FormFile("file"); err == nil {
		if fdata, err := ioutil.ReadAll(file); err == nil {
			bk, err := saveFile(c, fdata)
			if err != nil {
				serveError(w, err)
				return
			}
			task := taskqueue.NewPOSTTask(routeUrl("import-opml-task"), url.Values{
				"key":  {string(bk)},
				"user": {cu.ID},
			})
			taskqueue.Add(c, task, "import-reader")
		}
	}
}

func AddSubscription(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	url := r.FormValue("url")
	o := &OpmlOutline{
		Outline: []*OpmlOutline{
			&OpmlOutline{XmlUrl: url},
		},
	}
	if err := addFeed(c, cu.ID, o); err != nil {
		c.Errorf("add sub error (%s): %s", url, err.Error())
		serveError(w, err)
		return
	}

	gn := goon.FromContext(c)
	ud := UserData{Id: "data", Parent: gn.Key(&User{Id: cu.ID})}
	gn.Get(&ud)
	mergeUserOpml(&ud, o)
	gn.Put(&ud)
}

func ImportReader(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, oauth_conf.AuthCodeURL(""), http.StatusFound)
}

func Oauth2Callback(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	if cu == nil {
		serveError(w, errors.New("Not logged in"))
		return
	}
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	u.Messages = append(u.Messages,
		"Reader import is happening. It can take a minute. Don't reorganize your feeds until it's completed importing.",
		"Refresh to see its progress.",
	)
	gn.Put(&u)

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
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)
	bk, err := saveFile(c, b)
	if err != nil {
		serveError(w, err)
		return
	}
	task := taskqueue.NewPOSTTask(routeUrl("import-reader-task"), url.Values{
		"key":  {string(bk)},
		"user": {cu.ID},
	})
	taskqueue.Add(c, task, "import-reader")
	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func saveFile(c appengine.Context, b []byte) (appengine.BlobKey, error) {
	w, err := blobstore.Create(c, "application/json")
	if err != nil {
		return "", err
	}
	if _, err := w.Write(b); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return w.Key()
}

func ListFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := &User{Id: cu.ID}
	ud := &UserData{Id: "data", Parent: gn.Key(u)}
	gn.GetMulti([]interface{}{u, ud})

	read := make(Read)
	var uf Opml
	c.Step("unmarshal user data", func() {
		json.Unmarshal(ud.Read, &read)
		json.Unmarshal(ud.Opml, &uf)
	})
	var feeds []*Feed
	opmlMap := make(map[string]*OpmlOutline)
	var merr error
	c.Step("fetch feeds", func() {
		for _, outline := range uf.Outline {
			if outline.XmlUrl == "" {
				for _, so := range outline.Outline {
					feeds = append(feeds, &Feed{Url: so.XmlUrl})
					opmlMap[so.XmlUrl] = so
				}
			} else {
				feeds = append(feeds, &Feed{Url: outline.XmlUrl})
				opmlMap[outline.XmlUrl] = outline
			}
		}
		merr = gn.GetMulti(feeds)
	})
	lock := sync.Mutex{}
	fl := make(map[string][]*Story)
	q := datastore.NewQuery(gn.Key(&Story{}).Kind())
	hasStories := false
	updatedLinks := false
	icons := make(map[string]string)
	now := time.Now()

	c.Step("feed fetch + wait", func() {
		queue := make(chan *Feed)
		wg := sync.WaitGroup{}
		feedProc := func() {
			for f := range queue {
				defer wg.Done()
				var newStories []*Story

				if u.Read.Before(f.Date) {
					c.Debugf("query for %v", f.Url)
					fk := gn.Key(f)
					sq := q.Ancestor(fk).Filter("p >", u.Read).KeysOnly().Order("-p")
					keys, _ := gn.GetAll(sq, nil)
					stories := make([]*Story, len(keys))
					for j, key := range keys {
						stories[j] = &Story{
							Id:     key.StringID(),
							Parent: fk,
						}
					}
					gn.GetMulti(stories)
					for _, st := range stories {
						found := false
						for _, s := range read[f.Url] {
							if s == st.Id {
								found = true
								break
							}
						}
						if !found {
							newStories = append(newStories, st)
						}
					}
				}
				if f.Link != opmlMap[f.Url].HtmlUrl {
					updatedLinks = true
					opmlMap[f.Url].HtmlUrl = f.Link
				}
				if f.Errors == 0 && f.NextUpdate.Before(now) {
					t := taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
						"feed": {f.Url},
					})
					if _, err := taskqueue.Add(c, t, "update-manual"); err != nil {
						c.Errorf("taskqueue error: %v", err.Error())
					} else {
						c.Warningf("manual feed update: %v", f.Url)
					}
				}
				f.Subscribe(c)
				lock.Lock()
				fl[f.Url] = newStories
				if len(newStories) > 0 {
					hasStories = true
				}
				if f.Image != "" {
					icons[f.Url] = f.Image
				}
				lock.Unlock()
			}
		}
		for i := 0; i < 20; i++ {
			go feedProc()
		}
		for i, f := range feeds {
			if goon.NotFound(merr, i) {
				continue
			}
			wg.Add(1)
			queue <- f
		}
		close(queue)
		wg.Wait()
	})
	if !hasStories {
		var last time.Time
		for _, f := range feeds {
			if last.Before(f.Date) {
				last = f.Date
			}
		}
		if u.Read.Before(last) {
			c.Debugf("setting %v read to %v", cu.ID, last)
			u.Read = last
			ud.Read = nil
			gn.PutMany(u, ud)
		}
	}
	if updatedLinks {
		ud.Opml, _ = json.Marshal(&uf)
		gn.Put(ud)
	}
	c.Step("json marshal", func() {
		o := struct {
			Opml    []*OpmlOutline
			Stories map[string][]*Story
			Icons   map[string]string
			Options string
		}{
			Opml:    uf.Outline,
			Stories: fl,
			Icons:   icons,
			Options: u.Options,
		}
		b, err := json.Marshal(o)
		if err != nil {
			c.Errorf("cleaning")
			for _, v := range fl {
				for _, s := range v {
					n := cleanNonUTF8(s.Summary)
					if n != s.Summary {
						s.Summary = n
						c.Errorf("cleaned %v", s.Id)
						gn.Put(s)
					}
				}
			}
			b, _ = json.Marshal(o)
		}
		w.Write(b)
	})
	_ = utf8.RuneError
}

func cleanNonUTF8(s string) string {
	b := &bytes.Buffer{}
	for i := 0; i < len(s); i++ {
		c, size := utf8.DecodeRuneInString(s[i:])
		if c != utf8.RuneError || size != 1 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func MarkRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	read := make(Read)

	type readStory struct {
		Feed, Story string
	}
	var stories []readStory
	if r.FormValue("stories") != "" {
		json.Unmarshal([]byte(r.FormValue("stories")), &stories)
	}
	if r.FormValue("feed") != "" {
		stories = append(stories, readStory{
			Feed:  r.FormValue("feed"),
			Story: r.FormValue("story"),
		})
	}

	gn.RunInTransaction(func(gn *goon.Goon) error {
		u := &User{Id: cu.ID}
		ud := &UserData{
			Id:     "data",
			Parent: gn.Key(u),
		}
		gn.Get(ud)
		json.Unmarshal(ud.Read, &read)
		for _, s := range stories {
			read[s.Feed] = append(read[s.Feed], s.Story)
		}
		b, _ := json.Marshal(&read)
		ud.Read = b
		_, err := gn.Put(ud)
		return err
	}, nil)
}

func MarkAllRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := &User{Id: cu.ID}
	ud := &UserData{Id: "data", Parent: gn.Key(u)}
	last := r.FormValue("last")
	gn.RunInTransaction(func(gn *goon.Goon) error {
		gn.GetMulti([]interface{}{u, ud})
		if ilast, err := strconv.ParseInt(last, 10, 64); err == nil && ilast > 0 && false {
			u.Read = time.Unix(ilast/1000, 0)
		} else {
			u.Read = time.Now()
		}
		ud.Read = nil
		_, err := gn.PutMany(u, ud)
		return err
	}, nil)
}

func GetContents(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	var reqs []struct {
		Feed  string
		Story string
	}
	defer r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(b, &reqs); err != nil {
		serveError(w, err)
		return
	}
	scs := make([]*StoryContent, len(reqs))
	gn := goon.FromContext(c)
	for i, r := range reqs {
		f := &Feed{Url: r.Feed}
		s := &Story{Id: r.Story, Parent: gn.Key(f)}
		scs[i] = &StoryContent{Id: 1, Parent: gn.Key(s)}
	}
	gn.GetMulti(scs)
	ret := make([]string, len(reqs))
	for i, sc := range scs {
		ret[i] = sc.Content
	}
	b, _ = json.Marshal(&ret)
	w.Write(b)
}

func ClearFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if !isDevServer {
		return
	}

	cu := user.Current(c)
	gn := goon.FromContext(c)
	done := make(chan bool)
	go func() {
		u := &User{Id: cu.ID}
		defer func() { done <- true }()
		ud := &UserData{Id: "data", Parent: gn.Key(u)}
		if err := gn.Get(u); err != nil {
			c.Errorf("user del err: %v", err.Error())
			return
		}
		gn.Get(ud)
		u.Read = time.Time{}
		ud.Read = nil
		ud.Opml = nil
		gn.PutMany(u, ud)
		c.Infof("%v cleared", u.Email)
	}()
	del := func(kind string) {
		defer func() { done <- true }()
		q := datastore.NewQuery(kind).KeysOnly()
		keys, err := gn.GetAll(q, nil)
		if err != nil {
			c.Errorf("err: %v", err.Error())
			return
		}
		if err := gn.DeleteMulti(keys); err != nil {
			c.Errorf("err: %v", err.Error())
			return
		}
		c.Infof("%v deleted", kind)
	}
	for _, i := range []interface{}{&Feed{}, &Story{}, &StoryContent{}} {
		k := gn.Key(i).Kind()
		go del(k)
	}

	for i := 0; i < 4; i++ {
		<-done
	}

	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func ExportOpml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	ud := UserData{Id: "data", Parent: gn.Key(&User{Id: cu.ID})}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	gn.Get(&ud)
	opml := Opml{}
	json.Unmarshal(ud.Opml, &opml)
	b, _ := xml.MarshalIndent(&opml, "", "\t")
	w.Header().Add("Content-Type", "text/xml")
	w.Header().Add("Content-Disposition", "attachment; filename=subscriptions.opml")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`, string(b))
}

func UploadOpml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	opml := Opml{}
	if err := json.Unmarshal([]byte(r.FormValue("opml")), &opml.Outline); err != nil {
		serveError(w, err)
		return
	}
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	ud := UserData{Id: "data", Parent: gn.Key(&User{Id: cu.ID})}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	gn.Get(&ud)
	ud.Opml, _ = json.Marshal(&opml)
	gn.Put(&ud)
}

func SaveOptions(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	gn.RunInTransaction(func(gn *goon.Goon) error {
		u := User{Id: cu.ID}
		if err := gn.Get(&u); err != nil {
			serveError(w, err)
			return nil
		}
		u.Options = r.FormValue("options")
		_, err := gn.Put(&u)
		return err
	}, nil)
}

func GetFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	f := Feed{Url: r.URL.Query().Get("f")}
	fk := gn.Key(&f)
	q := datastore.NewQuery(gn.Key(&Story{}).Kind()).Ancestor(fk).KeysOnly()
	q = q.Order("-p")
	if c := r.URL.Query().Get("c"); c != "" {
		if dc, err := datastore.DecodeCursor(c); err == nil {
			q = q.Start(dc)
		}
	}
	iter := gn.Run(q)
	var stories []*Story
	for i := 0; i < 20; i++ {
		if k, err := iter.Next(nil); err == nil {
			stories = append(stories, &Story{
				Id:     k.StringID(),
				Parent: k.Parent(),
			})
		} else if err != datastore.Done {
			serveError(w, err)
			return
		}
	}
	cursor := ""
	if ic, err := iter.Cursor(); err == nil {
		cursor = ic.String()
	}
	gn.GetMulti(&stories)
	b, _ := json.Marshal(struct {
		Cursor  string
		Stories []*Story
	}{
		Cursor:  cursor,
		Stories: stories,
	})
	w.Write(b)
}

func DeleteAccount(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	ud := UserData{Id: "data", Parent: gn.Key(&u)}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	gn.Delete(gn.Key(&ud))
	gn.Delete(ud.Parent)
	http.Redirect(w, r, routeUrl("logout"), http.StatusFound)
}
