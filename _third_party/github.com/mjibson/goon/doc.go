/*
Package goon provides an autocaching interface to the app engine datastore
similar to the python NDB package.

Goon differs from the datastore package in various ways: it remembers the
appengine Context, which need only be specified once at creation time; kinds
need not be specified as they are computed, by default, from a type's name;
keys are inferred from specially-tagged fields on types, removing the need to
pass key objects around.

In general, the difference is that Goon's API is identical to the datastore API,
it's just shorter.

Keys in Goon are stored in the structs themselves. Below is an example struct
with a field to specify the id (see the Key Specifications section below for
full documentation).
	type User struct {
		Id    string `datastore:"-" goon:"id"`
		Name  string
	}

Thus, to get a User with id 2:
	userid := 2
	g := goon.NewGoon(r)
	u := &User{Id: userid}
	g.Get(u)

Key Specifications

For both the Key and KeyError functions, src must be a S or *S for some
struct type S. The key is extracted based on various fields of S. If a field
of type int64 or string has a struct tag named goon with value "id", it is
used as the key's id. If a field of type *datastore.Key has a struct tag
named goon with value "parent", it is used as the key's parent. If a field
of type string has a struct tag named goon with value "kind", it is used
as the key's kind. The "kind" field supports an optional second parameter
which is the default kind name. If no kind field exists, the struct's name
is used. These fields should all have their datastore field marked as "-".

Example, with kind User:
	type User struct {
		Id    string `datastore:"-" goon:"id"`
		Read  time.Time
	}

Example, with kind U if _kind is the empty string:
	type User struct {
		_kind string `goon:"kind,U"`
		Id    string `datastore:"-" goon:"id"`
		Read  time.Time
	}

To override kind of a single entity to UserKind:
	u := User{_kind: "UserKind"}

An example with both parent and kind:
	type UserData struct {
		Id     string         `datastore:"-" goon:"id"`
		_kind  string         `goon:"kind,UD"`
		Parent *datastore.Key `datastore:"-" goon:"parent"`
		Data   []byte
	}

Features

Datastore interaction with: Get, GetMulti, Put, PutMulti, Delete, DeleteMulti, Queries.

All key-based operations backed by memory and memcache.

Per-request, in-memory cache: fetch the same key twice, the second request is served from local memory.

Intelligent multi support: running GetMulti correctly fetches from memory, then memcache, then the datastore; each tier only sends keys off to the next one if they were missing.

Memcache control variance: long memcache requests are cancelled.

Transactions use a separate context, but locally cache any results on success.

Automatic kind naming: struct names are inferred by reflection, removing the need to manually specify key kinds.

Simpler API than appengine/datastore.

API comparison between goon and datastore

put with incomplete key

datastore:

	type Group struct {
		Name string
	}
	c := r.NewContext()
	g := &Group{Name: "name"}
	k := datastore.NewIncompleteKey(c, "Group", nil)
	err := datastore.Put(c, k, g)

goon:

	type Group struct {
		Id   int64 `datastore:"-" goon:"id"`
		Name string
	}
	n := goon.NewGoon(r)
	g := &Group{Name: "name"}
	err := n.Put(g)

get with known key

datastore:

	type Group struct {
		Name string
	}
	c := r.NewContext()
	g := &Group{}
	k := datastore.NewKey(c, "Group", "", 1, nil)
	err := datastore.Get(c, k, g)

goon:

	type Group struct {
		Id   int64 `datastore:"-" goon:"id"`
		Name string
	}
	n := goon.NewGoon(r)
	g := &Group{Id: 1}
	err := n.Get(g)

Memcache Control Variance

Memcache is generally fast. When it is slow, goon will timeout the memcache
requests and proceed to use the datastore directly. The memcache put and
get timeout variables determine how long to wait for various kinds of
requests. The default settings were determined experimentally and should
provide reasonable defaults for most applications.

See: http://talks.golang.org/2013/highperf.slide#23
*/
package goon
