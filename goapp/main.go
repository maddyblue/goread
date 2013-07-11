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

	"appengine"
	"appengine/datastore"
	"appengine_internal"
	"github.com/MiniProfiler/go/miniprofiler"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/gorilla/mux"
	"github.com/mjibson/goon"
)

var router = new(mux.Router)
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
	); err != nil {
		log.Fatal(err)
	}

	router.Handle("/", mpg.NewHandler(Main)).Name("main")
	router.Handle("/login/google", mpg.NewHandler(LoginGoogle)).Name("login-google")
	router.Handle("/logout", mpg.NewHandler(Logout)).Name("logout")
	router.Handle("/tasks/import-opml", mpg.NewHandler(ImportOpmlTask)).Name("import-opml-task")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/tasks/subscribe-feed", mpg.NewHandler(SubscribeFeed)).Name("subscribe-feed")
	router.Handle("/push/{feed}", mpg.NewHandler(SubscribeCallback)).Name("subscribe-callback")
	router.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	router.Handle("/user/clear-feeds", mpg.NewHandler(ClearFeeds)).Name("clear-feeds")
	router.Handle("/user/delete-account", mpg.NewHandler(DeleteAccount)).Name("delete-account")
	router.Handle("/user/export-opml", mpg.NewHandler(ExportOpml)).Name("export-opml")
	router.Handle("/user/get-contents", mpg.NewHandler(GetContents)).Name("get-contents")
	router.Handle("/user/get-feed", mpg.NewHandler(GetFeed)).Name("get-feed")
	router.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	router.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	router.Handle("/user/mark-all-read", mpg.NewHandler(MarkAllRead)).Name("mark-all-read")
	router.Handle("/user/mark-read", mpg.NewHandler(MarkRead)).Name("mark-read")
	router.Handle("/user/save-options", mpg.NewHandler(SaveOptions)).Name("save-options")
	router.Handle("/user/upload-opml", mpg.NewHandler(UploadOpml)).Name("upload-opml")

	router.Handle("/admin/all-feeds", mpg.NewHandler(AllFeeds)).Name("all-feeds")
	router.Handle("/admin/all-feeds-opml", mpg.NewHandler(AllFeedsOpml)).Name("all-feeds-opml")
	router.Handle("/admin/date-formats", mpg.NewHandler(AdminDateFormats)).Name("admin-date-formats")
	router.Handle("/admin/feed", mpg.NewHandler(AdminFeed)).Name("admin-feed")
	router.Handle("/admin/stats", mpg.NewHandler(AdminStats)).Name("admin-stats")
	router.Handle("/admin/update-feed", mpg.NewHandler(AdminUpdateFeed)).Name("admin-update-feed")
	router.Handle("/_ah/start", mpg.NewHandler(BackendStart))
	router.Handle("/_ah/stop", mpg.NewHandler(BackendStop))
	router.Handle("/tasks/cfixer", mpg.NewHandler(CFixer))
	router.Handle("/tasks/cfix", mpg.NewHandler(CFix))
	router.Handle("/user/charge", mpg.NewHandler(Charge)).Name("charge")
	router.Handle("/user/donate", mpg.NewHandler(Donate)).Name("donate")
	router.Handle("/user/account", mpg.NewHandler(Account)).Name("account")
	router.Handle("/user/uncheckout", mpg.NewHandler(Uncheckout)).Name("uncheckout")

	http.Handle("/", router)

	miniprofiler.ShowControls = true
	miniprofiler.StartHidden = true
	miniprofiler.ToggleShortcut = "Alt+C"
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "base.html", includes(c, w, r)); err != nil {
		c.Errorf("%v", err)
		serveError(w, err)
		return
	}
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
		if feed, stories := fetchFeed(c, o.XmlUrl, o.XmlUrl); feed == nil {
			return fmt.Errorf("could not add feed %s", o.XmlUrl)
		} else {
			f = *feed
			f.Updated = time.Time{}
			f.Checked = f.Updated
			f.NextUpdate = f.Updated
			gn.Put(&f)
			for _, s := range stories {
				s.Created = s.Published
			}
			if err := updateFeed(c, f.Url, feed, stories); err != nil {
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

func mergeUserOpml(ud *UserData, outlines ...*OpmlOutline) {
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

	ud.Opml, _ = json.Marshal(&fs)
}

// Below is from David Symonds; will be in next app engine release

// Timeout returns a replacement context that uses d as the default API RPC timeout.
func Timeout(c appengine.Context, d time.Duration) appengine.Context {
	return &timeoutContext{
		Context: c,
		d:       d,
	}
}

type timeoutContext struct {
	appengine.Context
	d time.Duration
}

func (t *timeoutContext) Call(service, method string, in, out appengine_internal.ProtoMessage, opts *appengine_internal.CallOptions) error {
	// Only affect calls that don't have a timeout.
	if opts == nil || opts.Timeout == 0 {
		newOpts := new(appengine_internal.CallOptions)
		if opts != nil {
			*newOpts = *opts
		}
		newOpts.Timeout = t.d
		opts = newOpts
	}
	return t.Context.Call(service, method, in, out, opts)
}
