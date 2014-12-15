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
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/MiniProfiler/go/miniprofiler"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/gorilla/mux"
	"github.com/mjibson/goon"

	"appengine"
	"appengine/datastore"
)

var router *mux.Router
var templates *template.Template

func init() {
	var err error
	if templates, err = template.New("").Funcs(funcs).
		ParseFiles(
		"templates/base.html",
		"templates/admin-all-feeds.html",
		"templates/admin-date-formats.html",
		"templates/admin-feed.html",
		"templates/admin-stats.html",
		"templates/admin-user.html",
	); err != nil {
		log.Fatal(err)
	}
	miniprofiler.ToggleShortcut = "Alt+C"
	miniprofiler.Position = "bottomleft"
}

func RegisterHandlers(r *mux.Router) {
	// This is ugly, but several code paths lead to routeUrl in funcs.go
	router = r

	r.Handle("/", mpg.NewHandler(Main)).Name("main")
	r.Handle("/login/google", mpg.NewHandler(LoginGoogle)).Name("login-google")
	r.Handle("/logout", mpg.NewHandler(Logout)).Name("logout")
	r.Handle("/push", mpg.NewHandler(SubscribeCallback)).Name("subscribe-callback")
	r.Handle("/tasks/import-opml", mpg.NewHandler(ImportOpmlTask)).Name("import-opml-task")
	r.Handle("/tasks/subscribe-feed", mpg.NewHandler(SubscribeFeed)).Name("subscribe-feed")
	r.Handle("/tasks/update-feed-last", mpg.NewHandler(UpdateFeedLast)).Name("update-feed-last")
	r.Handle("/tasks/update-feed-manual", mpg.NewHandler(UpdateFeed)).Name("update-feed-manual")
	r.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	r.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	r.Handle("/tasks/delete-old-feeds", mpg.NewHandler(DeleteOldFeeds)).Name("delete-old-feeds")
	r.Handle("/tasks/delete-old-feed", mpg.NewHandler(DeleteOldFeed)).Name("delete-old-feed")
	r.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	r.Handle("/user/delete-account", mpg.NewHandler(DeleteAccount)).Name("delete-account")
	r.Handle("/user/export-opml", mpg.NewHandler(ExportOpml)).Name("export-opml")
	r.Handle("/user/feed-history", mpg.NewHandler(FeedHistory)).Name("feed-history")
	r.Handle("/user/get-contents", mpg.NewHandler(GetContents)).Name("get-contents")
	r.Handle("/user/get-feed", mpg.NewHandler(GetFeed)).Name("get-feed")
	r.Handle("/user/get-stars", mpg.NewHandler(GetStars)).Name("get-stars")
	r.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	r.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	r.Handle("/user/mark-read", mpg.NewHandler(MarkRead)).Name("mark-read")
	r.Handle("/user/mark-unread", mpg.NewHandler(MarkUnread)).Name("mark-unread")
	r.Handle("/user/save-options", mpg.NewHandler(SaveOptions)).Name("save-options")
	r.Handle("/user/set-star", mpg.NewHandler(SetStar)).Name("set-star")
	r.Handle("/user/upload-opml", mpg.NewHandler(UploadOpml)).Name("upload-opml")

	r.Handle("/admin/all-feeds", mpg.NewHandler(AllFeeds)).Name("all-feeds")
	r.Handle("/admin/all-feeds-opml", mpg.NewHandler(AllFeedsOpml)).Name("all-feeds-opml")
	r.Handle("/admin/user", mpg.NewHandler(AdminUser)).Name("admin-user")
	r.Handle("/date-formats", mpg.NewHandler(AdminDateFormats)).Name("admin-date-formats")
	r.Handle("/admin/feed", mpg.NewHandler(AdminFeed)).Name("admin-feed")
	r.Handle("/admin/subhub", mpg.NewHandler(AdminSubHub)).Name("admin-subhub-feed")
	r.Handle("/admin/stats", mpg.NewHandler(AdminStats)).Name("admin-stats")
	r.Handle("/admin/update-feed", mpg.NewHandler(AdminUpdateFeed)).Name("admin-update-feed")
	r.Handle("/user/charge", mpg.NewHandler(Charge)).Name("charge")
	r.Handle("/user/account", mpg.NewHandler(Account)).Name("account")
	r.Handle("/user/uncheckout", mpg.NewHandler(Uncheckout)).Name("uncheckout")

	//r.Handle("/tasks/delete-blobs", mpg.NewHandler(DeleteBlobs)).Name("delete-blobs")

	// This doesn't really belong here, but it needs routeUrl.
	if len(PUBSUBHUBBUB_HOST) > 0 {
		u := url.URL{
			Scheme:   "http",
			Host:     PUBSUBHUBBUB_HOST,
			Path:     routeUrl("add-subscription"),
			RawQuery: url.Values{"url": {"{url}"}}.Encode(),
		}
		subURL = u.String()
	}

	if !isDevServer {
		return
	}
	r.Handle("/user/clear-feeds", mpg.NewHandler(ClearFeeds)).Name("clear-feeds")
	r.Handle("/user/clear-read", mpg.NewHandler(ClearRead)).Name("clear-read")
	r.Handle("/test/atom.xml", mpg.NewHandler(TestAtom)).Name("test-atom")
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "base.html", includes(c, w, r)); err != nil {
		c.Errorf("%v", err)
		serveError(w, err)
	}
	return
}

func addFeed(c mpg.Context, userid string, outline *OpmlOutline) error {
	gn := goon.FromContext(c)
	o := outline.Outline[0]
	c.Infof("adding feed %v to user %s", o.XmlUrl, userid)
	fu, ferr := url.Parse(o.XmlUrl)
	if ferr != nil {
		return ferr
	}
	fu.Fragment = ""
	o.XmlUrl = fu.String()

	f := Feed{Url: o.XmlUrl}
	if err := gn.Get(&f); err == datastore.ErrNoSuchEntity {
		if feed, stories, err := fetchFeed(c, o.XmlUrl, o.XmlUrl); err != nil {
			return fmt.Errorf("could not add feed %s: %v", o.XmlUrl, err)
		} else {
			f = *feed
			f.Updated = time.Time{}
			f.Checked = f.Updated
			f.NextUpdate = f.Updated
			f.LastViewed = time.Now()
			gn.Put(&f)
			for _, s := range stories {
				s.Created = s.Published
			}
			if err := updateFeed(c, f.Url, feed, stories, false, false, false); err != nil {
				return err
			}

			o.XmlUrl = feed.Url
			o.HtmlUrl = feed.Link
			if o.Title == "" {
				o.Title = feed.Title
			}
		}
	} else if err != nil {
		return err
	} else {
		o.HtmlUrl = f.Link
		if o.Title == "" {
			o.Title = f.Title
		}
	}
	o.Text = ""

	return nil
}

func mergeUserOpml(c appengine.Context, ud *UserData, outlines ...*OpmlOutline) error {
	var fs Opml
	json.Unmarshal(ud.Opml, &fs)
	urls := make(map[string]bool)

	for _, o := range fs.Outline {
		if o.XmlUrl != "" {
			urls[o.XmlUrl] = true
		} else {
			for _, so := range o.Outline {
				urls[so.XmlUrl] = true
			}
		}
	}

	mergeOutline := func(label string, outline *OpmlOutline) {
		if _, present := urls[outline.XmlUrl]; present {
			return
		} else {
			urls[outline.XmlUrl] = true

			if label == "" {
				fs.Outline = append(fs.Outline, outline)
			} else {
				done := false
				for _, ol := range fs.Outline {
					if ol.Title == label && ol.XmlUrl == "" {
						ol.Outline = append(ol.Outline, outline)
						done = true
						break
					}
				}
				if !done {
					fs.Outline = append(fs.Outline, &OpmlOutline{
						Title:   label,
						Outline: []*OpmlOutline{outline},
					})
				}
			}
		}
	}

	for _, outline := range outlines {
		if outline.XmlUrl != "" {
			mergeOutline("", outline)
		} else {
			for _, o := range outline.Outline {
				mergeOutline(outline.Title, o)
			}
		}
	}

	b, err := json.Marshal(&fs)
	if err != nil {
		return err
	}
	ud.Opml = b
	return nil
}
