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
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

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
	if u, err := user.LogoutURL(c, routeUrl("main")); err == nil {
		http.Redirect(w, r, u, http.StatusFound)
	} else {
		http.Redirect(w, r, routeUrl("main"), http.StatusFound)
	}
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
			task := taskqueue.NewPOSTTask(routeUrl("import-opml-task"), url.Values{
				"data": {string(fdata)},
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
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	u.Messages = append(u.Messages,
		"Reader import is happening. It can take a minute.",
		"Refresh at will - you'll continue to see this page until it's done.",
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
	b, _ := ioutil.ReadAll(resp.Body)
	task := taskqueue.NewPOSTTask(routeUrl("import-reader-task"), url.Values{
		"data": {string(b)},
		"user": {cu.ID},
	})
	taskqueue.Add(c, task, "import-reader")
	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
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
	c.Step("fetch feeds", func() {
		for _, outline := range uf.Outline {
			if outline.XmlUrl == "" {
				for _, so := range outline.Outline {
					feeds = append(feeds, &Feed{Url: so.XmlUrl})
				}
			} else {
				feeds = append(feeds, &Feed{Url: outline.XmlUrl})
			}
		}
		gn.GetMulti(feeds)
	})
	lock := sync.Mutex{}
	fl := make(map[string][]*Story)
	q := datastore.NewQuery(gn.Key(&Story{}).Kind())
	hasStories := false
	c.Step("feed fetch + wait", func() {
		wg := sync.WaitGroup{}
		wg.Add(len(feeds))
		for _i := range feeds {
			go func(i int) {
				f := feeds[i]
				defer wg.Done()
				ufeed := feeds[i]
				var newStories []*Story

				if u.Read.Before(f.Date) {
					c.Debugf("query for %v", feeds[i].Url)
					fk := gn.Key(feeds[i])
					sq := q.Ancestor(fk).Filter("p >", u.Read).KeysOnly()
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
						for _, s := range read[ufeed.Url] {
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
				lock.Lock()
				fl[ufeed.Url] = newStories
				if len(newStories) > 0 {
					hasStories = true
				}
				lock.Unlock()
			}(_i)
		}

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
			gn.PutMany(&u, &ud)
		}
	}
	c.Step("json marshal", func() {
		b, _ := json.Marshal(struct {
			Opml    []*OpmlOutline
			Stories map[string][]*Story
		}{
			Opml:    uf.Outline,
			Stories: fl,
		})
		w.Write(b)
	})
}

func MarkRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	read := make(Read)
	feed := r.FormValue("feed")
	story := r.FormValue("story")
	gn.RunInTransaction(func(gn *goon.Goon) error {
		u := &User{Id: cu.ID}
		ud := &UserData{
			Id:     "data",
			Parent: gn.Key(u),
		}
		gn.Get(ud)
		json.Unmarshal(ud.Read, &read)
		read[feed] = append(read[feed], story)
		b, _ := json.Marshal(&read)
		ud.Read = b
		return gn.Put(ud)
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
			u.Read = time.Unix(ilast, 0)
		} else {
			u.Read = time.Now()
		}
		ud.Read = nil
		return gn.PutMany(u, ud)
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
