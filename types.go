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
	"fmt"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/mjibson/goread/_third_party/github.com/mjibson/goon"

	"appengine"
	"appengine/datastore"
	"appengine/taskqueue"
	"appengine/user"
)

type User struct {
	_kind    string    `goon:"kind,U"`
	Id       string    `datastore:"-" goon:"id"`
	Email    string    `datastore:"e"`
	Messages []string  `datastore:"m,noindex"`
	Read     time.Time `datastore:"r"`
	Options  string    `datastore:"o,noindex"`
	Account  int       `datastore:"a"`
	Created  time.Time `datastore:"d"`
	Until    time.Time `datastore:"u"`
}

const (
	AFree = iota
	ADev
	APaid
)

func (u *User) String() string {
	return u.Email
}

// parent: User, key: "data"
type UserData struct {
	_kind  string         `goon:"kind,UD"`
	Id     string         `datastore:"-" goon:"id"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`
	Opml   []byte         `datastore:"o,noindex"`
	Read   []byte         `datastore:"r,noindex"`
}

// parent: User, key: time.Now().UnixNano()
type UserOpml struct {
	_kind      string         `goon:"kind,UO"`
	Id         int64          `datastore:"-" goon:"id"`
	Parent     *datastore.Key `datastore:"-" goon:"parent"`
	Opml       []byte         `datastore:"o,noindex"`
	Compressed []byte         `datastore:"z,noindex"`
}

func (uo *UserOpml) opml() []byte {
	if len(uo.Compressed) > 0 {
		buf := bytes.NewReader(uo.Compressed)
		if gz, err := gzip.NewReader(buf); err == nil {
			defer gz.Close()
			if b, _ := ioutil.ReadAll(gz); err == nil {
				return b
			}
		}
	}
	return uo.Opml
}

type UserStarFeed struct {
	_kind  string         `goon:"kind,USF"`
	Id     string         `datastore:"-" goon:"id"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`
}

// parent: UserStarFeed, key: Story.Key.Encode()
type UserStar struct {
	_kind   string         `goon:"kind,US"`
	Id      string         `datastore:"-" goon:"id"`
	Parent  *datastore.Key `datastore:"-" goon:"parent"`
	Created time.Time      `datastore:"c"`
}

func starKey(c appengine.Context, feed, story string) *UserStar {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uk := gn.Key(&u)
	return &UserStar{
		Parent: datastore.NewKey(c, "USF", feed, 0, uk),
		Id:     story,
	}
}

func starID(key *datastore.Key) string {
	return fmt.Sprintf("%s|%s", key.Parent().StringID(), key.StringID())
}

type readStory struct {
	Feed, Story string
}

type Read map[readStory]bool

type Feed struct {
	_kind      string        `goon:"kind,F"`
	Url        string        `datastore:"-" goon:"id"`
	Title      string        `datastore:"t,noindex"`
	Updated    time.Time     `datastore:"u,noindex" json:"-"`
	Date       time.Time     `datastore:"d,noindex" json:"-"`
	Checked    time.Time     `datastore:"c,noindex"`
	NextUpdate time.Time     `datastore:"n"`
	Link       string        `datastore:"l,noindex" json:"-"`
	Hub        string        `datastore:"h,noindex" json:"-"`
	Errors     int           `datastore:"e,noindex"`
	Image      string        `datastore:"i,noindex"`
	ImageDate  time.Time     `datastore:"g,noindex"`
	Subscribed time.Time     `datastore:"s,noindex" json:"-"`
	Average    time.Duration `datastore:"a,noindex" json:"-"`
	LastViewed time.Time     `datastore:"v" json:"-"`
	NoAds      bool          `datastore:"o,noindex" json:"-"`
}

func (f *Feed) Subscribe(c appengine.Context) {
	if !f.IsSubscribed() {
		gn := goon.FromContext(c)
		gn.Put(&Log{
			Parent: gn.Key(&f),
			Id:     time.Now().UnixNano(),
			Text:   fmt.Sprintf("Subscribe %v", f.Subscribed.String()),
		})
		t := taskqueue.NewPOSTTask(routeUrl("subscribe-feed"), url.Values{
			"feed": {f.Url},
		})
		if _, err := taskqueue.Add(c, t, "update-manual"); err != nil {
			c.Errorf("taskqueue error: %v", err.Error())
		} else {
			c.Warningf("subscribe feed: %v", f.Url)
		}
	}
}

func (f *Feed) IsSubscribed() bool {
	return !ENABLE_PUBSUBHUBBUB || f.Hub == "" || time.Now().Before(f.Subscribed)
}

func (f *Feed) PubSubURL() string {
	b := base64.URLEncoding.EncodeToString([]byte(f.Url))
	ru, _ := router.Get("subscribe-callback").URL()
	ru.Scheme = "http"
	ru.Host = PUBSUBHUBBUB_HOST
	ru.RawQuery = url.Values{
		"feed": {b},
	}.Encode()
	return ru.String()
}

func (f *Feed) NotViewed() bool {
	return time.Since(f.LastViewed) > notViewedDisabled
}

// parent: Feed, key: story ID
type Story struct {
	_kind        string         `goon:"kind,S"`
	Id           string         `datastore:"-" goon:"id"`
	Parent       *datastore.Key `datastore:"-" goon:"parent" json:"-"`
	Title        string         `datastore:"t,noindex"`
	Link         string         `datastore:"l,noindex"`
	Created      time.Time      `datastore:"c"`
	Published    time.Time      `datastore:"p,noindex" json:"-"`
	Updated      time.Time      `datastore:"u,noindex" json:"-"`
	Date         int64          `datastore:"e,noindex"`
	Author       string         `datastore:"a,noindex" json:",omitempty"`
	Summary      string         `datastore:"s,noindex"`
	MediaContent string         `datastore:"m,noindex" json:",omitempty"`

	content string
}

const IDX_COL = "c"

// parent: Story, key: 1
type StoryContent struct {
	_kind      string         `goon:"kind,SC"`
	Id         int64          `datastore:"-" goon:"id"`
	Parent     *datastore.Key `datastore:"-" goon:"parent"`
	Content    string         `datastore:"c,noindex"`
	Compressed []byte         `datastore:"z,noindex"`
}

func (sc *StoryContent) content() string {
	if len(sc.Compressed) > 0 {
		buf := bytes.NewReader(sc.Compressed)
		if gz, err := gzip.NewReader(buf); err == nil {
			defer gz.Close()
			if b, _ := ioutil.ReadAll(gz); err == nil {
				return string(b)
			}
		}
	}
	return sc.Content
}

type OpmlOutline struct {
	Outline []*OpmlOutline `xml:"outline" json:",omitempty"`
	Title   string         `xml:"title,attr,omitempty" json:",omitempty"`
	XmlUrl  string         `xml:"xmlUrl,attr,omitempty" json:",omitempty"`
	Type    string         `xml:"type,attr,omitempty" json:",omitempty"`
	Text    string         `xml:"text,attr,omitempty" json:",omitempty"`
	HtmlUrl string         `xml:"htmlUrl,attr,omitempty" json:",omitempty"`
}

type Opml struct {
	XMLName string         `xml:"opml"`
	Version string         `xml:"version,attr"`
	Title   string         `xml:"head>title"`
	Outline []*OpmlOutline `xml:"body>outline"`
}

type Image struct {
	_kind string            `goon:"kind,I"`
	Id    string            `datastore:"-" goon:"id"`
	Blob  appengine.BlobKey `datastore:"b,noindex"`
	Url   string            `datastore:"u,noindex"`
}

type Stories []*Story

func (s Stories) Len() int           { return len(s) }
func (s Stories) Less(i, j int) bool { return s[i].Created.Before(s[j].Created) }
func (s Stories) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type Log struct {
	_kind  string         `goon:"kind,L"`
	Id     int64          `datastore:"-" goon:"id"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`
	Text   string         `datastore:"t,noindex"`
}
