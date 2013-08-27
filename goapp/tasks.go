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
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"appengine"
	"appengine/blobstore"
	"appengine/datastore"
	"appengine/taskqueue"
	"appengine/urlfetch"
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/gorilla/mux"
	"github.com/mjibson/goon"
)

func ImportOpmlTask(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	userid := r.FormValue("user")
	bk := r.FormValue("key")
	fr := blobstore.NewReader(c, appengine.BlobKey(bk))

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
			if o.Title == "" {
				o.Title = o.Text
			}
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
	d := xml.NewDecoder(fr)
	d.CharsetReader = charset.NewReader
	d.Strict = false
	if err := d.Decode(&opml); err != nil {
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
		if err := mergeUserOpml(c, &ud, userOpml...); err != nil {
			return err
		}
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
	} else {
		c.Infof("opml import done: %v", userid)
	}
}

const IMPORT_LIMIT = 10

func SubscribeCallback(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	furl := r.FormValue("feed")
	oldURL := false
	if len(furl) == 0 {
		vars := mux.Vars(r)
		furl = vars["feed"]
		oldURL = true
	}
	b, _ := base64.URLEncoding.DecodeString(furl)
	f := Feed{Url: string(b)}
	c.Infof("url: %v", f.Url)
	if err := gn.Get(&f); err != nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	if r.Method == "GET" {
		if oldURL {
			c.Warningf("old url")
			http.Error(w, "", http.StatusNotFound)
			return
		}
		if f.NotViewed() || r.FormValue("hub.mode") != "subscribe" || r.FormValue("hub.topic") != f.Url {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		w.Write([]byte(r.FormValue("hub.challenge")))
		i, _ := strconv.Atoi(r.FormValue("hub.lease_seconds"))
		f.Subscribed = time.Now().Add(time.Second * time.Duration(i))
		gn.Put(&f)
		c.Debugf("subscribed: %v - %v", f.Url, f.Subscribed)
		return
	} else if !f.NotViewed() {
		c.Infof("push: %v", f.Url)
		defer r.Body.Close()
		b, _ := ioutil.ReadAll(r.Body)
		nf, ss, err := ParseFeed(c, f.Url, b)
		if err != nil {
			c.Errorf("parse error: %v", err)
			return
		}
		if err := updateFeed(c, f.Url, nf, ss, false, true, false); err != nil {
			c.Errorf("push error: %v", err)
		}
	} else {
		c.Infof("not viewed")
	}
}

func SubscribeFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	f := Feed{Url: r.FormValue("feed")}
	if err := gn.Get(&f); err != nil {
		c.Errorf("%v: %v", err, f.Url)
		serveError(w, err)
		return
	} else if f.IsSubscribed() {
		return
	}
	u := url.Values{}
	u.Add("hub.callback", f.PubSubURL())
	u.Add("hub.mode", "subscribe")
	u.Add("hub.verify", "sync")
	fu, _ := url.Parse(f.Url)
	fu.Fragment = ""
	u.Add("hub.topic", fu.String())
	req, err := http.NewRequest("POST", PUBSUBHUBBUB_HUB, strings.NewReader(u.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cl := &http.Client{
		Transport: &urlfetch.Transport{
			Context:  c,
			Deadline: time.Minute,
		},
	}
	resp, err := cl.Do(req)
	if err != nil {
		c.Errorf("req error: %v", err)
	} else if resp.StatusCode != http.StatusNoContent {
		f.Subscribed = time.Now().Add(time.Hour * 48)
		gn.Put(&f)
		if resp.StatusCode != http.StatusConflict {
			c.Errorf("resp: %v - %v", f.Url, resp.Status)
			c.Errorf("%s", resp.Body)
		}
	} else {
		c.Infof("subscribed: %v", f.Url)
	}
}

func UpdateFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	q := datastore.NewQuery("F").KeysOnly().Filter("n <=", time.Now())
	q = q.Limit(10 * 60 * 20) // 10/s queue, 20 min cron
	it := q.Run(appengine.Timeout(c, time.Second*60))
	tc := make(chan *taskqueue.Task)
	done := make(chan bool)
	i := 0
	u := routeUrl("update-feed")
	go taskSender(c, "update-feed", tc, done)
	for {
		k, err := it.Next(nil)
		if err == datastore.Done {
			break
		} else if err != nil {
			c.Errorf("next error: %v", err.Error())
			break
		}
		tc <- taskqueue.NewPOSTTask(u, url.Values{
			"feed": {k.StringID()},
		})
		i++
	}
	close(tc)
	<-done
	c.Infof("updating %d feeds", i)
}

func fetchFeed(c mpg.Context, origUrl, fetchUrl string) (*Feed, []*Story, error) {
	u, err := url.Parse(fetchUrl)
	if err != nil {
		return nil, nil, err
	}
	if u.Host == "" {
		u.Host = u.Path
		u.Path = ""
	}
	const clURL = "craigslist.org"
	if strings.HasSuffix(u.Host, clURL) || u.Host == clURL {
		return nil, nil, fmt.Errorf("Craigslist blocks our server host: not possible to subscribe")
	}
	if u.Scheme == "" {
		u.Scheme = "http"
		origUrl = u.String()
		fetchUrl = origUrl
		if origUrl == "" {
			return nil, nil, fmt.Errorf("bad URL")
		}
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
			if autoUrl != fetchUrl {
				return fetchFeed(c, origUrl, autoUrl)
			}
		}
		return ParseFeed(c, origUrl, b)
	} else if err != nil {
		c.Warningf("fetch feed error: %v", err)
		return nil, nil, fmt.Errorf("Could not fetch feed")
	} else {
		c.Warningf("fetch feed error: status code: %s", resp.Status)
		return nil, nil, fmt.Errorf("Bad response code from server")
	}
}

func updateFeed(c mpg.Context, url string, feed *Feed, stories []*Story, updateAll, fromSub, updateLast bool) error {
	gn := goon.FromContext(c)
	f := Feed{Url: url}
	if err := gn.Get(&f); err != nil {
		return fmt.Errorf("feed not found: %s", url)
	}

	// Compare the feed's listed update to the story's update.
	// Note: these may not be accurate, hence, only compare them to each other,
	// since they should have the same relative error.
	storyDate := f.Updated

	hasUpdated := !feed.Updated.IsZero()
	isFeedUpdated := f.Updated.Equal(feed.Updated)
	if !hasUpdated {
		feed.Updated = f.Updated
	}
	feed.Date = f.Date
	feed.Average = f.Average
	feed.LastViewed = f.LastViewed
	f = *feed
	if updateLast {
		f.LastViewed = time.Now()
	}

	if hasUpdated && isFeedUpdated && !updateAll && !fromSub {
		c.Errorf("feed %s already updated to %v, putting", url, feed.Updated)
		f.Updated = time.Now()
		scheduleNextUpdate(&f)
		gn.Put(&f)
		return nil
	}

	c.Debugf("hasUpdate: %v, isFeedUpdated: %v, storyDate: %v, stories: %v", hasUpdated, isFeedUpdated, storyDate, len(stories))
	puts := []interface{}{&f}

	// find non existant stories
	fk := gn.Key(&f)
	getStories := make([]*Story, len(stories))
	for i, s := range stories {
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
			updateStories = append(updateStories, stories[i])
		} else if (!stories[i].Updated.IsZero() && !stories[i].Updated.Equal(s.Updated)) || updateAll {
			stories[i].Created = s.Created
			stories[i].Published = s.Published
			updateStories = append(updateStories, stories[i])
		}
	}
	c.Debugf("%v update stories", len(updateStories))

	for _, s := range updateStories {
		puts = append(puts, s)
		sc := StoryContent{
			Id:     1,
			Parent: gn.Key(s),
		}
		buf := &bytes.Buffer{}
		if gz, err := gzip.NewWriterLevel(buf, gzip.BestCompression); err == nil {
			gz.Write([]byte(s.content))
			gz.Close()
			sc.Compressed = buf.Bytes()
		}
		if len(sc.Compressed) == 0 {
			sc.Content = s.content
		}
		gn.Put(&sc)
	}

	c.Debugf("putting %v entities", len(puts))
	if len(puts) > 1 {
		updateAverage(&f, f.Date, len(puts)-1)
		f.Date = time.Now()
		if !hasUpdated {
			f.Updated = f.Date
		}
	}
	if fromSub {
		f.NextUpdate = time.Now().Add(time.Hour * 24)
	} else {
		scheduleNextUpdate(&f)
	}
	delay := f.NextUpdate.Sub(time.Now())
	c.Infof("next update scheduled for %v from now", delay-delay%time.Second)
	gn.PutMulti(puts)
	return nil
}

func UpdateFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	url := r.FormValue("feed")
	c.Debugf("update feed %s", url)
	last := len(r.FormValue("last")) > 0
	f := Feed{Url: url}
	if err := gn.Get(&f); err == datastore.ErrNoSuchEntity {
		c.Errorf("no such entity")
		return
	} else if err != nil {
		return
	} else if last {
		// noop
	} else if time.Now().Before(f.NextUpdate) {
		c.Infof("feed %v already updated: %v", url, f.NextUpdate)
		return
	}
	f.Subscribe(c)

	feedError := func(err error) {
		f.Errors++
		v := f.Errors + 1
		const max = 24 * 7
		if v > max {
			v = max
		} else if f.Errors == 1 {
			v = 0
		}
		f.NextUpdate = time.Now().Add(time.Hour * time.Duration(v))
		gn.Put(&f)
		c.Warningf("error with %v (%v), bump next update to %v, %v", url, f.Errors, f.NextUpdate, err)
	}

	if feed, stories, err := fetchFeed(c, f.Url, f.Url); err == nil {
		if err := updateFeed(c, f.Url, feed, stories, false, false, last); err != nil {
			feedError(err)
		}
	} else {
		feedError(err)
	}
}

func UpdateFeedLast(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	url := r.FormValue("feed")
	c.Debugf("update feed last %s", url)
	f := Feed{Url: url}
	if err := gn.Get(&f); err != nil {
		return
	}
	f.LastViewed = time.Now()
	gn.Put(&f)
}

func CFixer(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	q := datastore.NewQuery("F").KeysOnly()
	q = q.Limit(1000)
	cs := r.FormValue("c")
	if len(cs) > 0 {
		if cur, err := datastore.DecodeCursor(cs); err == nil {
			q = q.Start(cur)
			c.Infof("starting at %v", cur)
		} else {
			c.Errorf("cursor error %v", err.Error())
		}
	}
	var keys []*datastore.Key
	it := q.Run(appengine.Timeout(c, time.Second*15))
	for {
		k, err := it.Next(nil)
		if err == datastore.Done {
			break
		} else if err != nil {
			c.Errorf("next error: %v", err.Error())
			break
		}
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		c.Errorf("no results")
		return
	} else {
		cur, err := it.Cursor()
		if err != nil {
			c.Errorf("to cur error %v", err.Error())
		} else {
			c.Infof("add with cur %v", cur)
			t := taskqueue.NewPOSTTask("/tasks/cfixer", url.Values{
				"c": {cur.String()},
			})
			taskqueue.Add(c, t, "cfixer")
		}
	}
	c.Infof("fixing %d feeds", len(keys))

	var tasks []*taskqueue.Task
	for _, k := range keys {
		c.Infof("f: %v", k.StringID())
		tasks = append(tasks, taskqueue.NewPOSTTask("/tasks/cfix", url.Values{
			"feed": {k.StringID()},
		}))
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
		if _, err := taskqueue.AddMulti(c, ts, "cfixer"); err != nil {
			c.Errorf("taskqueue error: %v", err.Error())
		}
	}
}

func CFix(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	url := r.FormValue("feed")
	c.Infof("fix feed %s", url)
	f := Feed{Url: url}
	if err := gn.Get(&f); err != nil {
		c.Criticalf("cfix err: %v", err)
		serveError(w, err)
		return
	}

	q := datastore.NewQuery("S").Ancestor(gn.Key(&f))
	var ss []*Story
	keys, err := q.GetAll(c, &ss)
	if err != nil {
		c.Errorf("getall err: %v", err)
		serveError(w, err)
		return
	}
	c.Infof("trying to fix %v stories", len(ss))
	const putLimit = 500
	for i := 0; i <= len(keys)/putLimit; i++ {
		lo := i * putLimit
		hi := (i + 1) * putLimit
		if hi > len(keys) {
			hi = len(keys)
		}
		c.Infof("%v - %v", lo, hi)
		if _, err := datastore.PutMulti(c, keys[lo:hi], ss[lo:hi]); err != nil {
			c.Errorf("err: %v, %v, %v", lo, hi, err)
		}
	}
}
