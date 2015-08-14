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
	"bytes"
	"errors"
	"io"

	"github.com/mjibson/goread/_third_party/golang.org/x/net/html"
	"github.com/mjibson/goread/_third_party/golang.org/x/net/html/atom"
)

var (
	ErrNoRssLink = errors.New("No rss link found")
	ErrNoIcon    = errors.New("No icon found")
)

func Autodiscover(b []byte) (string, error) {
	r := bytes.NewReader(b)
	z := html.NewTokenizer(r)
	for {
		if z.Next() == html.ErrorToken {
			if err := z.Err(); err == io.EOF {
				break
			} else {
				return "", ErrNoRssLink
			}
		}
		t := z.Token()
		switch t.DataAtom {
		case atom.Link:
			if t.Type == html.StartTagToken || t.Type == html.SelfClosingTagToken {
				attrs := make(map[string]string)
				for _, a := range t.Attr {
					attrs[a.Key] = a.Val
				}
				if attrs["rel"] == "alternate" && attrs["href"] != "" &&
					(attrs["type"] == "application/rss+xml" || attrs["type"] == "application/atom+xml") {
					return attrs["href"], nil
				}
			}
		}
	}
	return "", ErrNoRssLink
}

// Returns the href attribute of a <link rel="shortcut icon"> tag or error if not found.
func FindIcon(b []byte) (string, error) {
	r := bytes.NewReader(b)
	z := html.NewTokenizer(r)
	for {
		if z.Next() == html.ErrorToken {
			if err := z.Err(); err == io.EOF {
				break
			} else {
				return "", ErrNoIcon
			}
		}
		t := z.Token()
		switch t.DataAtom {
		case atom.Link:
			if t.Type == html.StartTagToken || t.Type == html.SelfClosingTagToken {
				attrs := make(map[string]string)
				for _, a := range t.Attr {
					attrs[a.Key] = a.Val
				}
				if attrs["rel"] == "shortcut icon" && attrs["href"] != "" {
					return attrs["href"], nil
				}
			}
		}
	}
	return "", ErrNoIcon
}
