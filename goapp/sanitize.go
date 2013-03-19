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
	"code.google.com/p/go.net/html"
	"code.google.com/p/go.net/html/atom"
	"fmt"
	"io"
)

func Sanitize(s string) string {
	r := bytes.NewReader([]byte(s))
	z := html.NewTokenizer(r)
	buf := &bytes.Buffer{}
	scripts := 0
	for {
		if z.Next() == html.ErrorToken {
			if err := z.Err(); err == io.EOF {
				break
			} else {
				fmt.Println("SANITIZE ERROR", err.Error())
				return s
			}
		}
		t := z.Token()
		if t.DataAtom == atom.Script {
			fmt.Println("NUKING", t)
			if t.Type == html.StartTagToken {
				scripts++
			} else if t.Type == html.EndTagToken {
				scripts--
			}
		} else if scripts == 0 {
			buf.WriteString(t.String())
		}
	}

	return buf.String()
}
