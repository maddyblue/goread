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
	"appengine/datastore"
	"appengine/taskqueue"
	"appengine/urlfetch"
	"appengine/user"
	"code.google.com/p/goauth2/oauth"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/mjibson/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	router.Handle("/tasks/add-feed", mpg.NewHandler(AddFeed)).Name("add-feed")
	router.Handle("/tasks/update-feeds", mpg.NewHandler(UpdateFeeds)).Name("update-feeds")
	router.Handle("/tasks/update-feed", mpg.NewHandler(UpdateFeed)).Name("update-feed")
	router.Handle("/user/add-subscription", mpg.NewHandler(AddSubscription)).Name("add-subscription")
	router.Handle("/user/import/opml", mpg.NewHandler(ImportOpml)).Name("import-opml")
	router.Handle("/user/import/reader", mpg.NewHandler(ImportReader)).Name("import-reader")
	router.Handle("/user/list-feeds", mpg.NewHandler(ListFeeds)).Name("list-feeds")
	http.Handle("/", router)

	miniprofiler.ShowControls = false
}

func Main(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	_ = goon.FromContext(c)
	if err := templates.ExecuteTemplate(w, "base.html", includes(c)); err != nil {
		serveError(w, err)
	}
}

func LoginGoogle(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if u := user.Current(c); u != nil {
		gn := goon.FromContext(c)
		user := User{}
		if e, err := gn.GetById(&user, u.ID, 0, nil); err == nil && e.NotFound {
			user.Email = u.Email
			gn.Put(e)
		}
	}

	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func Logout(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if u, err := user.LogoutURL(c, routeUrl("main")); err == nil {
		http.Redirect(w, r, u, http.StatusFound)
	} else {
		http.Redirect(w, r, routeUrl("main"), http.StatusFound)
	}
}

func ImportOpml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	type outline struct {
		Outline []outline `xml:"outline"`
		Title   string    `xml:"title,attr"`
		XmlUrl  string    `xml:"xmlUrl,attr"`
	}

	type Body struct {
		Outline []outline `xml:"outline"`
	}

	user := user.Current(c)

	var ts []*taskqueue.Task
	var proc func(label string, outlines []outline)
	proc = func(label string, outlines []outline) {
		for _, o := range outlines {
			if o.XmlUrl != "" {
				ts = append(ts, taskqueue.NewPOSTTask(routeUrl("add-feed"), url.Values{
					"user":  {user.ID},
					"label": {label},
					"feed":  {o.XmlUrl},
					"title": {o.Title},
				}))
			}

			if o.Title != "" && len(o.Outline) > 0 {
				proc(o.Title, o.Outline)
			}
		}
	}

	if file, _, err := r.FormFile("file"); err == nil {
		if fdata, err := ioutil.ReadAll(file); err == nil {
			fs := string(fdata)
			idx0 := strings.Index(fs, "<body>")
			idx1 := strings.LastIndex(fs, "</body>")
			fs = fs[idx0 : idx1+7]
			feed := Body{}
			if err = xml.Unmarshal([]byte(fs), &feed); err != nil {
				return
			}
			proc("", feed.Outline)
			taskqueue.AddMulti(c, ts, "")
		}
	}
}

func AddFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if err := addFeed(c, r.FormValue("user"), r.FormValue("feed"), r.FormValue("title"), r.FormValue("label"), r.FormValue("sortid")); err != nil {
		serveError(w, err)
	}
}

func addFeed(c mpg.Context, userid, url, title, label, sortid string) error {
	gn := goon.FromContext(c)

	u := User{}
	ue, _ := gn.GetById(&u, userid, 0, nil)
	if ue.NotFound {
		return nil
	}

	f := Feed{}
	fe, _ := gn.GetById(&f, url, 0, nil)
	if fe.NotFound {
		cl := urlfetch.Client(c)
		if r, err := cl.Get(url); err != nil {
			return err
		} else if r.StatusCode == http.StatusOK {
			b, _ := ioutil.ReadAll(r.Body)
			if feed, _ := ParseFeed(b); feed != nil {
				f = *feed
				gn.Put(fe)
			} else {
				return errors.New("Could not parse feed")
			}
		}
	}

	if title == "" && f.Title != "" {
		title = f.Title
	}

	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		ud := UserData{}
		ude, _ := gn.GetById(&ud, "data", 0, ue.Key)
		var fg Feeds

		if len(ud.Feeds) > 0 {
			if json.Unmarshal(ud.Feeds, &fg) != nil {
				return nil
			}
		}

		found := false
		for _, fd := range fg {
			if fd.Url == url {
				found = true
				break
			}
		}
		if !found {
			fg = append(fg, &UserFeed{
				Url:    url,
				Title:  title,
				Label:  label,
				Sortid: sortid,
			})
			b, _ := json.Marshal(fg)
			ud.Feeds = b
			gn.Put(ude)
		}

		fi := FeedIndex{}
		fie, _ := gn.GetById(&fi, "index", 0, fe.Key)
		found = false
		for _, fu := range fi.Users {
			if fu == userid {
				found = true
				break
			}
		}
		if !found {
			fi.Users = append(fi.Users, userid)
			gn.Put(fie)
		}

		return nil
	}, &datastore.TransactionOptions{XG: true}); err != nil {
		return err
	}

	return nil
}

func AddSubscription(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	u := user.Current(c)
	if err := addFeed(c, u.ID, r.FormValue("url"), "", "", ""); err != nil {
		serveError(w, err)
	}
}

func ImportReader(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, oauth_conf.AuthCodeURL(""), http.StatusFound)
}

func Oauth2Callback(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{}
	ue, _ := gn.GetById(&u, cu.ID, 0, nil)
	if ue.NotFound {
		return
	}

	t := &oauth.Transport{
		Config:    oauth_conf,
		Transport: &urlfetch.Transport{Context: c},
	}
	t.Exchange(r.FormValue("code"))
	cl := t.Client()
	resp, err := cl.Get("https://www.google.com/reader/api/0/subscription/list?output=json")
	if err != nil {
		serveError(w, err)
		return
	}
	b, _ := ioutil.ReadAll(resp.Body)

	v := struct {
		Subscriptions []struct {
			Id         string `json:"id"`
			Title      string `json:"title"`
			HtmlUrl    string `json:"htmlUrl"`
			Sortid     string `json:"sortid"`
			Categories []struct {
				Id    string `json:"id"`
				Label string `json:"label"`
			} `json:"categories"`
		} `json:"subscriptions"`
	}{}
	json.Unmarshal(b, &v)

	var ts []*taskqueue.Task
	for _, sub := range v.Subscriptions {
		var label []string
		if len(sub.Categories) > 0 {
			label = append(label, sub.Categories[0].Label)
		}
		ts = append(ts, taskqueue.NewPOSTTask(routeUrl("add-feed"), url.Values{
			"user":   {cu.ID},
			"label":  label,
			"feed":   {sub.Id[5:]},
			"title":  {sub.Title},
			"sortid": {sub.Sortid},
		}))
	}
	taskqueue.AddMulti(c, ts, "")

	http.Redirect(w, r, routeUrl("main"), http.StatusFound)
}

func ListFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	ud := UserData{}
	gn.GetById(&ud, "data", 0, datastore.NewKey(c, goon.Kind(&User{}), cu.ID, 0, nil))
	w.Write(ud.Feeds)
}

func UpdateFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	q := datastore.NewQuery(goon.Kind(&Feed{})).KeysOnly()
	q = q.Filter("u <=", time.Now().Add(-time.Hour))
	es, _ := gn.GetAll(q, nil)
	ts := make([]*taskqueue.Task, len(es))
	for i, e := range es {
		ts[i] = taskqueue.NewPOSTTask(routeUrl("update-feed"), url.Values{
			"feed": {e.Key.StringID()},
		})
	}
	taskqueue.AddMulti(c, ts, "")
}

func UpdateFeed(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	gn := goon.FromContext(c)
	url := r.FormValue("feed")
	f := Feed{}
	fe, _ := gn.GetById(&f, url, 0, nil)
	if fe.NotFound {
		return
	}
	cl := urlfetch.Client(c)
	if resp, err := cl.Get(url); err == nil && resp.StatusCode == http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		if feed, stories := ParseFeed(b); feed != nil {
			f = *feed
			f.Updated = time.Now()
			ses := make([]*goon.Entity, len(stories))
			sis := make([]StoryIndex, len(stories))
			sies := make([]*goon.Entity, len(stories))
			for i, s := range stories {
				ses[i], _ = gn.NewEntityById(s.Id, 0, fe.Key, s)
				sies[i], _ = gn.NewEntityById("index", 0, ses[i].Key, &sis[i])
			}
			gn.Put(fe)
			gn.PutMulti(ses)
			fmt.Println("PUT stories", len(ses), ses)

			fi := FeedIndex{}

			gn.RunInTransaction(func(gn *goon.Goon) error {
				gn.GetById(&fi, "index", 0, fe.Key)
				gn.GetMulti(sies)

				var puts []*goon.Entity
				for i, sie := range sies {
					if sie.NotFound {
						sis[i].Users = fi.Users
						puts = append(puts, sie)
					}
				}
				gn.PutMulti(puts)
				return nil
			}, nil)
		}
	}
}
