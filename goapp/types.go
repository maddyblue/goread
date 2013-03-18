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
	"time"
)

// key: ID
type User struct {
	Email string `datastore:"e,noindex"`
}

func (u *User) String() string {
	return u.Email
}

// parent: User, key: "data"
type UserData struct {
	Feeds []byte `datastore:"f,noindex"`
}

type UserFeed struct {
	Url    string
	Title  string
	Link   string
	Label  string
	Sortid string
}

type Feeds []*UserFeed

// key: URL
type Feed struct {
	Title   string    `datastore:"t,noindex"`
	Updated time.Time `datastore:"u"`
	Link    string    `datastore:"l,noindex"`
}

// parent: Feed, key: "index"
type FeedIndex struct {
	Users []string `datastore:"u"`
}

// parent: Feed, key: story ID
type Story struct {
	id        string
	Title     string    `datastore:"t,noindex"`
	Link      string    `datastore:"l,noindex"`
	Published time.Time `datastore:"d,noindex"`
	Updated   time.Time `datastore:"u"`
	Date      int64     `datastore:"e,noindex"`
	Author    string    `datastore:"a,noindex"`
	Summary   string    `datastore:"i,noindex"`
	Content   string    `datastore:"c,noindex"`
}

// parent: Story, key: "index"
type StoryIndex struct {
	Users []string `datastore:"u"`
}

type FeedList map[string]*FeedData

type FeedData struct {
	Feed    *UserFeed
	Stories []*Story
}
