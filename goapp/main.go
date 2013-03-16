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
	"appengine/urlfetch"
	"appengine/user"
	"code.google.com/p/goauth2/oauth"
	"encoding/json"
	"encoding/xml"
	"github.com/gorilla/mux"
	"github.com/mjibson/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
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
	router.Handle("/user/import/xml", mpg.NewHandler(ImportXml)).Name("import-xml")
	router.Handle("/user/import/reader", mpg.NewHandler(ImportReader)).Name("import-reader")
	router.Handle("/oauth2callback", mpg.NewHandler(Oauth2Callback)).Name("oauth2callback")
	http.Handle("/", router)

	miniprofiler.Position = "right"
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

	http.Redirect(w, r, url("main"), http.StatusFound)
}

func Logout(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if u, err := user.LogoutURL(c, url("main")); err == nil {
		http.Redirect(w, r, u, http.StatusFound)
	} else {
		http.Redirect(w, r, url("main"), http.StatusFound)
	}
}

func ImportXml(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	type outline struct {
		Outline []outline `xml:"outline"`
		Title   string    `xml:"title,attr"`
		Type    string    `xml:"type,attr"`
		XmlUrl  string    `xml:"xmlUrl,attr"`
		HtmlUrl string    `xml:"htmlUrl,attr"`
	}

	type Body struct {
		Outline []outline `xml:"outline"`
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
		}
	}
}

func ImportReader(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, oauth_conf.AuthCodeURL(""), http.StatusFound)
}

func Oauth2Callback(c mpg.Context, w http.ResponseWriter, r *http.Request) {
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
			Id string `json:"id"`
			Title string `json:"title"`
			HtmlUrl string `json:"htmlUrl"`
			Sortid string `json:"sortid"`
			Categories []struct {
				Id string `json:"id"`
				Label string `json:"label"`
			} `json:"categories"`
		} `json:"subscriptions"`
	}{}
	json.Unmarshal(b, &v)

	for _, sub := range v.Subscriptions {
		// add feed to user
		_ = sub
	}
}
