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
	"encoding/xml"
	"errors"
	"fmt"
	"goapp/atom"
	"html"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"appengine"
	"appengine/blobstore"
	aimage "appengine/image"
	"appengine/urlfetch"
	"appengine/user"
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"github.com/mjibson/rssgo"
)

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

type Includes struct {
	Angular             string
	BootstrapCss        string
	BootstrapJs         string
	Jquery              string
	JqueryUI            string
	Underscore          string
	MiniProfiler        template.HTML
	User                *User
	Messages            []string
	GoogleAnalyticsId   string
	GoogleAnalyticsHost string
	IsDev               bool
	IsAdmin             bool
}

var (
	Angular      string
	BootstrapCss string
	BootstrapJs  string
	Jquery       string
	JqueryUI     string
	Underscore   string
	isDevServer  bool
)

func init() {
	angular_ver := "1.0.5"
	bootstrap_ver := "2.3.1"
	jquery_ver := "1.9.1"
	jqueryui_ver := "1.10.3"
	underscore_ver := "1.4.4"
	isDevServer = appengine.IsDevAppServer()

	if appengine.IsDevAppServer() {
		Angular = fmt.Sprintf("/static/js/angular-%v.js", angular_ver)
		BootstrapCss = fmt.Sprintf("/static/css/bootstrap-combined-%v.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("/static/js/bootstrap-%v.js", bootstrap_ver)
		Jquery = fmt.Sprintf("/static/js/jquery-%v.js", jquery_ver)
		JqueryUI = fmt.Sprintf("/static/js/jquery-ui-%v.js", jqueryui_ver)
		Underscore = fmt.Sprintf("/static/js/underscore-%v.js", underscore_ver)
	} else {
		Angular = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/angularjs/%v/angular.min.js", angular_ver)
		BootstrapCss = fmt.Sprintf("//netdna.bootstrapcdn.com/twitter-bootstrap/%v/css/bootstrap-combined.min.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("//netdna.bootstrapcdn.com/twitter-bootstrap/%v/js/bootstrap.min.js", bootstrap_ver)
		Jquery = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/jquery/%v/jquery.min.js", jquery_ver)
		JqueryUI = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/jqueryui/%v/jquery-ui.min.js", jqueryui_ver)
		Underscore = fmt.Sprintf("/static/js/underscore-%v.min.js", underscore_ver)
	}
}

func includes(c mpg.Context, w http.ResponseWriter, r *http.Request) *Includes {
	i := &Includes{
		Angular:             Angular,
		BootstrapCss:        BootstrapCss,
		BootstrapJs:         BootstrapJs,
		Jquery:              Jquery,
		JqueryUI:            JqueryUI,
		Underscore:          Underscore,
		MiniProfiler:        c.Includes(r),
		GoogleAnalyticsId:   GOOGLE_ANALYTICS_ID,
		GoogleAnalyticsHost: GOOGLE_ANALYTICS_HOST,
		IsDev:               isDevServer,
	}

	if cu := user.Current(c); cu != nil {
		gn := goon.FromContext(c)
		user := &User{Id: cu.ID}
		if err := gn.Get(user); err == nil {
			i.User = user
			i.IsAdmin = cu.Admin

			if len(user.Messages) > 0 {
				i.Messages = user.Messages
				user.Messages = nil
				gn.Put(user)
			}

			/*
				if _, err := r.Cookie("update-bug"); err != nil {
					i.Messages = append(i.Messages, "Go Read had some problems updating feeds. It may take a while for new stories to appear again. Sorry about that.")
					http.SetCookie(w, &http.Cookie{
						Name: "update-bug",
						Value: "done",
						Expires: time.Now().Add(time.Hour * 24 * 7),
					})
				}
			*/
		}
	}

	return i
}

var dateFormats = []string{
	"01-02-2006",
	"01/02/2006 15:04:05 MST",
	"02 Jan 2006 15:04 MST",
	"02 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 MST",
	"02 Jan 2006 15:04:05 UT",
	"02 Jan 2006",
	"02-01-2006 15:04:05 MST",
	"02.01.2006 15:04:05",
	"02/01/2006 15:04:05",
	"1/2/2006 15:04:05 MST",
	"1/2/2006 3:04:05 PM",
	"2 Jan 2006 15:04:05 MST",
	"2 January 2006 15:04:05 -0700",
	"2 Jan 2006",
	"2 January 2006",
	"2006 January 02",
	"2006-01-02 00:00:00.0 15:04:05.0 -0700",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02",
	"2006-01-02T15:04-07:00",
	"2006-01-02T15:04:05 -0700",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05-07:00:00",
	"2006-01-02T15:04:05:-0700",
	"2006-01-02T15:04:05Z",
	"2006-1-2 15:04:05",
	"2006-1-2",
	"6-1-2 15:04",
	"6/1/2 15:04",
	"Jan 02 2006 03:04:05PM",
	"Jan 2, 2006 15:04:05 MST",
	"Jan 2, 2006 3:04:05 PM MST",
	"January 02, 2006 03:04 PM",
	"January 02, 2006 15:04:05 MST",
	"January 02, 2006",
	"January 2, 2006 03:04 PM",
	"January 2, 2006 15:04:05 MST",
	"January 2, 2006 15:04:05",
	"Mon 02 Jan 2006 15:04:05 -0700",
	"Mon 2 Jan 2006 15:04:05 MST",
	"Mon Jan 2 15:04:05 2006 MST",
	"Mon, 02 Jan 06 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04 -0700",
	"Mon, 02 Jan 2006 15:04 MST",
	"Mon, 02 Jan 2006 15:04:05 --0700",
	"Mon, 02 Jan 2006 15:04:05 -07",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 -07:00",
	"Mon, 02 Jan 2006 15:04:05 MST -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 MST-07:00",
	"Mon, 02 Jan 2006 15:04:05 UT",
	"Mon, 02 Jan 2006 15:04:05 Z",
	"Mon, 02 Jan 2006 15:04:05",
	"Mon, 02 Jan 2006 15:04:05MST",
	"Mon, 02 Jan 2006 3:04:05 PM MST",
	"Mon, 02 Jan 2006",
	"Mon, 02 January 2006",
	"Mon, 2 Jan 06 15:04:05 -0700",
	"Mon, 2 Jan 06 15:04:05 MST",
	"Mon, 2 Jan 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05 UT",
	"Mon, 2 Jan 2006 15:04:05",
	"Mon, 2 Jan 2006 15:04:05-0700",
	"Mon, 2 Jan 2006 15:4:5 MST",
	"Mon, 2 Jan 2006",
	"Mon, 2 Jan 2006, 15:04 -0700",
	"Mon, 2 January 2006 15:04:05 -0700",
	"Mon, 2 January 2006 15:04:05 MST",
	"Mon, 2 January 2006, 15:04 -0700",
	"Mon, 2 January 2006, 15:04:05 MST",
	"Mon, Jan 2 2006 15:04:05 -0700",
	"Mon, Jan 2 2006 15:04:05 -700",
	"Mon, January 02, 2006, 15:04:05 MST",
	"Mon, January 2 2006 15:04:05 -0700",
	"Mon,02 Jan 2006 15:04:05 -0700",
	"Mon,02 January 2006 14:04:05 MST",
	"Monday, 02 January 2006 15:04:05 -0700",
	"Monday, 02 January 2006 15:04:05 MST",
	"Monday, 2 Jan 2006 15:04:05 -0700",
	"Monday, 2 Jan 2006 15:04:05 MST",
	"Monday, 2 January 2006 15:04:05 -0700",
	"Monday, 2 January 2006 15:04:05 MST",
	"Monday, January 02, 2006",
	"Monday, January 2, 2006 03:04 PM",
	"Monday, January 2, 2006 15:04:05 MST",
	"mon,2 Jan 2006 15:04:05 MST",
	time.ANSIC,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RubyDate,
	time.UnixDate,
}

func parseDate(c appengine.Context, feed *Feed, ds ...string) (t time.Time, err error) {
	for _, d := range ds {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		for _, f := range dateFormats {
			if t, err = time.Parse(f, d); err == nil {
				return
			}
		}
		gn := goon.FromContext(c)
		gn.Put(&DateFormat{
			Id:     d,
			Parent: gn.Key(feed),
		})
	}
	err = errors.New(fmt.Sprintf("could not parse date: %v", strings.Join(ds, ", ")))
	return
}

func ParseFeed(c appengine.Context, u string, b []byte) (*Feed, []*Story) {
	f := Feed{Url: u}
	var s []*Story

	a := atom.Feed{}
	var atomerr, rsserr, rdferr error
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	if atomerr = d.Decode(&a); atomerr == nil {
		f.Title = a.Title
		if t, err := parseDate(c, &f, string(a.Updated)); err == nil {
			f.Updated = t
		}

		if len(a.Link) > 0 {
			f.Link = findBestAtomLink(c, a.Link).Href
		}

		for _, i := range a.Entry {
			st := Story{
				Id:    i.ID,
				Title: i.Title,
			}
			if t, err := parseDate(c, &f, string(i.Updated)); err == nil {
				st.Updated = t
			}
			if t, err := parseDate(c, &f, string(i.Published)); err == nil {
				st.Published = t
			}
			if len(i.Link) > 0 {
				st.Link = findBestAtomLink(c, i.Link).Href
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

		return parseFix(c, &f, s)
	}

	r := rssgo.Rss{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	d.DefaultSpace = "DefaultSpace"
	if rsserr = d.Decode(&r); rsserr == nil {
		f.Title = r.Title
		f.Link = r.Link
		if t, err := parseDate(c, &f, r.LastBuildDate, r.PubDate); err == nil {
			f.Updated = t
		} else {
			c.Warningf("no rss feed date: %v", f.Link)
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
			}
			if t, err := parseDate(c, &f, i.PubDate, i.Date, i.Published); err == nil {
				st.Published = t
				st.Updated = t
			}

			s = append(s, &st)
		}

		return parseFix(c, &f, s)
	}

	rdf := RDF{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	if rdferr = d.Decode(&rdf); rdferr == nil {
		if rdf.Channel != nil {
			f.Title = rdf.Channel.Title
			f.Link = rdf.Channel.Link
			if t, err := parseDate(c, &f, rdf.Channel.Date); err == nil {
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
			if t, err := parseDate(c, &f, i.Date); err == nil {
				st.Published = t
				st.Updated = t
			}
			s = append(s, &st)
		}

		return parseFix(c, &f, s)
	}

	c.Warningf("atom parse error: %s", atomerr.Error())
	c.Warningf("xml parse error: %s", rsserr.Error())
	c.Warningf("rdf parse error: %s", rdferr.Error())
	return nil, nil
}

func findBestAtomLink(c appengine.Context, links []atom.Link) atom.Link {
	getScore := func(l atom.Link) int {
		switch {
		case l.Type == "text/html":
			return 3
		case l.Rel != "self":
			return 2
		default:
			return 1
		}
	}

	var bestlink atom.Link
	bestscore := -1
	for _, l := range links {
		score := getScore(l)
		if score > bestscore {
			bestlink = l
			bestscore = score
		}
	}

	return bestlink
}

const UpdateTime = time.Hour * 3

func parseFix(c appengine.Context, f *Feed, ss []*Story) (*Feed, []*Story) {
	g := goon.FromContext(c)
	f.Checked = time.Now()
	f.NextUpdate = f.Checked.Add(UpdateTime - time.Second*time.Duration(rand.Int63n(60*30)))
	fk := g.Key(f)
	f.Image = loadImage(c, f)

	base, err := url.Parse(f.Link)
	if err != nil {
		c.Warningf("unable to parse link: %v", f.Link)
	}

	for _, s := range ss {
		s.Parent = fk
		s.Created = f.Checked
		if !s.Updated.IsZero() && s.Published.IsZero() {
			s.Published = s.Updated
		}
		if s.Published.IsZero() || f.Checked.Before(s.Published) {
			s.Published = f.Checked
		}
		if !s.Updated.IsZero() {
			s.Date = s.Updated.Unix()
		} else {
			s.Date = s.Published.Unix()
		}
		if s.Id == "" {
			if s.Link != "" {
				s.Id = s.Link
			} else if s.Title != "" {
				s.Id = s.Title
			} else {
				c.Errorf("story has no id: %v", s)
				return nil, nil
			}
		}
		if base != nil && s.Link != "" {
			link, err := base.Parse(s.Link)
			if err == nil {
				s.Link = link.String()
			} else {
				c.Warningf("unable to resolve link: %v", s.Link)
			}
		}
		const keySize = 499
		if len(s.Id) > keySize { // datastore limits keys to 500 chars
			s.Id = s.Id[:keySize]
		}
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.Id); err == nil {
				s.Link = u.String()
			}
		}
	}

	return f, ss
}

func loadImage(c appengine.Context, f *Feed) string {
	s := f.Link
	if s == "" {
		s = f.Url
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	u.Path = "/favicon.ico"
	u.RawQuery = ""
	u.Fragment = ""

	g := goon.FromContext(c)
	i := &Image{Id: u.String()}
	if err := g.Get(i); err == nil {
		return i.Url
	}
	client := urlfetch.Client(c)
	r, err := client.Get(u.String())
	if err != nil || r.StatusCode != http.StatusOK || r.ContentLength == 0 {
		return ""
	}
	b, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return ""
	}
	buf := bytes.NewBuffer(b)
	_, t, err := image.DecodeConfig(buf)
	if err != nil {
		t = "application/octet-stream"
	} else {
		t = "image/" + t
	}
	w, err := blobstore.Create(c, t)
	if err != nil {
		return ""
	}
	if _, err := w.Write(b); err != nil {
		return ""
	}
	if w.Close() != nil {
		return ""
	}
	i.Blob, _ = w.Key()
	su, err := aimage.ServingURL(c, i.Blob, &aimage.ServingURLOptions{Size: 16})
	if err != nil {
		return ""
	}
	i.Url = su.String()
	g.Put(i)
	return i.Url
}
