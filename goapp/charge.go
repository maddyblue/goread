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
	"strconv"
	"strings"
	"time"

	"appengine/datastore"
	"appengine/urlfetch"
	"appengine/user"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
)

type Plan struct {
	Id, Name, Desc string
	Amount         int
}

// parent: User, key: 1
type UserCharge struct {
	_kind  string         `goon:"kind,UC"`
	Id     int64          `datastore:"-" goon:"id"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`

	Customer string    `datastore:"c,noindex json:"-"`
	Created  time.Time `datastore:"r,noindex"`
	Last4    string    `datastore:"l,noindex" json:"-"`
	Next     time.Time `datastore:"n,noindex"`
	Amount   int       `datastore:"a,noindex"`
	Interval string    `datastore:"i,noindex"`
	Plan     string    `datastore:"p,noindex"`
}

type StripeCustomer struct {
	Id      string `json:"id"`
	Created int64  `json:"created"`
	Card    struct {
		Last4 string `json:"last4"`
	} `json:"active_card"`
	Subscription struct {
		Plan struct {
			Interval string `json:"interval"`
			Id       string `json:"id"`
			Amount   int    `json:"amount"`
		} `json:"plan"`
		End int64 `json:"current_period_end"`
	} `json:"subscription"`
}

func Charge(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uc := &UserCharge{Id: 1, Parent: gn.Key(&u)}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	} else if u.Account != AFree {
		serveError(w, fmt.Errorf("You're already subscribed."))
		return
	}
	if err := gn.Get(uc); err == nil && len(uc.Customer) > 0 {
		serveError(w, fmt.Errorf("You're already subscribed."))
		return
	} else if err != datastore.ErrNoSuchEntity {
		serveError(w, err)
		return
	}
	resp, err := stripe(c, "POST", "customers", url.Values{
		"email":       {u.Email},
		"description": {u.Id},
		"card":        {r.FormValue("token")},
		"plan":        {r.FormValue("plan")},
	}.Encode())
	if err != nil {
		serveError(w, err)
		return
	} else if resp.StatusCode != http.StatusOK {
		c.Errorf("%s", resp.Body)
		serveError(w, fmt.Errorf("Error"))
		return
	}
	uc, err = setCharge(c, resp)
	if err != nil {
		serveError(w, err)
		return
	}
	b, _ := json.Marshal(&uc)
	w.Write(b)
}

func setCharge(c mpg.Context, r *http.Response) (*UserCharge, error) {
	var sc StripeCustomer
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &sc); err != nil {
		return nil, err
	}
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uc := UserCharge{Id: 1, Parent: gn.Key(&u)}
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
		uc.Next = time.Unix(sc.Subscription.End, 0)
		uc.Amount = sc.Subscription.Plan.Amount
		uc.Interval = sc.Subscription.Plan.Interval
		uc.Plan = sc.Subscription.Plan.Id
		_, err := gn.PutMany(&u, &uc)
		return err
	}, nil); err != nil {
		return nil, err
	}
	return &uc, nil
}

func Account(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	uc := &UserCharge{Id: 1, Parent: gn.Key(&u)}
	if err := gn.Get(uc); err == nil {
		if uc.Next.Before(time.Now()) {
			if resp, err := stripe(c, "GET", "customers/"+uc.Customer, ""); err == nil {
				if nuc, err := setCharge(c, resp); err == nil {
					uc = nuc
					c.Infof("updated user charge %v", cu.ID)
				}
			}
		}
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
		c.Errorf("%s", resp.Body)
		c.Errorf("stripe delete error, but proceeding")
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

func Donate(c mpg.Context, w http.ResponseWriter, r *http.Request) {
	cu := user.Current(c)
	gn := goon.FromContext(c)
	u := User{Id: cu.ID}
	if err := gn.Get(&u); err != nil {
		serveError(w, err)
		return
	}
	amount, err := strconv.Atoi(r.FormValue("amount"))
	if err != nil || amount < 200 {
		serveError(w, fmt.Errorf("bad amount: %v", r.FormValue("amount")))
		return
	}
	resp, err := stripe(c, "POST", "charges", url.Values{
		"amount":      {r.FormValue("amount")},
		"description": {fmt.Sprintf("%v - %v", u.Id, u.Email)},
		"card":        {r.FormValue("stripeToken")},
		"currency":    {"usd"},
	}.Encode())
	if err != nil {
		serveError(w, err)
		return
	} else if resp.StatusCode != http.StatusOK {
		c.Errorf("%s", resp.Body)
		serveError(w, fmt.Errorf("Error"))
		return
	}
}
