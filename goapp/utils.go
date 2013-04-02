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
	"appengine/user"
	"bytes"
	"code.google.com/p/rsc/blog/atom"
	"encoding/xml"
	"errors"
	"fmt"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"github.com/mjibson/rssgo"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

type Includes struct {
	Angular      string
	BootstrapCss string
	BootstrapJs  string
	Jquery       string
	MiniProfiler template.HTML
	User         *User
	Messages     []string
}

var (
	Angular      string
	BootstrapCss string
	BootstrapJs  string
	Jquery       string
)

func init() {
	angular_ver := "1.0.5"
	bootstrap_ver := "2.3.1"
	jquery_ver := "1.9.1"

	if appengine.IsDevAppServer() {
		Angular = fmt.Sprintf("/static/js/angular-%v.js", angular_ver)
		BootstrapCss = fmt.Sprintf("/static/css/bootstrap-%v.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("/static/js/bootstrap-%v.js", bootstrap_ver)
		Jquery = fmt.Sprintf("/static/js/jquery-%v.js", jquery_ver)
	} else {
		Angular = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/angularjs/%v/angular.min.js", angular_ver)
		BootstrapCss = fmt.Sprintf("//netdna.bootstrapcdn.com/twitter-bootstrap/%v/css/bootstrap-combined.min.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("//netdna.bootstrapcdn.com/twitter-bootstrap/%v/js/bootstrap.min.js", bootstrap_ver)
		Jquery = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/jquery/%v/jquery.min.js", jquery_ver)
	}
}

func includes(c mpg.Context) *Includes {
	i := &Includes{
		Angular:      Angular,
		BootstrapCss: BootstrapCss,
		BootstrapJs:  BootstrapJs,
		Jquery:       Jquery,
		MiniProfiler: c.P.Includes(),
	}

	if u := user.Current(c); u != nil {
		gn := goon.FromContext(c)
		user := new(User)
		if ue, err := gn.GetById(user, u.ID, 0, nil); err == nil && !ue.NotFound {
			i.User = user

			if len(user.Messages) > 0 {
				i.Messages = user.Messages
				user.Messages = nil
				gn.Put(ue)
			}
		}
	}

	return i
}

var dateFormats = []string{
	"2006-01-02",
	"2006-01-02T15:04:05-07:00",
	"Mon, 2 Jan 2006, 15:04 -0700",
	"Mon, 2 January 2006, 15:04 -0700",
	time.ANSIC,
	time.RubyDate,
	time.UnixDate,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
}

func parseDate(c appengine.Context, ds ...string) (t time.Time, err error) {
	for _, d := range ds {
		if d == "" {
			continue
		}
		if t, err = rssgo.ParseRssDate(d); err == nil {
			return
		}
		for _, f := range dateFormats {
			if t, err = time.Parse(f, d); err == nil {
				return
			}
		}
		c.Errorf("could not parse date: %v", d)
	}
	err = errors.New(fmt.Sprintf("could not parse date: %v", strings.Join(ds, ", ")))
	return
}

func ParseFeed(c appengine.Context, b []byte) (*Feed, []*Story) {
	f := Feed{}
	var s []*Story

	a := atom.Feed{}
	var atomerr, rsserr, rdferr error
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = CharsetReader
	if atomerr = d.Decode(&a); atomerr == nil {
		f.Title = a.Title
		if t, err := parseDate(c, string(a.Updated)); err == nil {
			f.Updated = t
		}
		for _, l := range a.Link {
			if l.Rel != "self" {
				f.Link = l.Href
				break
			}
		}

		for _, i := range a.Entry {
			st := Story{
				Id:    i.ID,
				Title: i.Title,
			}
			if t, err := parseDate(c, string(i.Updated)); err == nil {
				st.Updated = t
			}
			if len(i.Link) > 0 {
				st.Link = i.Link[0].Href
			}
			if i.Author != nil {
				st.Author = i.Author.Name
			}
			if i.Content != nil {
				st.content, st.Summary = Sanitize(i.Content.Body)
			} else if i.Summary != nil {
				st.content, st.Summary = Sanitize(i.Summary.Body)
			}
			s = append(s, &st)
		}

		return parseFix(&f, s)
	}

	r := rssgo.Rss{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = CharsetReader
	if rsserr = d.Decode(&r); rsserr == nil {
		f.Title = r.Title
		f.Link = r.Link
		if t, err := parseDate(c, r.LastBuildDate, r.PubDate); err == nil {
			f.Updated = t
		}

		for _, i := range r.Items {
			st := Story{
				Link:   i.Link,
				Author: i.Author,
			}
			if i.Title != "" {
				st.Title = i.Title
			} else if i.Description != "" {
				i.Title = i.Description
			}
			if i.Content != "" {
				st.content, st.Summary = Sanitize(i.Content)
			} else if i.Title != "" && i.Description != "" {
				st.content, st.Summary = Sanitize(i.Description)
			}
			if i.Guid != nil {
				st.Id = i.Guid.Guid
			} else {
				st.Id = i.Title
			}
			if t, err := parseDate(c, i.PubDate, i.Date, i.Published); err == nil {
				st.Updated = t
			}

			s = append(s, &st)
		}

		return parseFix(&f, s)
	}

	rdf := RDF{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = CharsetReader
	if rdferr = d.Decode(&rdf); rdferr == nil {
		if rdf.Channel != nil {
			f.Title = rdf.Channel.Title
			f.Link = rdf.Channel.Link
			if t, err := parseDate(c, rdf.Channel.Date); err == nil {
				f.Updated = t
			}
		}

		for _, i := range rdf.Item {
			st := Story{
				Id:     i.About,
				Title:  i.Title,
				Link:   i.Link,
				Author: i.Creator,
			}
			st.content, st.Summary = Sanitize(html.UnescapeString(i.Description))
			if i.About == "" && i.Link != "" {
				st.Id = i.Link
			} else if i.About == "" && i.Link == "" {
				c.Errorf("rdf error, no story id: %v", i)
				return nil, nil
			}
			if t, err := parseDate(c, i.Date); err == nil {
				st.Updated = t
			}
			s = append(s, &st)
		}

		return parseFix(&f, s)
	}

	c.Errorf("atom parse error: %s", atomerr.Error())
	c.Errorf("xml parse error: %s", rsserr.Error())
	c.Errorf("rdf parse error: %s", rdferr.Error())
	return nil, nil
}

const UpdateTime = time.Hour

func parseFix(f *Feed, ss []*Story) (*Feed, []*Story) {
	f.Checked = time.Now()
	f.NextUpdate = f.Checked.Add(UpdateTime)

	for _, s := range ss {
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.Id); err == nil {
				s.Link = u.String()
			}
		}
	}

	return f, ss
}
