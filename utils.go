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
	"fmt"
	"html"
	"html/template"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"github.com/mjibson/goread/atom"
	"github.com/mjibson/goread/rdf"
	"github.com/mjibson/goread/rss"
	"github.com/mjibson/goread/sanitizer"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"appengine"
	"appengine/memcache"
	"appengine/taskqueue"
	"appengine/urlfetch"
	"appengine/user"
)

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

type Includes struct {
	Angular             string
	BootstrapCss        string
	BootstrapJs         string
	FontAwesome         string
	Jquery              string
	JqueryUI            string
	Underscore          string
	MiniProfiler        template.HTML
	User                *User
	Messages            []string
	GoogleAnalyticsId   string
	GoogleAnalyticsHost string
	SubURL              string
	IsDev               bool
	IsAdmin             bool
	StripeKey           string
	StripePlans         []Plan
}

var (
	Angular      string
	BootstrapCss string
	BootstrapJs  string
	FontAwesome  string
	Jquery       string
	JqueryUI     string
	Underscore   string
	isDevServer  bool
	subURL       string
)

func init() {
	angular_ver := "1.2.1"
	bootstrap_ver := "3.0.2"
	font_awesome_ver := "4.0.3"
	jquery_ver := "2.0.3"
	jqueryui_ver := "1.10.3.sortable"
	isDevServer = appengine.IsDevAppServer()

	if appengine.IsDevAppServer() {
		Angular = "/static/js/angular.js"
		BootstrapCss = "/static/css/bootstrap.css"
		BootstrapJs = "/static/js/bootstrap.js"
		FontAwesome = "/static/css/font-awesome.css"
		Jquery = fmt.Sprintf("/static/js/jquery-%v.js", jquery_ver)
		JqueryUI = fmt.Sprintf("/static/js/jquery-ui-%v.js", jqueryui_ver)
		Underscore = "/static/js/underscore.js"
	} else {
		Angular = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/angularjs/%v/angular.min.js", angular_ver)
		BootstrapCss = fmt.Sprintf("//netdna.bootstrapcdn.com/bootstrap/%v/css/bootstrap.min.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("//netdna.bootstrapcdn.com/bootstrap/%v/js/bootstrap.min.js", bootstrap_ver)
		FontAwesome = fmt.Sprintf("//netdna.bootstrapcdn.com/font-awesome/%v/css/font-awesome.min.css", font_awesome_ver)
		Jquery = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/jquery/%v/jquery.min.js", jquery_ver)
		JqueryUI = fmt.Sprintf("/static/js/jquery-ui-%v.min.js", jqueryui_ver)
		Underscore = "/static/js/underscore-min.js"
	}
}

func includes(c mpg.Context, w http.ResponseWriter, r *http.Request) *Includes {
	i := &Includes{
		Angular:             Angular,
		BootstrapCss:        BootstrapCss,
		BootstrapJs:         BootstrapJs,
		FontAwesome:         FontAwesome,
		Jquery:              Jquery,
		JqueryUI:            JqueryUI,
		Underscore:          Underscore,
		MiniProfiler:        c.Includes(),
		GoogleAnalyticsId:   GOOGLE_ANALYTICS_ID,
		GoogleAnalyticsHost: GOOGLE_ANALYTICS_HOST,
		SubURL:              subURL,
		IsDev:               isDevServer,
		StripeKey:           STRIPE_KEY,
		StripePlans:         STRIPE_PLANS,
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
	"01/02/2006",
	"01/02/2006 - 15:04",
	"01/02/2006 15:04:05 MST",
	"01/02/2006 3:04 PM",
	"02-01-2006",
	"02/01/2006",
	"02.01.2006 -0700",
	"02/01/2006 - 15:04",
	"02.01.2006 15:04",
	"02/01/2006 15:04:05",
	"02.01.2006 15:04:05",
	"02-01-2006 15:04:05 MST",
	"02/01/2006 15:04 MST",
	"02 Jan 2006",
	"02 Jan 2006 15:04:05",
	"02 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 MST",
	"02 Jan 2006 15:04:05 UT",
	"02 Jan 2006 15:04 MST",
	"02 Monday, Jan 2006 15:04",
	"06-1-2 15:04",
	"06/1/2 15:04",
	"1/2/2006",
	"1/2/2006 15:04:05 MST",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 3:04:05 PM MST",
	"15:04 02.01.2006 -0700",
	"2006-01-02",
	"2006/01/02",
	"2006-01-02 00:00:00.0 15:04:05.0 -0700",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05-0700",
	"2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05Z",
	"2006-01-02 at 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05:00",
	"2006-01-02T15:04:05 -0700",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05:-0700",
	"2006-01-02T15:04:05-07:00:00",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04-07:00",
	"2006-01-02T15:04Z",
	"2006-1-02T15:04:05Z",
	"2006-1-2",
	"2006-1-2 15:04:05",
	"2006-1-2T15:04:05Z",
	"2006 January 02",
	"2-1-2006",
	"2/1/2006",
	"2.1.2006 15:04:05",
	"2 Jan 2006",
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 Z",
	"2 January 2006",
	"2 January 2006 15:04:05 -0700",
	"2 January 2006 15:04:05 MST",
	"6-1-2 15:04",
	"6/1/2 15:04",
	"Jan 02, 2006",
	"Jan 02 2006 03:04:05PM",
	"Jan 2, 2006",
	"Jan 2, 2006 15:04:05 MST",
	"Jan 2, 2006 3:04:05 PM",
	"Jan 2, 2006 3:04:05 PM MST",
	"January 02, 2006",
	"January 02, 2006 03:04 PM",
	"January 02, 2006 15:04",
	"January 02, 2006 15:04:05 MST",
	"January 2, 2006",
	"January 2, 2006 03:04 PM",
	"January 2, 2006 15:04:05",
	"January 2, 2006 15:04:05 MST",
	"January 2, 2006, 3:04 p.m.",
	"January 2, 2006 3:04 PM",
	"Mon, 02 Jan 06 15:04:05 MST",
	"Mon, 02 Jan 2006",
	"Mon, 02 Jan 2006 15:04:05",
	"Mon, 02 Jan 2006 15:04:05 00",
	"Mon, 02 Jan 2006 15:04:05 -07",
	"Mon 02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 --0700",
	"Mon, 02 Jan 2006 15:04:05 -07:00",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon,02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 GMT-0700",
	"Mon , 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05MST",
	"Mon, 02 Jan 2006, 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 MST -0700",
	"Mon, 02 Jan 2006 15:04:05 MST-07:00",
	"Mon, 02 Jan 2006 15:04:05 UT",
	"Mon, 02 Jan 2006 15:04:05 Z",
	"Mon, 02 Jan 2006 15:04 -0700",
	"Mon, 02 Jan 2006 15:04 MST",
	"Mon,02 Jan 2006 15:04 MST",
	"Mon, 02 Jan 2006 15 -0700",
	"Mon, 02 Jan 2006 3:04:05 PM MST",
	"Mon, 02 January 2006",
	"Mon,02 January 2006 14:04:05 MST",
	"Mon, 2006-01-02 15:04",
	"Mon, 2 Jan 06 15:04:05 -0700",
	"Mon, 2 Jan 06 15:04:05 MST",
	"Mon, 2 Jan 15:04:05 MST",
	"Mon, 2 Jan 2006",
	"Mon,2 Jan 2006",
	"Mon, 2 Jan 2006 15:04",
	"Mon, 2 Jan 2006 15:04:05",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05-0700",
	"Mon, 2 Jan 2006 15:04:05 -0700 MST",
	"mon,2 Jan 2006 15:04:05 MST",
	"Mon 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05MST",
	"Mon, 2 Jan 2006 15:04:05 UT",
	"Mon, 2 Jan 2006 15:04 -0700",
	"Mon, 2 Jan 2006, 15:04 -0700",
	"Mon, 2 Jan 2006 15:04 MST",
	"Mon, 2, Jan 2006 15:4",
	"Mon, 2 Jan 2006 15:4:5 -0700 GMT",
	"Mon, 2 Jan 2006 15:4:5 MST",
	"Mon, 2 Jan 2006 3:04:05 PM -0700",
	"Mon, 2 January 2006",
	"Mon, 2 January 2006 15:04:05 -0700",
	"Mon, 2 January 2006 15:04:05 MST",
	"Mon, 2 January 2006, 15:04:05 MST",
	"Mon, 2 January 2006, 15:04 -0700",
	"Mon, 2 January 2006 15:04 MST",
	"Monday, 02 January 2006 15:04:05",
	"Monday, 02 January 2006 15:04:05 -0700",
	"Monday, 02 January 2006 15:04:05 MST",
	"Monday, 2 Jan 2006 15:04:05 -0700",
	"Monday, 2 Jan 2006 15:04:05 MST",
	"Monday, 2 January 2006 15:04:05 -0700",
	"Monday, 2 January 2006 15:04:05 MST",
	"Monday, January 02, 2006",
	"Monday, January 2, 2006",
	"Monday, January 2, 2006 03:04 PM",
	"Monday, January 2, 2006 15:04:05 MST",
	"Mon Jan 02 2006 15:04:05 -0700",
	"Mon, Jan 02,2006 15:04:05 MST",
	"Mon Jan 02, 2006 3:04 pm",
	"Mon Jan 2 15:04:05 2006 MST",
	"Mon Jan 2 15:04 2006",
	"Mon, Jan 2 2006 15:04:05 -0700",
	"Mon, Jan 2 2006 15:04:05 -700",
	"Mon, Jan 2, 2006 15:04:05 MST",
	"Mon, Jan 2 2006 15:04 MST",
	"Mon, Jan 2, 2006 15:04 MST",
	"Mon, January 02, 2006 15:04:05 MST",
	"Mon, January 02, 2006, 15:04:05 MST",
	"Mon, January 2 2006 15:04:05 -0700",
	time.ANSIC,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RubyDate,
	time.UnixDate,
	"Updated January 2, 2006",
}

const dateFormatCount = 500

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
		df := memcache.Item{
			Key:   fmt.Sprintf("_dateformat-%v", rand.Int63n(dateFormatCount)),
			Value: []byte(fmt.Sprintf("%v|%v", d, feed.Url)),
		}
		memcache.Add(c, &df)
	}
	err = fmt.Errorf("could not parse date: %v", strings.Join(ds, ", "))
	return
}

func encodingReader(body []byte, contentType string) (encoding.Encoding, error) {
	preview := make([]byte, 1024)
	var r io.Reader = bytes.NewReader(body)
	n, err := io.ReadFull(r, preview)
	switch {
	case err == io.ErrUnexpectedEOF:
		preview = preview[:n]
		r = bytes.NewReader(preview)
	case err != nil:
		return nil, err
	default:
		r = io.MultiReader(bytes.NewReader(preview), r)
	}

	e, _, certain := charset.DetermineEncoding(preview, contentType)
	if !certain && e == charmap.Windows1252 && utf8.Valid(body) {
		e = encoding.Nop
	}
	return e, nil
}

func defaultCharsetReader(cs string, input io.Reader) (io.Reader, error) {
	e, _ := charset.Lookup(cs)
	if e == nil {
		return nil, fmt.Errorf("cannot decode charset %v", cs)
	}
	return transform.NewReader(input, e.NewDecoder()), nil
}

func nilCharsetReader(cs string, input io.Reader) (io.Reader, error) {
	return input, nil
}

func ParseFeed(c appengine.Context, contentType, origUrl, fetchUrl string, body []byte) (*Feed, []*Story, error) {
	cr := defaultCharsetReader
	if !bytes.EqualFold(body[:len(xml.Header)], []byte(xml.Header)) {
		enc, err := encodingReader(body, contentType)
		if err != nil {
			return nil, nil, err
		}
		if enc != encoding.Nop {
			cr = nilCharsetReader
			body, err = ioutil.ReadAll(transform.NewReader(bytes.NewReader(body), enc.NewDecoder()))
			if err != nil {
				return nil, nil, err
			}
		}
	}
	var feed *Feed
	var stories []*Story
	var atomerr, rsserr, rdferr error
	feed, stories, atomerr = parseAtom(c, body, cr)
	if feed == nil {
		feed, stories, rsserr = parseRSS(c, body, cr)
	}
	if feed == nil {
		feed, stories, rdferr = parseRDF(c, body, cr)
	}
	if feed == nil {
		c.Warningf("atom parse error: %s", atomerr.Error())
		c.Warningf("xml parse error: %s", rsserr.Error())
		c.Warningf("rdf parse error: %s", rdferr.Error())
		return nil, nil, fmt.Errorf("Could not parse feed data")
	}
	feed.Url = origUrl
	return parseFix(c, feed, stories, fetchUrl)
}

func parseAtom(c appengine.Context, body []byte, charsetReader func(string, io.Reader) (io.Reader, error)) (*Feed, []*Story, error) {
	var f Feed
	var s []*Story
	var err error
	a := atom.Feed{}
	var fb, eb *url.URL
	d := xml.NewDecoder(bytes.NewReader(body))
	d.CharsetReader = charsetReader
	if err := d.Decode(&a); err != nil {
		return nil, nil, err
	}
	f.Title = a.Title
	if t, err := parseDate(c, &f, string(a.Updated)); err == nil {
		f.Updated = t
	}

	if fb, err = url.Parse(a.XMLBase); err != nil {
		fb, _ = url.Parse("")
	}
	if len(a.Link) > 0 {
		f.Link = findBestAtomLink(c, a.Link)
		if l, err := fb.Parse(f.Link); err == nil {
			f.Link = l.String()
		}
		for _, l := range a.Link {
			if l.Rel == "hub" {
				f.Hub = l.Href
				break
			}
		}
	}

	for _, i := range a.Entry {
		if eb, err = fb.Parse(i.XMLBase); err != nil {
			eb = fb
		}
		st := Story{
			Id:    i.ID,
			Title: atomTitle(i.Title),
		}
		if t, err := parseDate(c, &f, string(i.Updated)); err == nil {
			st.Updated = t
		}
		if t, err := parseDate(c, &f, string(i.Published)); err == nil {
			st.Published = t
		}
		if len(i.Link) > 0 {
			st.Link = findBestAtomLink(c, i.Link)
			if l, err := eb.Parse(st.Link); err == nil {
				st.Link = l.String()
			}
		}
		if i.Author != nil {
			st.Author = i.Author.Name
		}
		if i.Content != nil {
			if len(strings.TrimSpace(i.Content.Body)) != 0 {
				st.content = i.Content.Body
			} else if len(i.Content.InnerXML) != 0 {
				st.content = i.Content.InnerXML
			}
		} else if i.Summary != nil {
			st.content = i.Summary.Body
		}
		s = append(s, &st)
	}
	return &f, s, nil
}

func parseRSS(c appengine.Context, body []byte, charsetReader func(string, io.Reader) (io.Reader, error)) (*Feed, []*Story, error) {
	var f Feed
	var s []*Story
	r := rss.Rss{}
	d := xml.NewDecoder(bytes.NewReader(body))
	d.CharsetReader = charsetReader
	d.DefaultSpace = "DefaultSpace"
	if err := d.Decode(&r); err != nil {
		return nil, nil, err
	}
	f.Title = r.Title
	if t, err := parseDate(c, &f, r.LastBuildDate, r.PubDate); err == nil {
		f.Updated = t
	} else {
		c.Warningf("no rss feed date: %v", f.Link)
	}
	f.Link = r.BaseLink()
	f.Hub = r.Hub()

	for _, i := range r.Items {
		st := Story{
			Link:   i.Link,
			Author: i.Author,
		}
		if i.Content != "" {
			st.content = i.Content
		} else if i.Description != "" {
			st.content = i.Description
		}
		if i.Title != "" {
			st.Title = i.Title
		} else if i.Description != "" {
			st.Title = i.Description
		}
		if st.content == st.Title {
			st.Title = ""
		}
		st.Title = textTitle(st.Title)
		if i.Guid != nil {
			st.Id = i.Guid.Guid
		}
		if i.Enclosure != nil && strings.HasPrefix(i.Enclosure.Type, "audio/") {
			st.MediaContent = i.Enclosure.Url
		} else if i.Media != nil && strings.HasPrefix(i.Media.Type, "audio/") {
			st.MediaContent = i.Media.URL
		}
		if t, err := parseDate(c, &f, i.PubDate, i.Date, i.Published); err == nil {
			st.Published = t
			st.Updated = t
		}

		s = append(s, &st)
	}
	return &f, s, nil
}

func parseRDF(c appengine.Context, body []byte, charsetReader func(string, io.Reader) (io.Reader, error)) (*Feed, []*Story, error) {
	var f Feed
	var s []*Story
	rd := rdf.RDF{}
	d := xml.NewDecoder(bytes.NewReader(body))
	d.CharsetReader = charsetReader
	if err := d.Decode(&rd); err != nil {
		return nil, nil, err
	}
	if rd.Channel != nil {
		f.Title = rd.Channel.Title
		f.Link = rd.Channel.Link
		if t, err := parseDate(c, &f, rd.Channel.Date); err == nil {
			f.Updated = t
		}
	}

	for _, i := range rd.Item {
		st := Story{
			Id:     i.About,
			Title:  textTitle(i.Title),
			Link:   i.Link,
			Author: i.Creator,
		}
		if len(i.Description) > 0 {
			st.content = html.UnescapeString(i.Description)
		} else if len(i.Content) > 0 {
			st.content = html.UnescapeString(i.Content)
		}
		if t, err := parseDate(c, &f, i.Date); err == nil {
			st.Published = t
			st.Updated = t
		}
		s = append(s, &st)
	}
	return &f, s, nil
}

func textTitle(t string) string {
	return html.UnescapeString(t)
}

func atomTitle(t *atom.Text) string {
	if t == nil {
		return ""
	}
	if t.Type == "html" {
		return html.UnescapeString(sanitizer.StripTags(t.Body))
	}
	return textTitle(t.Body)
}

func findBestAtomLink(c appengine.Context, links []atom.Link) string {
	getScore := func(l atom.Link) int {
		switch {
		case l.Rel == "hub":
			return 0
		case l.Rel == "alternate" && l.Type == "text/html":
			return 5
		case l.Type == "text/html":
			return 4
		case l.Rel == "self":
			return 2
		case l.Rel == "":
			return 3
		default:
			return 1
		}
	}

	var bestlink string
	bestscore := -1
	for _, l := range links {
		score := getScore(l)
		if score > bestscore {
			bestlink = l.Href
			bestscore = score
		}
	}

	return bestlink
}

func parseFix(c appengine.Context, f *Feed, ss []*Story, fetchUrl string) (*Feed, []*Story, error) {
	g := goon.FromContext(c)
	f.Checked = time.Now()
	fk := g.Key(f)
	f.Link = strings.TrimSpace(f.Link)
	f.Title = html.UnescapeString(strings.TrimSpace(f.Title))

	if u, err := url.Parse(f.Url); err == nil {
		if ul, err := u.Parse(f.Link); err == nil {
			f.Link = ul.String()
		}
	}
	base, err := url.Parse(f.Link)
	if err != nil {
		c.Warningf("unable to parse link: %v", f.Link)
	}

	var nss []*Story
	for _, s := range ss {
		s.Parent = fk
		s.Created = f.Checked
		s.Link = strings.TrimSpace(s.Link)
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
				continue
			}
		}
		if fetchUrl == "http://blogs.msdn.com/rss.aspx" {
			s.Id = s.Link
		}
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.Id); err == nil {
				s.Link = u.String()
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
		const keySize = 500
		sk := g.Key(s)
		if kl := len(sk.String()); kl > keySize {
			c.Warningf("key too long: %v, %v, %v", kl, f.Url, s.Id)
			continue
		}
		su, serr := url.Parse(s.Link)
		if serr != nil {
			su = &url.URL{}
			s.Link = ""
		}
		const snipLen = 100
		s.content, s.Summary = sanitizer.Sanitize(s.content, su)
		s.Summary = sanitizer.SnipText(s.Summary, snipLen)
		nss = append(nss, s)
	}

	return f, nss, nil
}

func loadImage(c appengine.Context, f *Feed) {
	if f.ImageDate.After(time.Now()) {
		return
	}
	f.ImageDate = time.Now().Add(time.Hour * 24 * 7)
	s := f.Link
	if s == "" {
		s = f.Url
	}
	u, err := url.Parse(s)
	if err != nil {
		return
	}
	u.RawQuery = ""
	u.Fragment = ""
	p := "/favicon.ico"
	client := urlfetch.Client(c)
	if r, err := client.Get(u.String()); err == nil {
		b, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err == nil {
			i, err := FindIcon(b)
			if err == nil {
				p = i
			}
		}
	}
	u, err = u.Parse(p)
	if err != nil {
		return
	}
	us := u.String()
	r, err := client.Get(us)
	if err != nil || r.StatusCode != http.StatusOK || r.ContentLength == 0 {
		us = ""
	}
	f.Image = us
}

func updateAverage(f *Feed, previousUpdate time.Time, updateCount int) {
	if previousUpdate.IsZero() || updateCount < 1 {
		return
	}

	// if multiple updates occurred, assume they were evenly spaced
	interval := time.Since(previousUpdate) / time.Duration(updateCount)

	// rather than calculate a strict mean, we weight
	// each new interval, gradually decaying the influence
	// of older intervals
	old := float64(f.Average) * (1.0 - NewIntervalWeight)
	cur := float64(interval) * NewIntervalWeight
	f.Average = time.Duration(old + cur)
}

const notViewedDisabled = oldDuration + time.Hour*24*7

var timeMax time.Time = time.Date(3000, time.January, 1, 0, 0, 0, 0, time.UTC)

func scheduleNextUpdate(c appengine.Context, f *Feed) {
	loadImage(c, f)
	if f.NotViewed() {
		f.NextUpdate = timeMax
		return
	}

	now := time.Now()
	if f.Date.IsZero() {
		f.NextUpdate = now.Add(UpdateDefault)
		return
	}

	// calculate the delay until next check based on average time between updates
	pause := time.Duration(float64(f.Average) * UpdateFraction)

	// if we have never found an update, start with a default wait time
	if pause == 0 {
		pause = UpdateDefault
	}

	// if it has been much longer than expected since the last update,
	// gradually reduce the frequency of checks
	since := time.Since(f.Date)
	if since > pause*UpdateLongFactor {
		pause = time.Duration(float64(since) / UpdateLongFactor)
	}

	// enforce some limits
	if pause < UpdateMin {
		pause = UpdateMin
	}
	if pause > UpdateMax {
		pause = UpdateMax
	}

	// introduce a little random jitter to break up
	// convoys of updates
	jitter := time.Duration(rand.Int63n(int64(UpdateJitter)))
	if rand.Intn(2) == 0 {
		pause += jitter
	} else {
		pause -= jitter
	}
	f.NextUpdate = time.Now().Add(pause)
}

func taskSender(c mpg.Context, queue string, tc chan *taskqueue.Task, done chan bool) {
	const taskLimit = 100
	tasks := make([]*taskqueue.Task, 0, taskLimit)
	send := func() {
		taskqueue.AddMulti(c, tasks, queue)
		c.Infof("added %v tasks", len(tasks))
		tasks = tasks[0:0]
	}
	for t := range tc {
		tasks = append(tasks, t)
		if len(tasks) == taskLimit {
			send()
		}
	}
	if len(tasks) > 0 {
		send()
	}
	done <- true
}
