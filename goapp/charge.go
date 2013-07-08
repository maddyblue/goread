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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"appengine/datastore"
	"appengine/urlfetch"
	"appengine/user"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
)

// parent: User, key: 1
type UserCharge struct {
	_kind  string         `goon:"kind,UC"`
	Id     int64          `datastore:"-" goon:"id"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`

	Customer string    `datastore:"c,noindex json:"-"`
	Created  time.Time `datastore:"r,noindex"`
	Last4    string    `datastore:"l,noindex" json:"-"`
}

type StripeCustomer struct {
	Id      string      `json:"id"`
	Created int64       `json:"created"`
	Card    *StripeCard `json:"active_card"`
}

type StripeCard struct {
	Last4 string `json:"last4"`
}

func Charge(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uc := UserCharge{Id: 1, Parent: gn.Key(&u)}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	} else if u.Account != AFree {
		serveError(w, fmt.Errorf("You're already subscribed."))
		return
	}
	if err := gn.Get(&uc); err == nil && len(uc.Customer) > 0 {
		serveError(w, fmt.Errorf("You're already subscribed."))
		return
	} else if err != datastore.ErrNoSuchEntity {
		serveError(w, err)
		return
	}
	resp, err := stripe(c, "POST", "customers", url.Values{
		"email":       {u.Email},
		"description": {u.Id},
		"card":        {r.FormValue("stripeToken")},
		"plan":        {STRIPE_PLAN},
	}.Encode())
	if err != nil {
		serveError(w, err)
		return
	} else if resp.StatusCode != http.StatusOK {
		serveError(w, fmt.Errorf("Error"))
		return
	}
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)
	var sc StripeCustomer
	if err := json.Unmarshal(b, &sc); err != nil {
		serveError(w, err)
		return
	}
	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		if err := gn.Get(&u); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		if err := gn.Get(&uc); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		u.Account = APaid
		uc.Customer = sc.Id
		uc.Last4 = sc.Card.Last4
		uc.Created = time.Unix(sc.Created, 0)
		_, err := gn.PutMany(&u, &uc)
		return err
	}, nil); err != nil {
		serveError(w, err)
		return
	}
	b, _ = json.Marshal(&uc)
	w.Write(b)
}

func Account(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uc := UserCharge{Id: 1, Parent: gn.Key(&u)}
	if err := gn.Get(&uc); err == nil {
		b, _ := json.Marshal(&uc)
		w.Write(b)
	} else if err != datastore.ErrNoSuchEntity {
		serveError(w, err)
		return
	}
}

func Uncheckout(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uc := UserCharge{Id: 1, Parent: gn.Key(&u)}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	if err := gn.Get(&uc); err != nil || len(uc.Customer) == 0 {
		serveError(w, err)
		return
	}
	resp, err := stripe(c, "DELETE", "customers/"+uc.Customer, "")
	if err != nil {
		serveError(w, err)
		return
	} else if resp.StatusCode != http.StatusOK {
		serveError(w, fmt.Errorf("Error"))
		return
	}
	if err := gn.RunInTransaction(func(gn *goon.Goon) error {
		if err := gn.Get(&u); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		u.Account = AFree
		if err := gn.Delete(gn.Key(&uc)); err != nil {
			return err
		}
		_, err := gn.Put(&u)
		return err
	}, nil); err != nil {
		serveError(w, err)
		return
	}
	b, _ := json.Marshal(&uc)
	w.Write(b)
}

func stripe(c mpg.Context, method, urlStr, body string) (*http.Response, error) {
	cl := &http.Client{
		Transport: &urlfetch.Transport{
			Context:  c,
			Deadline: time.Minute,
		},
	}
	req, err := http.NewRequest(method, fmt.Sprintf("https://api.stripe.com/v1/%s", urlStr), strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(STRIPE_SECRET, "")
	return cl.Do(req)
}
