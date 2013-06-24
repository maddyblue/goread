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
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

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
	f := Feed{Url: r.URL.Query().Get("f")}
	gn.Get(&f)
	q := datastore.NewQuery(gn.Key(&Story{}).Kind()).KeysOnly()
	fk := gn.Key(&f)
	q = q.Ancestor(fk)
	q = q.Filter("p >", time.Now().Add(time.Hour*-48))
	q = q.Order("-p")
	keys, _ := gn.GetAll(q, nil)
	stories := make([]*Story, len(keys))
	for j, key := range keys {
		stories[j] = &Story{
			Id:     key.StringID(),
			Parent: fk,
		}
	}
	gn.GetMulti(stories)

	templates.ExecuteTemplate(w, "admin-feed.html", struct {
		Feed    *Feed
		Stories []*Story
		Now     time.Time
	}{
		&f,
		stories,
		time.Now(),
	})
}

func AdminUpdateFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("f")
	if feed, stories := fetchFeed(c, url, url); feed != nil {
		updateFeed(c, url, feed, stories)
		fmt.Fprintf(w, "updated: %v", url)
	} else {
		fmt.Fprintf(w, "error updating: %v", url)
	}
}

func AdminDateFormats(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(gn.Key(&DateFormat{}).Kind()).KeysOnly()
	keys, err := gn.GetAll(q, nil)
	if err != nil {
		serveError(w, err)
		return
	}
	if err := templates.ExecuteTemplate(w, "admin-date-formats.html", keys); err != nil {
		serveError(w, err)
	}
}
