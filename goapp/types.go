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
	"encoding/base64"
	"net/url"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/taskqueue"
)

type User struct {
	_kind    string    `goon:"kind,U"`
	Id       string    `datastore:"-" goon:"id"`
	Email    string    `datastore:"e,noindex"`
	Messages []string  `datastore:"m,noindex"`
	Read     time.Time `datastore:"r,noindex"`
	Options  string    `datastore:"o,noindex"`
	Account  int       `datastore:"a,noindex"`
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

type Read map[string][]string

type Feed struct {
	_kind      string        `goon:"kind,F"`
	Url        string        `datastore:"-" goon:"id"`
	Title      string        `datastore:"t,noindex"`
	Updated    time.Time     `datastore:"u,noindex"`
	Date       time.Time     `datastore:"d,noindex"`
	Checked    time.Time     `datastore:"c,noindex"`
	NextUpdate time.Time     `datastore:"n"`
	Link       string        `datastore:"l,noindex"`
	Errors     int           `datastore:"e,noindex"`
	Image      string        `datastore:"i,noindex"`
	Subscribed time.Time     `datastore:"s,noindex"`
	Average    time.Duration `datastore:"a,noindex"`
}

func (f Feed) Subscribe(c appengine.Context) {
	if !f.IsSubscribed() {
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

func (f Feed) IsSubscribed() bool {
	return !ENABLE_PUBSUBHUBBUB || time.Now().Before(f.Subscribed)
}

func (f Feed) PubSubURL() string {
	b := base64.URLEncoding.EncodeToString([]byte(f.Url))
	ru, _ := router.Get("subscribe-callback").URL("feed", b)
	ru.Scheme = "http"
	ru.Host = PUBSUBHUBBUB_HOST
	return ru.String()
}

// parent: Feed, key: story ID
type Story struct {
	_kind        string         `goon:"kind,S"`
	Id           string         `datastore:"-" goon:"id"`
	Parent       *datastore.Key `datastore:"-" goon:"parent" json:"-"`
	Title        string         `datastore:"t,noindex"`
	Link         string         `datastore:"l,noindex"`
	Created      time.Time      `datastore:"c" json:"-"`
	Published    time.Time      `datastore:"p" json:"-"`
	Updated      time.Time      `datastore:"u,noindex" json:"-"`
	Date         int64          `datastore:"e,noindex"`
	Author       string         `datastore:"a,noindex" json:",omitempty"`
	Summary      string         `datastore:"s,noindex"`
	MediaContent string         `datastore:"m,noindex" json:",omitempty"`

	content string
}

const IDX_COL = "p"

// parent: Story, key: 1
type StoryContent struct {
	_kind      string         `goon:"kind,SC"`
	Id         int64          `datastore:"-" goon:"id"`
	Parent     *datastore.Key `datastore:"-" goon:"parent"`
	Content    string         `datastore:"c,noindex"`
	Compressed []byte         `datastore:"z,noindex"`
}

type OpmlOutline struct {
	Outline []*OpmlOutline `xml:"outline" json:",omitempty"`
	Title   string         `xml:"title,attr,omitempty" json:",omitempty"`
	XmlUrl  string         `xml:"xmlUrl,attr" json:",omitempty"`
	Type    string         `xml:"type,attr,omitempty" json:",omitempty"`
	Text    string         `xml:"text,attr,omitempty" json:",omitempty"`
	HtmlUrl string         `xml:"htmlUrl,attr,omitempty" json:",omitempty"`
}

type Opml struct {
	XMLName string         `xml:"opml"`
	Version string         `xml:"version,attr"`
	Outline []*OpmlOutline `xml:"body>outline"`
}

type DateFormat struct {
	Id     string         `datastore:"-" goon:"id"`
	_kind  string         `goon:"kind,DF"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`
}

type Image struct {
	Id   string            `datastore:"-" goon:"id"`
	Blob appengine.BlobKey `datastore:"b,noindex"`
	Url  string            `datastore:"u,noindex"`
}
