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
	"net/http"

	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
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
	if err := gn.GetMulti(fs); err != nil {
		serveError(w, err)
		return
	}
	b := feedsToOpml(fs)
	w.Write(b)
}

func feedsToOpml(feeds []*Feed) []byte {
	opml := Opml{}
	opml.Outline = make([]outline, len(feeds))
	for i, f := range feeds {
		opml.Outline[i] = outline{
			XmlUrl:  f.Url,
			Type:    "rss",
			Text:    f.Title,
			Title:   f.Title,
			HtmlUrl: f.Link,
		}
	}
	b, _ := xml.Marshal(&opml)
	return b
}
