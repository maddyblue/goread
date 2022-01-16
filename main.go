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
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mjibson/goread/_third_party/github.com/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/goread/_third_party/github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goread/_third_party/github.com/gorilla/mux"
	"github.com/mjibson/goread/_third_party/github.com/mjibson/goon"

	"golang.org/x/net/context"
	"google.golang.org/appengine/v2/datastore"
	alog "google.golang.org/appengine/v2/log"
)

var (
	router      = new(mux.Router)
	templates   *template.Template
	mobileIndex []byte
)

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
	mobileIndex, err = ioutil.ReadFile("static/index.html")
	if err != nil {
		log.Fatal(err)
	}

	miniprofiler.ToggleShortcut = "Alt+C"
	miniprofiler.Position = "bottomleft"

	router := mux.NewRouter()
	RegisterHandlers(router)
	http.Handle("/", router)
}

func RegisterHandlers(r *mux.Router) {
	router = r
	router.Handle("/", mpg.NewHandler(Main)).Name("main")
	router.Handle("/login/google", mpg.NewHandler(LoginGoogle)).Name("login-google")
	router.Handle("/login/redirect", mpg.NewHandler(LoginRedirect))
	router.Handle("/logout", mpg.NewHandler(Logout)).Name("logout")
	router.Handle("/push", mpg.NewHandler(SubscribeCallback)).Name("subscribe-callback")
	router.Handle("/tasks/import-opml", mpg.NewHandler(ImportOpmlTask)).Name("import-opml-task")
	router.Handle("/tasks/subscribe-feed", mpg.NewHandler(SubscribeFeed)).Name("subscribe-feed")
	router.Handle("/tasks/update-feed-last", mpg.NewHandler(UpdateFeedLast)).Name("update-feed-last")
	router.Handle("/tasks/update-feed-manual", mpg.NewHandler(UpdateFeed)).Name("update-feed-manual")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/tasks/delete-old-feeds", mpg.NewHandler(DeleteOldFeeds)).Name("delete-old-feeds")
	router.Handle("/tasks/delete-old-feed", mpg.NewHandler(DeleteOldFeed)).Name("delete-old-feed")

	router.Handle("/user/add-subscription", wrap(AddSubscription)).Name("add-subscription")
	router.Handle("/user/delete-account", wrap(DeleteAccount)).Name("delete-account")
	router.Handle("/user/export-opml", wrap(ExportOpml)).Name("export-opml")
	router.Handle("/user/feed-history", wrap(FeedHistory)).Name("feed-history")
	router.Handle("/user/get-contents", wrap(GetContents)).Name("get-contents")
	router.Handle("/user/get-feed", wrap(GetFeed)).Name("get-feed")
	router.Handle("/user/get-stars", wrap(GetStars)).Name("get-stars")
	router.Handle("/user/import/get-url", wrap(UploadUrl)).Name("upload-url")
	router.Handle("/user/import/opml", wrap(ImportOpml)).Name("import-opml")
	router.Handle("/user/list-feeds", wrap(ListFeeds)).Name("list-feeds")
	router.Handle("/user/mark-read", wrap(MarkRead)).Name("mark-read")
	router.Handle("/user/mark-unread", wrap(MarkUnread)).Name("mark-unread")
	router.Handle("/user/save-options", wrap(SaveOptions)).Name("save-options")
	router.Handle("/user/set-star", wrap(SetStar)).Name("set-star")
	router.Handle("/user/upload-opml", wrap(UploadOpml)).Name("upload-opml")

	router.Handle("/admin/all-feeds", mpg.NewHandler(AllFeeds)).Name("all-feeds")
	router.Handle("/admin/all-feeds-opml", mpg.NewHandler(AllFeedsOpml)).Name("all-feeds-opml")
	router.Handle("/admin/user", mpg.NewHandler(AdminUser)).Name("admin-user")
	router.Handle("/date-formats", mpg.NewHandler(AdminDateFormats)).Name("admin-date-formats")
	router.Handle("/admin/feed", mpg.NewHandler(AdminFeed)).Name("admin-feed")
	router.Handle("/admin/subhub", mpg.NewHandler(AdminSubHub)).Name("admin-subhub-feed")
	router.Handle("/admin/stats", mpg.NewHandler(AdminStats)).Name("admin-stats")
	router.Handle("/admin/update-feed", mpg.NewHandler(AdminUpdateFeed)).Name("admin-update-feed")
	router.Handle("/user/charge", mpg.NewHandler(Charge)).Name("charge")
	router.Handle("/user/account", mpg.NewHandler(Account)).Name("account")
	router.Handle("/user/uncheckout", mpg.NewHandler(Uncheckout)).Name("uncheckout")

	//router.Handle("/tasks/delete-blobs", mpg.NewHandler(DeleteBlobs)).Name("delete-blobs")

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
	router.Handle("/user/clear-feeds", mpg.NewHandler(ClearFeeds)).Name("clear-feeds")
	router.Handle("/user/clear-read", mpg.NewHandler(ClearRead)).Name("clear-read")
	router.Handle("/test/atom.xml", mpg.NewHandler(TestAtom)).Name("test-atom")
}

func wrap(f func(mpg.Context, http.ResponseWriter, *http.Request)) http.Handler {
	handler := mpg.NewHandler(f)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDevServer {
			w.Header().Add("Access-Control-Allow-Origin", r.Header.Get("Origin"))
			w.Header().Add("Access-Control-Allow-Credentials", "true")
		}
		handler.ServeHTTP(w, r)
	})
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	ua := r.Header.Get("User-Agent")
	mobile := strings.Contains(ua, "Mobi")
	if desktop, _ := r.Cookie("goread-desktop"); desktop != nil {
		switch desktop.Value {
		case "desktop":
			mobile = false
		case "mobile":
			mobile = true
		}
	}
	if mobile {
		w.Write(mobileIndex)
	} else {
		if err := templates.ExecuteTemplate(w, "base.html", includes(c, w, r)); err != nil {
			alog.Errorf(c, "%v", err)
			serveError(w, err)
		}
	}
}

func addFeed(c mpg.Context, userid string, outline *OpmlOutline) error {
	gn := goon.FromContext(c)
	o := outline.Outline[0]
	alog.Infof(c, "adding feed %v to user %s", o.XmlUrl, userid)
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

func mergeUserOpml(c context.Context, ud *UserData, outlines ...*OpmlOutline) error {
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
