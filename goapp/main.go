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
	"appengine/user"
	"github.com/gorilla/mux"
	"github.com/mjibson/MiniProfiler/go/miniprofiler"
	mpg "github.com/mjibson/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"html/template"
	"log"
	"net/http"
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
