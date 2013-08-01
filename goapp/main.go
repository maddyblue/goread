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
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"time"

	"appengine/datastore"
	"appengine/user"
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
		"templates/sitemap.html",
		"templates/sitemap-feed.html",
		"templates/story.html",
		"templates/admin-all-feeds.html",
		"templates/admin-date-formats.html",
		"templates/admin-feed.html",
		"templates/admin-stats.html",
	); err != nil {
		log.Fatal(err)
	}

	router.Handle("/", mpg.NewHandler(Main)).Name("main")
	router.Handle("/s/{feed}/{story}", mpg.NewHandler(Main)).Name("main-story")
	router.Handle("/login/google", mpg.NewHandler(LoginGoogle)).Name("login-google")
	router.Handle("/logout", mpg.NewHandler(Logout)).Name("logout")
	router.Handle("/push/{feed}", mpg.NewHandler(SubscribeCallback))
	router.Handle("/push", mpg.NewHandler(SubscribeCallback)).Name("subscribe-callback")
	router.Handle("/tasks/import-opml", mpg.NewHandler(ImportOpmlTask)).Name("import-opml-task")
	router.Handle("/tasks/subscribe-feed", mpg.NewHandler(SubscribeFeed)).Name("subscribe-feed")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	router.Handle("/user/clear-feeds", mpg.NewHandler(ClearFeeds)).Name("clear-feeds")
	router.Handle("/user/delete-account", mpg.NewHandler(DeleteAccount)).Name("delete-account")
	router.Handle("/user/export-opml", mpg.NewHandler(ExportOpml)).Name("export-opml")
	router.Handle("/user/feed-history", mpg.NewHandler(FeedHistory)).Name("feed-history")
	router.Handle("/user/get-contents", mpg.NewHandler(GetContents)).Name("get-contents")
	router.Handle("/user/get-feed", mpg.NewHandler(GetFeed)).Name("get-feed")
	router.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	router.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	router.Handle("/user/mark-all-read", mpg.NewHandler(MarkAllRead)).Name("mark-all-read")
	router.Handle("/user/mark-read", mpg.NewHandler(MarkRead)).Name("mark-read")
	router.Handle("/user/mark-unread", mpg.NewHandler(MarkUnread)).Name("mark-unread")
	router.Handle("/user/save-options", mpg.NewHandler(SaveOptions)).Name("save-options")
	router.Handle("/user/upload-opml", mpg.NewHandler(UploadOpml)).Name("upload-opml")

	router.Handle("/admin/all-feeds", mpg.NewHandler(AllFeeds)).Name("all-feeds")
	router.Handle("/admin/all-feeds-opml", mpg.NewHandler(AllFeedsOpml)).Name("all-feeds-opml")
	router.Handle("/date-formats", mpg.NewHandler(AdminDateFormats)).Name("admin-date-formats")
	router.Handle("/admin/feed", mpg.NewHandler(AdminFeed)).Name("admin-feed")
	router.Handle("/admin/stats", mpg.NewHandler(AdminStats)).Name("admin-stats")
	router.Handle("/admin/update-feed", mpg.NewHandler(AdminUpdateFeed)).Name("admin-update-feed")
	router.Handle("/tasks/cfixer", mpg.NewHandler(CFixer))
	router.Handle("/tasks/cfix", mpg.NewHandler(CFix))
	router.Handle("/user/charge", mpg.NewHandler(Charge)).Name("charge")
	router.Handle("/user/donate", mpg.NewHandler(Donate)).Name("donate")
	router.Handle("/user/account", mpg.NewHandler(Account)).Name("account")
	router.Handle("/user/uncheckout", mpg.NewHandler(Uncheckout)).Name("uncheckout")
	router.Handle("/sitemap.xml", mpg.NewHandler(SitemapXML))
	router.Handle("/sitemap", mpg.NewHandler(Sitemap)).Name("sitemap")
	router.Handle("/sitemap/{feed}", mpg.NewHandler(SitemapFeed)).Name("sitemap-feed")

	http.Handle("/", router)

	miniprofiler.ShowControls = true
	miniprofiler.StartHidden = true
	miniprofiler.ToggleShortcut = "Alt+C"
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	feed, _ := url.QueryUnescape(vars["feed"])
	story, _ := url.QueryUnescape(vars["story"])
	cu := user.Current(c)
	if len(feed) == 0 || len(story) == 0 || cu != nil {
		if err := templates.ExecuteTemplate(w, "base.html", includes(c, w, r)); err != nil {
			c.Errorf("%v", err)
			serveError(w, err)
		}
		return
	}
	b, _ := base64.URLEncoding.DecodeString(feed)
	feed = string(b)
	b, _ = base64.URLEncoding.DecodeString(story)
	story = string(b)
	gn := goon.FromContext(c)
	f := &Feed{Url: feed}
	s := &Story{Id: story, Parent: gn.Key(f)}
	sc := &StoryContent{Id: 1, Parent: gn.Key(s)}
	if err := gn.GetMulti([]interface{}{f, s, sc}); err != nil {
		c.Errorf("%v", err)
		serveError(w, err)
		return
	}
	if err := templates.ExecuteTemplate(w, "story.html", struct {
		Story   *Story
		Feed    *Feed
		Content template.HTML
		Ad      template.HTML
	}{
		Story:   s,
		Feed:    f,
		Content: template.HTML(sc.content()),
		Ad:      template.HTML(GOOGLE_AD),
	}); err != nil {
		c.Errorf("%v", err)
		serveError(w, err)
		return
	}
}

func SitemapXML(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url>
		<loc>http://%v/sitemap</loc>
		<changefreq>hourly</changefreq>
	</url>
</urlset>
`, r.Host)
}

const Limit = 1000

func Sitemap(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	q := datastore.NewQuery("F").KeysOnly()
	q = q.Limit(Limit)
	cs := r.FormValue("c")
	if len(cs) > 0 {
		if cur, err := datastore.DecodeCursor(cs); err == nil {
			q = q.Start(cur)
		}
	}
	var keys []*datastore.Key
	it := q.Run(c)
	for {
		k, err := it.Next(nil)
		if err == datastore.Done {
			break
		} else if err != nil {
			c.Errorf("next error: %v", err)
			break
		}
		keys = append(keys, k)
	}
	cs = ""
	if len(keys) == Limit {
		if cur, err := it.Cursor(); err == nil {
			cs = cur.String()
		}
	}
	if err := templates.ExecuteTemplate(w, "sitemap.html", struct {
		Keys   []*datastore.Key
		Cursor string
	}{
		Keys:   keys,
		Cursor: cs,
	}); err != nil {
		c.Errorf("%v", err)
		serveError(w, err)
		return
	}
}

func SitemapFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	feed := vars["feed"]
	fk, err := datastore.DecodeKey(feed)
	if err != nil {
		serveError(w, err)
		return
	}
	bf := base64.URLEncoding.EncodeToString([]byte(fk.StringID()))
	q := datastore.NewQuery("S").KeysOnly().Ancestor(fk)
	q = q.Limit(Limit)
	cs := r.FormValue("c")
	if len(cs) > 0 {
		if cur, err := datastore.DecodeCursor(cs); err == nil {
			q = q.Start(cur)
		}
	}
	stories := make(map[string]string)
	it := q.Run(c)
	for {
		k, err := it.Next(nil)
		if err == datastore.Done {
			break
		} else if err != nil {
			c.Errorf("next error: %v", err)
			break
		}
		stories[k.StringID()] = base64.URLEncoding.EncodeToString([]byte(k.StringID()))
	}
	cs = ""
	if len(stories) == Limit {
		if cur, err := it.Cursor(); err == nil {
			cs = cur.String()
		}
	}
	if err := templates.ExecuteTemplate(w, "sitemap-feed.html", struct {
		Feed, Feed64 string
		Stories      map[string]string
		Cursor       string
	}{
		Feed:    feed,
		Feed64:  bf,
		Stories: stories,
		Cursor:  cs,
	}); err != nil {
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
			if err := updateFeed(c, f.Url, feed, stories, false, false); err != nil {
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
