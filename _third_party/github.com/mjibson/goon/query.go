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
	"fmt"
	"reflect"

	"google.golang.org/appengine/datastore"
)

// Count returns the number of results for the query.
func (g *Goon) Count(q *datastore.Query) (int, error) {
	return q.Count(g.Context)
}

// GetAll runs the query and returns all the keys that match the query, as well
// as appending the values to dst, setting the goon key fields of dst, and
// caching the returned data in local memory.
//
// For "keys-only" queries dst can be nil, however if it is not, then GetAll
// appends zero value structs to dst, only setting the goon key fields.
// No data is cached with "keys-only" queries.
//
// See: https://developers.google.com/appengine/docs/go/datastore/reference#Query.GetAll
func (g *Goon) GetAll(q *datastore.Query, dst interface{}) ([]*datastore.Key, error) {
	v := reflect.ValueOf(dst)
	vLenBefore := 0

	if dst != nil {
		if v.Kind() != reflect.Ptr {
			return nil, fmt.Errorf("goon: Expected dst to be a pointer to a slice or nil, got instead: %v", v.Kind())
		}

		v = v.Elem()
		if v.Kind() != reflect.Slice {
			return nil, fmt.Errorf("goon: Expected dst to be a pointer to a slice or nil, got instead: %v", v.Kind())
		}

		vLenBefore = v.Len()
	}

	keys, err := q.GetAll(g.Context, dst)
	if err != nil {
		g.error(err)
		return nil, err
	}
	if dst == nil || len(keys) == 0 {
		return keys, nil
	}

	keysOnly := ((v.Len() - vLenBefore) != len(keys))
	updateCache := !g.inTransaction && !keysOnly

	// If this is a keys-only query, we need to fill the slice with zero value elements
	if keysOnly {
		elemType := v.Type().Elem()
		ptr := false
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
			ptr = true
		}

		if elemType.Kind() != reflect.Struct {
			return keys, fmt.Errorf("goon: Expected struct, got instead: %v", elemType.Kind())
		}

		for i := 0; i < len(keys); i++ {
			ev := reflect.New(elemType)
			if !ptr {
				ev = ev.Elem()
			}

			v.Set(reflect.Append(v, ev))
		}
	}

	if updateCache {
		g.cacheLock.Lock()
		defer g.cacheLock.Unlock()
	}

	for i, k := range keys {
		var e interface{}
		vi := v.Index(vLenBefore + i)
		if vi.Kind() == reflect.Ptr {
			e = vi.Interface()
		} else {
			e = vi.Addr().Interface()
		}

		if err := g.setStructKey(e, k); err != nil {
			return nil, err
		}

		if updateCache {
			// Cache lock is handled before the for loop
			g.cache[memkey(k)] = e
		}
	}

	return keys, nil
}

// Run runs the query.
func (g *Goon) Run(q *datastore.Query) *Iterator {
	return &Iterator{
		g: g,
		i: q.Run(g.Context),
	}
}

// Iterator is the result of running a query.
type Iterator struct {
	g *Goon
	i *datastore.Iterator
}

// Cursor returns a cursor for the iterator's current location.
func (t *Iterator) Cursor() (datastore.Cursor, error) {
	return t.i.Cursor()
}

// Next returns the entity of the next result. When there are no more results,
// datastore.Done is returned as the error. If dst is null (for a keys-only
// query), nil is returned as the entity.
//
// If the query is not keys only and dst is non-nil, it also loads the entity
// stored for that key into the struct pointer dst, with the same semantics
// and possible errors as for the Get function. This result is cached in memory.
//
// If the query is keys only, dst must be passed as nil. Otherwise the cache
// will be populated with empty entities since there is no way to detect the
// case of a keys-only query.
//
// Refer to appengine/datastore.Iterator.Next:
// https://developers.google.com/appengine/docs/go/datastore/reference#Iterator.Next
func (t *Iterator) Next(dst interface{}) (*datastore.Key, error) {
	k, err := t.i.Next(dst)
	if err != nil {
		return k, err
	}

	if dst != nil {
		// Update the struct to have correct key info
		t.g.setStructKey(dst, k)

		if !t.g.inTransaction {
			t.g.cacheLock.Lock()
			t.g.cache[memkey(k)] = dst
			t.g.cacheLock.Unlock()
		}
	}

	return k, err
}
