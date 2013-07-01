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
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"appengine/blobstore"
	"appengine/datastore"
	"appengine/runtime"
	"appengine/taskqueue"
	"appengine/urlfetch"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
)

func ImportOpmlTask(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	userid := r.FormValue("user")
	bk := r.FormValue("key")
	fr := blobstore.NewReader(c, appengine.BlobKey(bk))
	data, err := ioutil.ReadAll(fr)
	if err != nil {
		return
	}

	var skip int
	if s, err := strconv.Atoi(r.FormValue("skip")); err == nil {
		skip = s
	}
	c.Debugf("reader import for %v, skip %v", userid, skip)

	var userOpml []*OpmlOutline
	remaining := skip

	var proc func(label string, outlines []*OpmlOutline)
	proc = func(label string, outlines []*OpmlOutline) {
		for _, o := range outlines {
			if o.XmlUrl != "" {
				if remaining > 0 {
					remaining--
				} else if len(userOpml) < IMPORT_LIMIT {
					userOpml = append(userOpml, &OpmlOutline{
						Title:   label,
						Outline: []*OpmlOutline{o},
					})
				}
			}

			if o.Title != "" && len(o.Outline) > 0 {
				proc(o.Title, o.Outline)
			}
		}
	}

	opml := Opml{}
	if err := xml.Unmarshal(data, &opml); err != nil {
		c.Errorf("opml error: %v", err.Error())
		return
	}
	proc("", opml.Outline)

	// todo: refactor below with similar from ImportReaderTask
	wg := sync.WaitGroup{}
	wg.Add(len(userOpml))
	for i := range userOpml {
		go func(i int) {
			o := userOpml[i].Outline[0]
			if err := addFeed(c, userid, userOpml[i]); err != nil {
				c.Warningf("opml import error: %v", err.Error())
				// todo: do something here?
			}
			c.Debugf("opml import: %s, %s", o.Title, o.XmlUrl)
			wg.Done()
		}(i)
	}
	wg.Wait()

	ud := UserData{Id: "data", Parent: gn.Key(&User{Id: userid})}
	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		gn.Get(&ud)
		mergeUserOpml(&ud, opml.Outline...)
		_, err := gn.Put(&ud)
		return err
	}, nil); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		c.Errorf("ude update error: %v", err.Error())
		return
	}

	if len(userOpml) == IMPORT_LIMIT {
		task := taskqueue.NewPOSTTask(routeUrl("import-opml-task"), url.Values{
			"key":  {bk},
			"user": {userid},
			"skip": {strconv.Itoa(skip + IMPORT_LIMIT)},
		})
		taskqueue.Add(c, task, "import-reader")
	}
}

const IMPORT_LIMIT = 10

func ImportReaderTask(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	userid := r.FormValue("user")
	bk := r.FormValue("key")
	fr := blobstore.NewReader(c, appengine.BlobKey(bk))
	data, err := ioutil.ReadAll(fr)
	if err != nil {
		return
	}

	var skip int
	if s, err := strconv.Atoi(r.FormValue("skip")); err == nil {
		skip = s
	}

	v := struct {
		Subscriptions []struct {
			Id         string `json:"id"`
			Title      string `json:"title"`
			HtmlUrl    string `json:"htmlUrl"`
			Categories []struct {
				Id    string `json:"id"`
				Label string `json:"label"`
			} `json:"categories"`
		} `json:"subscriptions"`
	}{}
	json.Unmarshal(data, &v)
	c.Debugf("reader import for %v, skip %v, len %v", userid, skip, len(v.Subscriptions))

	end := skip + IMPORT_LIMIT
	if end > len(v.Subscriptions) {
		end = len(v.Subscriptions)
	}

	wg := sync.WaitGroup{}
	wg.Add(end - skip)
	userOpml := make([]*OpmlOutline, end-skip)

	for i := range v.Subscriptions[skip:end] {
		go func(i int) {
			sub := v.Subscriptions[skip+i]
			var label string
			if len(sub.Categories) > 0 {
				label = sub.Categories[0].Label
			}
			outline := &OpmlOutline{
				Title: label,
				Outline: []*OpmlOutline{
					&OpmlOutline{
						XmlUrl: sub.Id[5:],
						Title:  sub.Title,
					},
				},
			}
			userOpml[i] = outline
			if err := addFeed(c, userid, outline); err != nil {
				c.Warningf("reader import error: %v", err.Error())
				// todo: do something here?
			}
			c.Debugf("reader import: %s, %s", sub.Title, sub.Id)
			wg.Done()
		}(i)
	}
	wg.Wait()

	ud := UserData{Id: "data", Parent: gn.Key(&User{Id: userid})}
	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		gn.Get(&ud)
		mergeUserOpml(&ud, userOpml...)
		_, err := gn.Put(&ud)
		return err
	}, nil); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		c.Errorf("ude update error: %v", err.Error())
		return
	}

	if end < len(v.Subscriptions) {
		task := taskqueue.NewPOSTTask(routeUrl("import-reader-task"), url.Values{
			"key":  {bk},
			"user": {userid},
			"skip": {strconv.Itoa(skip + IMPORT_LIMIT)},
		})
		taskqueue.Add(c, task, "import-reader")
	} else {
		blobstore.Delete(c, appengine.BlobKey(bk))
	}
}

func BackendStart(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	const sz = 100
	ic := 0
	gn := goon.FromContext(c)
	fk := gn.Key(&Feed{Url: "a"})
	q := datastore.NewQuery("F").Filter("__key__ <", fk).Order("__key__").KeysOnly().Limit(1)
	keys, _ := q.GetAll(c, nil)
	if len(keys) == 0 {
		return
	}
	c.Errorf("start: %v", keys[0])
	startid := keys[0].IntID() / sz

	var f func(appengine.Context)
	f = func(c appengine.Context) {
		c.Errorf("new request: %d", ic)
		t1 := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(sz)
		var j int64
		for j = 0; j < sz; j++ {
			go func(j int64) {
				k := datastore.NewKey(c, "F", "", startid*sz+j, nil)
				c.Infof("del: %v", k)
				if err := datastore.Delete(c, k); err != nil {
					c.Errorf("delete err: %v", err.Error())
				}
				wg.Done()
			}(j)
		}
		wg.Wait()
		t2 := time.Now()
		c.Infof("%v, %v, %v", t1, t2, t2.Sub(t1))
		ic++
		startid++
		runtime.RunInBackground(c, f)
	}
	runtime.RunInBackground(c, f)
}

func BackendStop(c mpg.Context, w http.ResponseWriter, r *http.Request) {
}

func BackendStart1(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	dc := 0
	up := 0
	ic := 0
	const limit = 100
	done := make(map[string]bool)

	var f func(appengine.Context)
	f = func(c appengine.Context) {
		c.Errorf("new request: %d", ic)
		wg := sync.WaitGroup{}
		ic++
		gn := goon.FromContext(c)
		fk := gn.Key(&Feed{Url: "a"})
		q := datastore.NewQuery(fk.Kind()).KeysOnly().Limit(limit).Filter("__key__ <", fk)
		it := q.Run(c)
		var update, delete []*datastore.Key
		for i := 0; i < limit; i++ {
			c.Infof("i: %d", i)
			if k, err := it.Next(nil); err != nil {
				c.Errorf("next error: %v", err.Error())
				break
			} else if len(k.StringID()) == 0 {
				delete = append(delete, k)
			} else {
				//update = append(update, k)
			}
		}
		for _, k := range delete {
			if done[k.String()] {
				continue
			} else {
				done[k.String()] = true
			}
			go func(k *datastore.Key) {
				if err := datastore.Delete(c, k); err != nil {
					c.Errorf("delete error: %v, %v", k, err.Error())
				} else {
					c.Infof("deleted: %v, %d, %d", k, dc, len(done))
					dc++
				}
				wg.Done()
			}(k)
			wg.Add(1)
		}
		for _, k := range update {
			go func(k *datastore.Key) {
				t := taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
					"feed": {k.StringID()},
				})
				if _, err := taskqueue.Add(c, t, "update-feed"); err != nil {
					c.Errorf("taskqueue error: %v, %v", k, err.Error())
				} else {
					c.Infof("updating: %v, %d", k, up)
					up++
				}
				wg.Done()
			}(k)
			wg.Add(1)
		}

		wg.Wait()
		time.Sleep(1)
		runtime.RunInBackground(c, f)
	}
	runtime.RunInBackground(c, f)
}

func UpdateFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	q := datastore.NewQuery("F").KeysOnly()
	q = q.Filter("n <=", time.Now())
	q = q.Limit(3000)
	var keys []*datastore.Key
	for i := 0; i < 5; i++ {
		if _keys, err := q.GetAll(c, nil); err != nil {
			c.Errorf("get all error: %v, retry %v", err.Error(), i)
		} else {
			c.Errorf("got %v keys", len(_keys))
			keys = _keys
			break
		}
	}
	if len(keys) == 0 {
		c.Errorf("giving up")
		return
	}

	tasks := make([]*taskqueue.Task, len(keys))
	for i, k := range keys {
		tasks[i] = taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {k.StringID()},
		})
	}
	var ts []*taskqueue.Task
	const taskLimit = 100
	for len(tasks) > 0 {
		if len(tasks) > taskLimit {
			ts = tasks[:taskLimit]
			tasks = tasks[taskLimit:]
		} else {
			ts = tasks
			tasks = tasks[0:0]
		}
		if _, err := taskqueue.AddMulti(c, ts, "update-feed"); err != nil {
			c.Errorf("taskqueue error: %v", err.Error())
		}
	}
	c.Infof("updating %d feeds", len(keys))
}

func fetchFeed(c mpg.Context, origUrl, fetchUrl string) (*Feed, []*Story) {
	u, err := url.Parse(fetchUrl)
	_orig := origUrl
	if err == nil && u.Scheme == "" {
		u.Scheme = "http"
		origUrl = u.String()
		fetchUrl = origUrl
		if origUrl == "" {
			c.Criticalf("badurl1: %v, %v, %v, %v", _orig, u, origUrl, fetchUrl)
			return nil, nil
		}
	}
	if strings.TrimSpace(origUrl) == "" {
		c.Criticalf("badurl2: %v, %v", _orig, origUrl)
		return nil, nil
	}

	cl := &http.Client{
		Transport: &urlfetch.Transport{
			Context:  c,
			Deadline: time.Minute,
		},
	}
	if resp, err := cl.Get(fetchUrl); err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		b, _ := ioutil.ReadAll(resp.Body)
		if autoUrl, err := Autodiscover(b); err == nil && origUrl == fetchUrl {
			if autoU, err := url.Parse(autoUrl); err == nil {
				if autoU.Scheme == "" {
					autoU.Scheme = u.Scheme
				}
				if autoU.Host == "" {
					autoU.Host = u.Host
				}
				autoUrl = autoU.String()
			}
			return fetchFeed(c, origUrl, autoUrl)
		}
		return ParseFeed(c, origUrl, b)
	} else if err != nil {
		c.Warningf("fetch feed error: %s", err.Error())
	} else {
		c.Warningf("fetch feed error: status code: %s", resp.Status)
	}
	return nil, nil
}

func updateFeed(c mpg.Context, url string, feed *Feed, stories []*Story) error {
	gn := goon.FromContext(c)
	f := Feed{Url: url}
	if err := gn.Get(&f); err != nil {
		return errors.New(fmt.Sprintf("feed not found: %s", url))
	}

	// Compare the feed's listed update to the story's update.
	// Note: these may not be accurate, hence, only compare them to each other,
	// since they should have the same relative error.
	storyDate := f.Updated

	hasUpdated := !feed.Updated.IsZero()
	isFeedUpdated := f.Updated == feed.Updated
	if !hasUpdated {
		feed.Updated = f.Updated
	}
	feed.Date = f.Date
	f = *feed

	if hasUpdated && isFeedUpdated {
		c.Infof("feed %s already updated to %v, putting", url, feed.Updated)
		f.Updated = time.Now()
		if strings.TrimSpace(f.Url) == "" {
			c.Criticalf("badurl5: %v, %v", url, f)
			return errors.New("badurl5")
		}
		gn.PutComplete(&f)
		return nil
	}

	c.Debugf("hasUpdate: %v, isFeedUpdated: %v, storyDate: %v", hasUpdated, isFeedUpdated, storyDate)

	var newStories []*Story
	for _, s := range stories {
		if s.Updated.IsZero() || !s.Updated.Before(storyDate) {
			newStories = append(newStories, s)
		}
	}
	c.Debugf("%v possible stories to update", len(newStories))

	puts := []interface{}{&f}

	// find non existant stories
	fk := gn.Key(&f)
	getStories := make([]*Story, len(newStories))
	for i, s := range newStories {
		getStories[i] = &Story{Id: s.Id, Parent: fk}
	}
	err := gn.GetMulti(getStories)
	if _, ok := err.(appengine.MultiError); err != nil && !ok {
		c.Errorf("get multi error: %v", err.Error())
		return err
	}
	var updateStories []*Story
	for i, s := range getStories {
		if goon.NotFound(err, i) {
			updateStories = append(updateStories, newStories[i])
		} else if !newStories[i].Updated.IsZero() && !newStories[i].Updated.Equal(s.Updated) {
			newStories[i].Created = s.Created
			newStories[i].Published = s.Published
			updateStories = append(updateStories, newStories[i])
		}
	}
	c.Debugf("%v update stories", len(updateStories))

	for _, s := range updateStories {
		puts = append(puts, s)
		gn.Put(&StoryContent{
			Id:      1,
			Parent:  gn.Key(s),
			Content: s.content,
		})
	}

	c.Debugf("putting %v entities", len(puts))
	if len(puts) > 1 {
		f.Date = time.Now()
		if !hasUpdated {
			f.Updated = f.Date
		}
	}
	if f.Url == "" {
		c.Criticalf("badurl6: %v", f)
		return errors.New("badurl6")
	}
	gn.PutMultiComplete(puts)

	return nil
}

func UpdateFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	url := r.FormValue("feed")
	c.Debugf("update feed %s", url)
	f := Feed{Url: url}
	if err := gn.Get(&f); err == datastore.ErrNoSuchEntity {
		return
	} else if time.Now().Before(f.NextUpdate) {
		c.Infof("feed %v already updated", url)
		return
	}
	if f.Url == "" {
		c.Criticalf("badurl7: %v", url)
		return
	}

	feedError := func() {
		f.Errors++
		v := f.Errors + 1
		const max = 24 * 7
		if v > max {
			v = max
		} else if f.Errors == 1 {
			v = 0
		}
		f.NextUpdate = time.Now().Add(time.Hour * time.Duration(v))
		gn.PutComplete(&f)
		c.Warningf("error with %v (%v), bump next update to %v", url, f.Errors, f.NextUpdate)
	}

	if feed, stories := fetchFeed(c, url, url); feed != nil {
		if err := updateFeed(c, url, feed, stories); err != nil {
			feedError()
		}
	} else {
		feedError()
	}
}
