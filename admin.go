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
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"appengine/datastore"

	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
)

func AllFeedsOpml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(gn.Key(&Feed{}).Kind()).KeysOnly()
	keys, _ := gn.GetAll(q, nil)
	fs := make([]*Feed, len(keys))
	for i, k := range keys {
		fs[i] = &Feed{Url: k.StringID()}
	}
	b := feedsToOpml(fs)
	w.Header().Add("Content-Type", "text/xml")
	w.Header().Add("Content-Disposition", "attachment; filename=all.opml")
	w.Write(b)
}

func feedsToOpml(feeds []*Feed) []byte {
	opml := Opml{Version: "1.0"}
	opml.Outline = make([]*OpmlOutline, len(feeds))
	for i, f := range feeds {
		opml.Outline[i] = &OpmlOutline{
			XmlUrl:  f.Url,
			Type:    "rss",
			Text:    f.Title,
			Title:   f.Title,
			HtmlUrl: f.Link,
		}
	}
	b, _ := xml.Marshal(&opml)
	b = append([]byte(`<?xml version="1.0" encoding="UTF-8"?>`), b...)
	return b
}

func AllFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(gn.Key(&Feed{}).Kind()).KeysOnly()
	keys, _ := gn.GetAll(q, nil)
	templates.ExecuteTemplate(w, "admin-all-feeds.html", keys)
}

func AdminFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	f := Feed{Url: r.FormValue("f")}
	if err := gn.Get(&f); err != nil {
		serveError(w, err)
		return
	}
	q := datastore.NewQuery(gn.Key(&Story{}).Kind()).KeysOnly()
	fk := gn.Key(&f)
	q = q.Ancestor(fk)
	q = q.Limit(100)
	q = q.Order("-" + IDX_COL)
	keys, _ := gn.GetAll(q, nil)
	stories := make([]*Story, len(keys))
	for j, key := range keys {
		stories[j] = &Story{
			Id:     key.StringID(),
			Parent: fk,
		}
	}
	gn.GetMulti(stories)
	q = datastore.NewQuery(gn.Key(&Log{}).Kind()).KeysOnly()
	q = q.Ancestor(fk)
	keys, _ = gn.GetAll(q, nil)
	logs := make([]*Log, len(keys))
	for j, key := range keys {
		logs[j] = &Log{
			Id:     key.IntID(),
			Parent: fk,
		}
	}
	gn.GetMulti(logs)
	f.Subscribe(c)

	templates.ExecuteTemplate(w, "admin-feed.html", struct {
		Feed    *Feed
		Logs    []*Log
		Stories []*Story
		Now     time.Time
	}{
		&f,
		logs,
		stories,
		time.Now(),
	})
}

func AdminUpdateFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("f")
	if feed, stories, err := fetchFeed(c, url, url); err == nil {
		updateFeed(c, url, feed, stories, true, false, false)
		fmt.Fprintf(w, "updated: %v", url)
	} else {
		fmt.Fprintf(w, "error updating %v: %v", url, err)
	}
}

func AdminDateFormats(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	var dfs []*DateFormat
	q := datastore.NewQuery(gn.Key(&DateFormat{}).Kind()).Limit(dateFormatCount)
	gn.GetAll(q, &dfs)
	if err := templates.ExecuteTemplate(w, "admin-date-formats.html", dfs); err != nil {
		serveError(w, err)
	}
}

func AdminStats(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	uc, _ := datastore.NewQuery(gn.Key(&User{}).Kind()).Count(c)
	templates.ExecuteTemplate(w, "admin-stats.html", struct {
		Users int
	}{
		uc,
	})
}

func AdminUser(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(gn.Key(&User{}).Kind()).Limit(1)
	q = q.Filter("e =", r.FormValue("u"))
	it := gn.Run(q)
	var u User
	ud := UserData{Id: "data"}
	var h []Log
	k, err := it.Next(&u)
	if err == nil {
		q = datastore.NewQuery(gn.Key(&Log{}).Kind()).Ancestor(k)
		_, err = gn.GetAll(q, &h)
		ud.Parent = gn.Key(&u)
		gn.Get(&ud)
	}
	if err := templates.ExecuteTemplate(w, "admin-user.html", struct {
		User  User
		Data  UserData
		Log   []Log
		Error error
	}{
		u,
		ud,
		h,
		err,
	}); err != nil {
		serveError(w, err)
	}
}
