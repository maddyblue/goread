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
	"sync"
	"time"

	"appengine/blobstore"
	"appengine/datastore"
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

func UpdateFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(gn.Key(&Feed{}).Kind()).KeysOnly()
	q = q.Filter("n <=", time.Now())

	//* iterator
	it := gn.Run(q)
	i := 0
	for {
		k, err := it.Next(nil)
		if err == datastore.Done {
			break
		} else if err != nil {
			c.Errorf("next error: %v", err.Error())
			break
		}
		t := taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {k.StringID()},
		})
		if _, err := taskqueue.Add(c, t, "update-feed"); err != nil {
			c.Errorf("taskqueue error: %v", err.Error())
		}
		i++
	}
	c.Infof("updating %d feeds", i)
	fmt.Fprintf(w, "updating %d feeds", i)
	//*/

	/* get all
	q = q.Limit(1000)
	keys, _ := gn.GetAll(q, nil)
	for _, k := range keys {
		t := taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {k.StringID()},
		})
		if _, err := taskqueue.Add(c, t, "update-feed"); err != nil {
			c.Errorf("taskqueue error: %v", err.Error())
		}
	}
	c.Infof("updating %d feeds", len(keys))
	fmt.Fprintf(w, "updating %d feeds", len(keys))
	//*/
}

func fetchFeed(c mpg.Context, origUrl, fetchUrl string) (*Feed, []*Story) {
	u, err := url.Parse(fetchUrl)
	if err == nil && u.Scheme == "" {
		u.Scheme = "http"
		origUrl = u.String()
		fetchUrl = origUrl
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
		gn.Put(&f)
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
		if !f.Date.IsZero() {
			// the feed has been updated, so update the average
			// interval between updates
			//
			// rather than calculate a strict mean, we weight
			// each new interval, gradually decaying the influence
			// of older intervals
			//
			// the real update could have happened any time since
			// our last check, so we treat it as being right in the middle
			interval := time.Since(f.Date) - f.Average/2
			if interval < UpdateMin/2 {
				interval = UpdateMin / 2
			}
			old := float64(f.Average) * (1.0 - NewIntervalWeight)
			cur := float64(interval) * NewIntervalWeight
			f.Average = time.Duration(old + cur)
		}
		f.Date = time.Now()
		if !hasUpdated {
			f.Updated = f.Date
		}
	}
	gn.PutMulti(puts)

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
	if feed, stories := fetchFeed(c, url, url); feed != nil {
		updateFeed(c, url, feed, stories)
	} else {
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
		c.Warningf("error with %v (%v), bump next update to %v", url, f.Errors, f.NextUpdate)
	}
}
