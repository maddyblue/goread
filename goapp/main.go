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
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"appengine/datastore"
	"github.com/gorilla/mux"
	"github.com/mjibson/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
)

var router = new(mux.Router)
var templates *template.Template

func init() {
	var err error

	if templates, err = template.New("").Funcs(funcs).
		ParseFiles(
		"templates/base.html",
	); err != nil {
		log.Fatal(err)
	}

	router.Handle("/", mpg.NewHandler(Main)).Name("main")
	router.Handle("/login/google", mpg.NewHandler(LoginGoogle)).Name("login-google")
	router.Handle("/logout", mpg.NewHandler(Logout)).Name("logout")
	router.Handle("/oauth2callback", mpg.NewHandler(Oauth2Callback)).Name("oauth2callback")
	router.Handle("/tasks/import-reader", mpg.NewHandler(ImportReaderTask)).Name("import-reader-task")
	router.Handle("/tasks/import-opml", mpg.NewHandler(ImportOpmlTask)).Name("import-opml-task")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	router.Handle("/user/get-contents", mpg.NewHandler(GetContents)).Name("get-contents")
	router.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	router.Handle("/user/import/reader", mpg.NewHandler(ImportReader)).Name("import-reader")
	router.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	router.Handle("/user/mark-all-read", mpg.NewHandler(MarkAllRead)).Name("mark-all-read")
	router.Handle("/user/mark-read", mpg.NewHandler(MarkRead)).Name("mark-read")
	router.Handle("/user/clear-feeds", mpg.NewHandler(ClearFeeds)).Name("clear-feeds")
	http.Handle("/", router)

	miniprofiler.ShowControls = true
	miniprofiler.StartHidden = true
	miniprofiler.ToggleShortcut = "Alt+C"
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "base.html", includes(c)); err != nil {
		serveError(w, err)
	}
}

func addFeed(c mpg.Context, userid string, uf *UserFeed) error {
	gn := goon.FromContext(c)
	c.Infof("adding feed %s to user %s", uf.Url, userid)

	f := Feed{Url: uf.Url}
	if err := gn.Get(&f); err == datastore.ErrNoSuchEntity {
		if feed, stories := fetchFeed(c, uf.Url); feed == nil {
			return errors.New(fmt.Sprintf("could not add feed %s", uf.Url))
		} else {
			f = *feed
			f.Updated = time.Time{}
			f.Checked = f.Updated
			f.NextUpdate = f.Updated
			gn.Put(&f)
			if err := updateFeed(c, uf.Url, feed, stories); err != nil {
				return err
			}

			uf.Link = feed.Link
			if uf.Title == "" {
				uf.Title = feed.Title
			}
		}
	} else if err != nil {
		return err
	} else {
		uf.Link = f.Link
		if uf.Title == "" {
			uf.Title = f.Title
		}
	}

	return nil
}

func addUserFeed(ud *UserData, ufs ...*UserFeed) {
	var fs Feeds
	json.Unmarshal(ud.Feeds, &fs)
	for _, uf := range ufs {
		found := false
		for _, f := range fs {
			if f.Url == uf.Url {
				found = true
				break
			}
		}
		if !found {
			fs = append(fs, uf)
		}
	}
	ud.Feeds, _ = json.Marshal(&fs)
}
