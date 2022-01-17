/*
 * Copyright (c) 2012 Matt Jibson <matt.jibson@gmail.com>
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

package goon

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/log"
	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/memcache"
)

var (
	// LogErrors issues context.Context.Errorf on any error.
	LogErrors = true
	// LogTimeoutErrors issues context.Context.Warningf on memcache timeout errors.
	LogTimeoutErrors = false

	// MemcachePutTimeoutThreshold is the number of bytes at which the memcache
	// timeout uses the large setting.
	MemcachePutTimeoutThreshold = 1024 * 50
	// MemcachePutTimeoutSmall is the amount of time to wait during memcache
	// Put operations before aborting them and using the datastore.
	MemcachePutTimeoutSmall = time.Millisecond * 5
	// MemcachePutTimeoutLarge is the amount of time to wait for large memcache
	// Put requests.
	MemcachePutTimeoutLarge = time.Millisecond * 15
	// MemcacheGetTimeout is the amount of time to wait for all memcache Get
	// requests.
	MemcacheGetTimeout = time.Millisecond * 10
)

// Goon holds the app engine context and the request memory cache.
type Goon struct {
	Context       context.Context
	cache         map[string]interface{}
	cacheLock     sync.RWMutex // protect the cache from concurrent goroutines to speed up RPC access
	inTransaction bool
	toSet         map[string]interface{}
	toDelete      map[string]bool
	toDeleteMC    map[string]bool
	// KindNameResolver is used to determine what Kind to give an Entity.
	// Defaults to DefaultKindName
	KindNameResolver KindNameResolver
}

func memkey(k *datastore.Key) string {
	// Versioning, so that incompatible changes to the cache system won't cause problems
	return "g2:" + k.Encode()
}

// NewGoon creates a new Goon object from the given request.
func NewGoon(r *http.Request) *Goon {
	return FromContext(appengine.NewContext(r))
}

// FromContext creates a new Goon object from the given context Context.
// Useful with profiling packages like appstats.
func FromContext(c context.Context) *Goon {
	return &Goon{
		Context:          c,
		cache:            make(map[string]interface{}),
		KindNameResolver: DefaultKindName,
	}
}

func (g *Goon) error(err error) {
	if !LogErrors {
		return
	}
	_, filename, line, ok := runtime.Caller(1)
	if ok {
		log.Errorf(g.Context, "goon - %s:%d - %v", filepath.Base(filename), line, err)
	} else {
		log.Errorf(g.Context, "goon - %v", err)
	}
}

func (g *Goon) timeoutError(err error) {
	if LogTimeoutErrors {
		log.Warningf(g.Context, "goon memcache timeout: %v", err)
	}
}

func (g *Goon) extractKeys(src interface{}, putRequest bool) ([]*datastore.Key, error) {
	v := reflect.Indirect(reflect.ValueOf(src))
	if v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("goon: value must be a slice or pointer-to-slice")
	}
	l := v.Len()

	keys := make([]*datastore.Key, l)
	for i := 0; i < l; i++ {
		vi := v.Index(i)
		key, hasStringId, err := g.getStructKey(vi.Interface())
		if err != nil {
			return nil, err
		}
		if !putRequest && key.Incomplete() {
			return nil, fmt.Errorf("goon: cannot find a key for struct - %v", vi.Interface())
		} else if putRequest && key.Incomplete() && hasStringId {
			return nil, fmt.Errorf("goon: empty string id on put")
		}
		keys[i] = key
	}
	return keys, nil
}

// Key is the same as KeyError, except nil is returned on error or if the key
// is incomplete.
func (g *Goon) Key(src interface{}) *datastore.Key {
	if k, err := g.KeyError(src); err == nil {
		return k
	}
	return nil
}

// Kind returns src's datastore Kind or "" on error.
func (g *Goon) Kind(src interface{}) string {
	if k, err := g.KeyError(src); err == nil {
		return k.Kind()
	}
	return ""
}

// KeyError returns the key of src based on its properties.
func (g *Goon) KeyError(src interface{}) (*datastore.Key, error) {
	key, _, err := g.getStructKey(src)
	return key, err
}

// RunInTransaction runs f in a transaction. It calls f with a transaction
// context tg that f should use for all App Engine operations. Neither cache nor
// memcache are used or set during a transaction.
//
// Otherwise similar to appengine/datastore.RunInTransaction:
// https://developers.google.com/appengine/docs/go/datastore/reference#RunInTransaction
func (g *Goon) RunInTransaction(f func(tg *Goon) error, opts *datastore.TransactionOptions) error {
	var ng *Goon
	err := datastore.RunInTransaction(g.Context, func(tc context.Context) error {
		ng = &Goon{
			Context:          tc,
			inTransaction:    true,
			toSet:            make(map[string]interface{}),
			toDelete:         make(map[string]bool),
			toDeleteMC:       make(map[string]bool),
			KindNameResolver: g.KindNameResolver,
		}
		return f(ng)
	}, opts)

	if err == nil {
		if len(ng.toDeleteMC) > 0 {
			var memkeys []string
			for k := range ng.toDeleteMC {
				memkeys = append(memkeys, k)
			}
			memcache.DeleteMulti(g.Context, memkeys)
		}

		g.cacheLock.Lock()
		defer g.cacheLock.Unlock()
		for k, v := range ng.toSet {
			g.cache[k] = v
		}

		for k := range ng.toDelete {
			delete(g.cache, k)
		}
	} else {
		g.error(err)
	}

	return err
}

// Put saves the entity src into the datastore based on src's key k. If k
// is an incomplete key, the returned key will be a unique key generated by
// the datastore.
func (g *Goon) Put(src interface{}) (*datastore.Key, error) {
	ks, err := g.PutMulti([]interface{}{src})
	if err != nil {
		if me, ok := err.(appengine.MultiError); ok {
			return nil, me[0]
		}
		return nil, err
	}
	return ks[0], nil
}

const putMultiLimit = 500

// PutMulti is a batch version of Put.
//
// src must be a *[]S, *[]*S, *[]I, []S, []*S, or []I, for some struct type S,
// or some interface type I. If *[]I or []I, each element must be a struct pointer.
func (g *Goon) PutMulti(src interface{}) ([]*datastore.Key, error) {
	keys, err := g.extractKeys(src, true) // allow incomplete keys on a Put request
	if err != nil {
		return nil, err
	}

	var memkeys []string
	for _, key := range keys {
		if !key.Incomplete() {
			memkeys = append(memkeys, memkey(key))
		}
	}

	// Memcache needs to be updated after the datastore to prevent a common race condition,
	// where a concurrent request will fetch the not-yet-updated data from the datastore
	// and populate memcache with it.
	if g.inTransaction {
		for _, mk := range memkeys {
			g.toDeleteMC[mk] = true
		}
	} else {
		defer memcache.DeleteMulti(g.Context, memkeys)
	}

	v := reflect.Indirect(reflect.ValueOf(src))
	multiErr, any := make(appengine.MultiError, len(keys)), false
	goroutines := (len(keys)-1)/putMultiLimit + 1
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			lo := i * putMultiLimit
			hi := (i + 1) * putMultiLimit
			if hi > len(keys) {
				hi = len(keys)
			}
			rkeys, pmerr := datastore.PutMulti(g.Context, keys[lo:hi], v.Slice(lo, hi).Interface())
			if pmerr != nil {
				any = true // this flag tells PutMulti to return multiErr later
				merr, ok := pmerr.(appengine.MultiError)
				if !ok {
					g.error(pmerr)
					for j := lo; j < hi; j++ {
						multiErr[j] = pmerr
					}
					return
				}
				copy(multiErr[lo:hi], merr)
			}

			for i, key := range keys[lo:hi] {
				if multiErr[lo+i] != nil {
					continue // there was an error writing this value, go to next
				}
				vi := v.Index(lo + i).Interface()
				if key.Incomplete() {
					g.setStructKey(vi, rkeys[i])
					keys[i] = rkeys[i]
				}
				if g.inTransaction {
					mk := memkey(rkeys[i])
					delete(g.toDelete, mk)
					g.toSet[mk] = vi
				} else {
					g.putMemory(vi)
				}
			}
		}(i)
	}
	wg.Wait()
	if any {
		return keys, realError(multiErr)
	}
	return keys, nil
}

func (g *Goon) putMemoryMulti(src interface{}, exists []byte) {
	v := reflect.Indirect(reflect.ValueOf(src))
	for i := 0; i < v.Len(); i++ {
		if exists[i] == 0 {
			continue
		}
		g.putMemory(v.Index(i).Interface())
	}
}

func (g *Goon) putMemory(src interface{}) {
	key, _, _ := g.getStructKey(src)
	g.cacheLock.Lock()
	defer g.cacheLock.Unlock()
	g.cache[memkey(key)] = src
}

// FlushLocalCache clears the local memory cache.
func (g *Goon) FlushLocalCache() {
	g.cacheLock.Lock()
	g.cache = make(map[string]interface{})
	g.cacheLock.Unlock()
}

func (g *Goon) putMemcache(srcs []interface{}, exists []byte) error {
	items := make([]*memcache.Item, len(srcs))
	payloadSize := 0
	for i, src := range srcs {
		toSerialize := src
		if exists[i] == 0 {
			toSerialize = nil
		}
		data, err := serializeStruct(toSerialize)
		if err != nil {
			g.error(err)
			return err
		}
		key, _, err := g.getStructKey(src)
		if err != nil {
			return err
		}
		// payloadSize will overflow if we push 2+ gigs on a 32bit machine
		payloadSize += len(data)
		items[i] = &memcache.Item{
			Key:   memkey(key),
			Value: data,
		}
	}
	memcacheTimeout := MemcachePutTimeoutSmall
	if payloadSize >= MemcachePutTimeoutThreshold {
		memcacheTimeout = MemcachePutTimeoutLarge
	}
	errc := make(chan error)
	go func() {
		c, _ := context.WithTimeout(g.Context, memcacheTimeout)
		errc <- memcache.SetMulti(c, items)
	}()
	g.putMemoryMulti(srcs, exists)
	err := <-errc
	if appengine.IsTimeoutError(err) {
		g.timeoutError(err)
		err = nil
	} else if err != nil {
		g.error(err)
	}
	return err
}

// Get loads the entity based on dst's key into dst
// If there is no such entity for the key, Get returns
// datastore.ErrNoSuchEntity.
func (g *Goon) Get(dst interface{}) error {
	set := reflect.ValueOf(dst)
	if set.Kind() != reflect.Ptr {
		return fmt.Errorf("goon: expected pointer to a struct, got %#v", dst)
	}
	if !set.CanSet() {
		set = set.Elem()
	}
	dsts := []interface{}{dst}
	if err := g.GetMulti(dsts); err != nil {
		// Look for an embedded error if it's multi
		if me, ok := err.(appengine.MultiError); ok {
			return me[0]
		}
		// Not multi, normal error
		return err
	}
	set.Set(reflect.Indirect(reflect.ValueOf(dsts[0])))
	return nil
}

const getMultiLimit = 1000

// GetMulti is a batch version of Get.
//
// dst must be a *[]S, *[]*S, *[]I, []S, []*S, or []I, for some struct type S,
// or some interface type I. If *[]I or []I, each element must be a struct pointer.
func (g *Goon) GetMulti(dst interface{}) error {
	keys, err := g.extractKeys(dst, false) // don't allow incomplete keys on a Get request
	if err != nil {
		return err
	}

	v := reflect.Indirect(reflect.ValueOf(dst))

	if g.inTransaction {
		// todo: support getMultiLimit in transactions
		return datastore.GetMulti(g.Context, keys, v.Interface())
	}

	var dskeys []*datastore.Key
	var dsdst []interface{}
	var dixs []int

	var memkeys []string
	var mixs []int

	g.cacheLock.RLock()
	for i, key := range keys {
		m := memkey(key)
		vi := v.Index(i)

		if vi.Kind() == reflect.Struct {
			vi = vi.Addr()
		}

		if s, present := g.cache[m]; present {
			if vi.Kind() == reflect.Interface {
				vi = vi.Elem()
			}

			reflect.Indirect(vi).Set(reflect.Indirect(reflect.ValueOf(s)))
		} else {
			memkeys = append(memkeys, m)
			mixs = append(mixs, i)
			dskeys = append(dskeys, key)
			dsdst = append(dsdst, vi.Interface())
			dixs = append(dixs, i)
		}
	}
	g.cacheLock.RUnlock()

	if len(memkeys) == 0 {
		return nil
	}

	multiErr, any := make(appengine.MultiError, len(keys)), false

	c, _ := context.WithTimeout(g.Context, MemcacheGetTimeout)
	memvalues, err := memcache.GetMulti(c, memkeys)
	if appengine.IsTimeoutError(err) {
		g.timeoutError(err)
		err = nil
	} else if err != nil {
		g.error(err) // timing out or another error from memcache isn't something to fail over, but do log it
		// No memvalues found, prepare the datastore fetch list already prepared above
	} else if len(memvalues) > 0 {
		// since memcache fetch was successful, reset the datastore fetch list and repopulate it
		dskeys = dskeys[:0]
		dsdst = dsdst[:0]
		dixs = dixs[:0]
		// we only want to check the returned map if there weren't any errors
		// unlike the datastore, memcache will return a smaller map with no error if some of the keys were missed

		for i, m := range memkeys {
			d := v.Index(mixs[i]).Interface()
			if v.Index(mixs[i]).Kind() == reflect.Struct {
				d = v.Index(mixs[i]).Addr().Interface()
			}
			if s, present := memvalues[m]; present {
				err := deserializeStruct(d, s.Value)
				if err == datastore.ErrNoSuchEntity {
					any = true // this flag tells GetMulti to return multiErr later
					multiErr[mixs[i]] = err
				} else if err != nil {
					g.error(err)
					return err
				} else {
					g.putMemory(d)
				}
			} else {
				dskeys = append(dskeys, keys[mixs[i]])
				dsdst = append(dsdst, d)
				dixs = append(dixs, mixs[i])
			}
		}
		if len(dskeys) == 0 {
			if any {
				return realError(multiErr)
			}
			return nil
		}
	}

	goroutines := (len(dskeys)-1)/getMultiLimit + 1
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			var toCache []interface{}
			var exists []byte
			lo := i * getMultiLimit
			hi := (i + 1) * getMultiLimit
			if hi > len(dskeys) {
				hi = len(dskeys)
			}
			gmerr := datastore.GetMulti(g.Context, dskeys[lo:hi], dsdst[lo:hi])
			if gmerr != nil {
				any = true // this flag tells GetMulti to return multiErr later
				merr, ok := gmerr.(appengine.MultiError)
				if !ok {
					g.error(gmerr)
					for j := lo; j < hi; j++ {
						multiErr[j] = gmerr
					}
					return
				}
				for i, idx := range dixs[lo:hi] {
					if merr[i] == nil {
						toCache = append(toCache, dsdst[lo+i])
						exists = append(exists, 1)
					} else {
						if merr[i] == datastore.ErrNoSuchEntity {
							toCache = append(toCache, dsdst[lo+i])
							exists = append(exists, 0)
						}
						multiErr[idx] = merr[i]
					}
				}
			} else {
				toCache = append(toCache, dsdst[lo:hi]...)
				exists = append(exists, bytes.Repeat([]byte{1}, hi-lo)...)
			}
			if len(toCache) > 0 {
				if err := g.putMemcache(toCache, exists); err != nil {
					g.error(err)
					// since putMemcache() gives no guarantee it will actually store the data in memcache
					// we log and swallow this error
				}

			}
		}(i)
	}
	wg.Wait()
	if any {
		return realError(multiErr)
	}
	return nil
}

// Delete deletes the entity for the given key.
func (g *Goon) Delete(key *datastore.Key) error {
	keys := []*datastore.Key{key}
	err := g.DeleteMulti(keys)
	if me, ok := err.(appengine.MultiError); ok {
		return me[0]
	}
	return err
}

const deleteMultiLimit = 500

// Returns a single error if each error in MultiError is the same
// otherwise, returns multiError or nil (if multiError is empty)
func realError(multiError appengine.MultiError) error {
	if len(multiError) == 0 {
		return nil
	}
	init := multiError[0]
	for i := 1; i < len(multiError); i++ {
		// since type error could hold structs, pointers, etc,
		// the only way to compare non-nil errors is by their string output
		if init == nil || multiError[i] == nil {
			if init != multiError[i] {
				return multiError
			}
		} else if init.Error() != multiError[i].Error() {
			return multiError
		}
	}
	// all errors are the same
	// some errors are *always* returned in MultiError form from the datastore
	if _, ok := init.(*datastore.ErrFieldMismatch); ok { // returned in GetMulti
		return multiError
	}
	if init == datastore.ErrInvalidEntityType || // returned in GetMulti
		init == datastore.ErrNoSuchEntity { // returned in GetMulti
		return multiError
	}
	// datastore.ErrInvalidKey is returned as a single error in PutMulti
	return init
}

// DeleteMulti is a batch version of Delete.
func (g *Goon) DeleteMulti(keys []*datastore.Key) error {
	if len(keys) == 0 {
		return nil
		// not an error, and it was "successful", so return nil
	}
	memkeys := make([]string, len(keys))

	g.cacheLock.Lock()
	for i, k := range keys {
		mk := memkey(k)
		memkeys[i] = mk

		if g.inTransaction {
			delete(g.toSet, mk)
			g.toDelete[mk] = true
		} else {
			delete(g.cache, mk)
		}
	}
	g.cacheLock.Unlock()

	// Memcache needs to be updated after the datastore to prevent a common race condition,
	// where a concurrent request will fetch the not-yet-updated data from the datastore
	// and populate memcache with it.
	if g.inTransaction {
		for _, mk := range memkeys {
			g.toDeleteMC[mk] = true
		}
	} else {
		defer memcache.DeleteMulti(g.Context, memkeys)
	}

	multiErr, any := make(appengine.MultiError, len(keys)), false
	goroutines := (len(keys)-1)/deleteMultiLimit + 1
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			lo := i * deleteMultiLimit
			hi := (i + 1) * deleteMultiLimit
			if hi > len(keys) {
				hi = len(keys)
			}
			dmerr := datastore.DeleteMulti(g.Context, keys[lo:hi])
			if dmerr != nil {
				any = true // this flag tells DeleteMulti to return multiErr later
				merr, ok := dmerr.(appengine.MultiError)
				if !ok {
					g.error(dmerr)
					for j := lo; j < hi; j++ {
						multiErr[j] = dmerr
					}
					return
				}
				copy(multiErr[lo:hi], merr)
			}
		}(i)
	}
	wg.Wait()
	if any {
		return realError(multiErr)
	}
	return nil
}

// NotFound returns true if err is an appengine.MultiError and err[idx] is a datastore.ErrNoSuchEntity.
func NotFound(err error, idx int) bool {
	if merr, ok := err.(appengine.MultiError); ok {
		return idx < len(merr) && merr[idx] == datastore.ErrNoSuchEntity
	}
	return false
}
