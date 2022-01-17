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

package appstats

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/log"
	"google.golang.org/appengine/v2/memcache"
	"google.golang.org/appengine/v2/user"
)

var (
	// RecordFraction is the fraction of requests to record.
	// Set to a number between 0.0 (none) and 1.0 (all).
	RecordFraction float64 = 1.0

	// ShouldRecord is the function used to determine if recording will occur
	// for a given request. The default is to use RecordFraction.
	ShouldRecord = DefaultShouldRecord

	// ProtoMaxBytes is the amount of protobuf data to record.
	// Data after this is truncated.
	ProtoMaxBytes = 150

	// Namespace is the memcache namespace under which to store appstats data.
	Namespace = "__appstats__"
)

const (
	serveURL   = "/_ah/stats/"
	detailsURL = serveURL + "details"
	fileURL    = serveURL + "file"
	staticURL  = serveURL + "static/"
)

func init() {
	http.HandleFunc(serveURL, appstatsHandler)
}

// DefaultShouldRecord will record a request based on RecordFraction.
func DefaultShouldRecord(r *http.Request) bool {
	if RecordFraction >= 1.0 {
		return true
	}

	return rand.Float64() < RecordFraction
}

// Context is a timing-aware context.Context.
type Context struct {
	context.Context
	header http.Header
	stats  *requestStats
}

// Call times an context.Context Call. Internal use only.
/*
func (c Context) Call(service, method string, in, out internal.Proto.Message) error {
	c.stats.wg.Add(1)
	defer c.stats.wg.Done()

	if service == "__go__" {
		return c.Context.Call(service, method, in, out)
	}

	stat := rpcStat{
		Service:   service,
		Method:    method,
		Start:     time.Now(),
		Offset:    time.Since(c.stats.Start),
		StackData: string(debug.Stack()),
	}
	err := c.Context.Call(service, method, in, out)
	stat.Duration = time.Since(stat.Start)
	stat.In = in.String()
	stat.Out = out.String()
	stat.Cost = getCost(out)

	if len(stat.In) > ProtoMaxBytes {
		stat.In = stat.In[:ProtoMaxBytes] + "..."
	}
	if len(stat.Out) > ProtoMaxBytes {
		stat.Out = stat.Out[:ProtoMaxBytes] + "..."
	}

	c.stats.lock.Lock()
	c.stats.RPCStats = append(c.stats.RPCStats, stat)
	c.stats.Cost += stat.Cost
	c.stats.lock.Unlock()
	return err
}*/

// NewContext creates a new timing-aware context from req.
func NewContext(req *http.Request) Context {
	c := appengine.NewContext(req)
	var uname string
	var admin bool
	if u := user.Current(c); u != nil {
		uname = u.String()
		admin = u.Admin
	}
	return Context{
		Context: c,
		header:  req.Header,
		stats: &requestStats{
			User:   uname,
			Admin:  admin,
			Method: req.Method,
			Path:   req.URL.Path,
			Query:  req.URL.RawQuery,
			Start:  time.Now(),
		},
	}
}

// WithContext enables profiling of functions without a corresponding request,
// as in the appengine/delay package. method and path may be empty.
func WithContext(context context.Context, method, path string, f func(Context)) {
	var uname string
	var admin bool
	if u := user.Current(context); u != nil {
		uname = u.String()
		admin = u.Admin
	}
	c := Context{
		Context: context,
		stats: &requestStats{
			User:   uname,
			Admin:  admin,
			Method: method,
			Path:   path,
			Start:  time.Now(),
		},
	}
	f(c)
	c.save()
}

const bufMaxLen = 1000000

func (c Context) save() {
	c.stats.wg.Wait()
	c.stats.Duration = time.Since(c.stats.Start)

	var buf_part, buf_full bytes.Buffer
	full := stats_full{
		Header: c.header,
		Stats:  c.stats,
	}
	if err := gob.NewEncoder(&buf_full).Encode(&full); err != nil {
		log.Errorf(c.Context, "appstats Save error: %v", err)
		return
	} else if buf_full.Len() > bufMaxLen {
		// first try clearing stack traces
		for i := range full.Stats.RPCStats {
			full.Stats.RPCStats[i].StackData = ""
		}
		buf_full.Truncate(0)
		gob.NewEncoder(&buf_full).Encode(&full)
	}
	part := stats_part(*c.stats)
	for i := range part.RPCStats {
		part.RPCStats[i].StackData = ""
		part.RPCStats[i].In = ""
		part.RPCStats[i].Out = ""
	}
	if err := gob.NewEncoder(&buf_part).Encode(&part); err != nil {
		log.Errorf(c.Context, "appstats Save error: %v", err)
		return
	}

	item_part := &memcache.Item{
		Key:   c.stats.PartKey(),
		Value: buf_part.Bytes(),
	}

	item_full := &memcache.Item{
		Key:   c.stats.FullKey(),
		Value: buf_full.Bytes(),
	}

	log.Infof(c.Context, "Saved; %s: %s, %s: %s, link: %v",
		item_part.Key,
		byteSize(len(item_part.Value)),
		item_full.Key,
		byteSize(len(item_full.Value)),
		c.URL(),
	)

	nc := c.storeContext()
	memcache.SetMulti(nc, []*memcache.Item{item_part, item_full})
}

// URL returns the appstats URL for the current request.
func (c Context) URL() string {
	u := url.URL{
		Path:     detailsURL,
		RawQuery: fmt.Sprintf("time=%v", c.stats.Start.Nanosecond()),
	}
	return u.String()
}

func (c Context) storeContext() context.Context {
	nc, _ := appengine.Namespace(c.Context, Namespace)
	return nc
}

func _context(r *http.Request) context.Context {
	c := appengine.NewContext(r)
	nc, _ := appengine.Namespace(c, Namespace)
	return nc
}

// handler is an http.Handler that records RPC statistics.
type handler struct {
	f func(context.Context, http.ResponseWriter, *http.Request)
}

// NewHandler returns a new Handler that will execute f.
func NewHandler(f func(context.Context, http.ResponseWriter, *http.Request)) http.Handler {
	return handler{
		f: f,
	}
}

// NewHandlerFunc returns a new HandlerFunc that will execute f.
func NewHandlerFunc(f func(context.Context, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := handler{
			f: f,
		}
		h.ServeHTTP(w, r)
	}
}

type responseWriter struct {
	http.ResponseWriter

	c Context
}

func (r responseWriter) Write(b []byte) (int, error) {
	// Emulate the behavior of http.ResponseWriter.Write since it doesn't
	// call our WriteHeader implementation.
	if r.c.stats.Status == 0 {
		r.WriteHeader(http.StatusOK)
	}

	return r.ResponseWriter.Write(b)
}

func (r responseWriter) WriteHeader(i int) {
	r.c.stats.Status = i
	r.ResponseWriter.WriteHeader(i)
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ShouldRecord(r) {
		c := NewContext(r)
		rw := responseWriter{
			ResponseWriter: w,
			c:              c,
		}
		h.f(c, rw, r)
		c.save()
	} else {
		c := appengine.NewContext(r)
		h.f(c, w, r)
	}
}
