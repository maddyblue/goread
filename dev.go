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
	"fmt"
	"net/http"
	"time"

	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"

	"appengine/datastore"
	"appengine/user"
)

func ClearRead(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if !isDevServer {
		return
	}

	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := &User{Id: cu.ID}
	ud := &UserData{Id: "data", Parent: gn.Key(u)}
	if err := gn.Get(u); err != nil {
		c.Errorf("err: %v", err.Error())
		return
	}
	gn.Get(ud)
	u.Read = time.Time{}
	ud.Read = nil
	gn.PutMulti([]interface{}{u, ud})
	http.Redirect(w, r, "/", http.StatusFound)
}

func ClearFeeds(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	if !isDevServer {
		return
	}

	cu := user.Current(c)
	gn := goon.FromContext(c)
	done := make(chan bool)
	go func() {
		u := &User{Id: cu.ID}
		defer func() { done <- true }()
		ud := &UserData{Id: "data", Parent: gn.Key(u)}
		if err := gn.Get(u); err != nil {
			c.Errorf("user del err: %v", err.Error())
			return
		}
		gn.Get(ud)
		u.Read = time.Time{}
		ud.Read = nil
		ud.Opml = nil
		gn.PutMulti([]interface{}{u, ud})
		c.Infof("%v cleared", u.Email)
	}()
	del := func(kind string) {
		defer func() { done <- true }()
		q := datastore.NewQuery(kind).KeysOnly()
		keys, err := gn.GetAll(q, nil)
		if err != nil {
			c.Errorf("err: %v", err.Error())
			return
		}
		if err := gn.DeleteMulti(keys); err != nil {
			c.Errorf("err: %v", err.Error())
			return
		}
		c.Infof("%v deleted", kind)
	}
	types := []interface{}{
		&Feed{},
		&Story{},
		&StoryContent{},
		&Log{},
		&UserOpml{},
	}
	for _, i := range types {
		k := gn.Kind(i)
		go del(k)
	}

	for i := 0; i < len(types); i++ {
		<-done
	}

	http.Redirect(w, r, fmt.Sprintf("%s?url=http://localhost:8080%s", routeUrl("add-subscription"), routeUrl("test-atom")), http.StatusFound)
}

func TestAtom(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(testAtom))
}

const testAtom = `<?xml version="1.0"?>

<feed xmlns="http://www.w3.org/2005/Atom">
 <title>goread atom test</title>
<entry><id>1</id><title>1 A look at &lt;em>em&lt;/em></title></entry>
<entry><id>2</id><title type="html">2 A look at &lt;em>em&lt;/em></title></entry>
<entry>
 <id>4</id>
 <title>Issue 8311 created: &quot;HtmlElementBuilder doesn&#39;t allow &lt;col&gt; elements in &lt;colgroup&gt;&quot;</title>
</entry>
</feed>
`
