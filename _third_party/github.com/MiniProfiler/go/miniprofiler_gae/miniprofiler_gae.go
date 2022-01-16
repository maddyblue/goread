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

package miniprofiler_gae

import (
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/memcache"
	"google.golang.org/appengine/v2/user"
	"github.com/mjibson/goread/_third_party/github.com/MiniProfiler/go/miniprofiler"
	"github.com/mjibson/goread/_third_party/github.com/mjibson/appstats"
)

func init() {
	miniprofiler.Enable = EnableIfAdminOrDev
	miniprofiler.Get = GetMemcache
	miniprofiler.Store = StoreMemcache
	miniprofiler.MachineName = Instance
}

// EnableIfAdminOrDev returns true if this is the dev server or the current
// user is an admin. This is the default for miniprofiler.Enable.
func EnableIfAdminOrDev(r *http.Request) bool {
	if appengine.IsDevAppServer() {
		return true
	}
	c := appengine.NewContext(r)
	if u := user.Current(c); u != nil {
		return u.Admin
	}
	return false
}

// Instance returns the app engine instance id, or the hostname on dev.
// This is the default for miniprofiler.MachineName.
func Instance() string {
	if i := appengine.InstanceID(); i != "" && !appengine.IsDevAppServer() {
		return i[len(i)-8:]
	}
	return miniprofiler.Hostname()
}

// StoreMemcache stores the Profile in memcache. This is the default for
// miniprofiler.Store.
func StoreMemcache(r *http.Request, p *miniprofiler.Profile) {
	item := &memcache.Item{
		Key:   mp_key(string(p.Id)),
		Value: p.Json(),
	}
	c := appengine.NewContext(r)
	memcache.Set(c, item)
}

// GetMemcache gets the Profile from memcache. This is the default for
// miniprofiler.Get.
func GetMemcache(r *http.Request, id string) *miniprofiler.Profile {
	c := appengine.NewContext(r)
	item, err := memcache.Get(c, mp_key(id))
	if err != nil {
		return nil
	}
	return miniprofiler.ProfileFromJson(item.Value)
}

type Context struct {
	appstats.Context
	miniprofiler.Timer
}

/*
func (c Context) Call(service, method string, in, out internal.Proto.Message) (err error) {
	if c.Timer != nil && service != "__go__" {
		c.StepCustomTiming(service, method, fmt.Sprintf("%v\n\n%v", method, in.String()), func() {
			err = c.Context.Call(service, method, in, out)
		})
	} else {
		err = c.Context.Call(service, method, in, out)
	}
	return
}*/

func (c Context) Step(name string, f func(Context)) {
	if c.Timer != nil {
		c.Timer.Step(name, func(t miniprofiler.Timer) {
			f(Context{
				Context: c.Context,
				Timer:   t,
			})
		})
	} else {
		f(c)
	}
}

// NewHandler returns a profiled, appstats-aware appengine.Context.
func NewHandler(f func(Context, http.ResponseWriter, *http.Request)) http.Handler {
	return appstats.NewHandler(func(c context.Context, w http.ResponseWriter, r *http.Request) {
		h := miniprofiler.NewHandler(func(t miniprofiler.Timer, w http.ResponseWriter, r *http.Request) {
			pc := Context{
				Context: c.(appstats.Context),
				Timer:   t,
			}
			t.SetName(miniprofiler.FuncName(f))
			f(pc, w, r)
			t.AddCustomLink("appstats", pc.URL())
		})
		h.ServeHTTP(w, r)
	})
}

func mp_key(id string) string {
	return fmt.Sprintf("mini-profiler-results:%s", id)
}
