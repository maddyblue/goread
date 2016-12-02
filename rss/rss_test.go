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

package rss

import (
	"encoding/xml"
	"strings"
	"testing"

	"code.google.com/p/go-charset/charset"
)

func TestCDATALink(t *testing.T) {
	r := Rss{}
	d := xml.NewDecoder(strings.NewReader(ATALSOFT_FEED))
	d.CharsetReader = charset.NewReader
	d.DefaultSpace = "DefaultSpace"
	if err := d.Decode(&r); err != nil {
		t.Fatal(err)
	}
	if r.BaseLink() != "http://www.atalasoft.com/blogs/blogsrss.aspx?rss=loufranco" {
		t.Error("bad link", r.BaseLink())
	}
}

const ATALSOFT_FEED = `
<?xml version="1.0" encoding="utf-8"?><rss version="2.0">
<channel>
<title><![CDATA[Lou Franco]]></title>
<link><![CDATA[http://www.atalasoft.com/blogs/blogsrss.aspx?rss=loufranco]]></link>
<description><![CDATA[Lou Franco Atalasoft RSS]]></description>
<language><![CDATA[en-US]]></language>
</channel>
</rss>
`

func TestParseHub(t *testing.T) {
	r := Rss{}
	d := xml.NewDecoder(strings.NewReader(WP_FEED))
	d.CharsetReader = charset.NewReader
	d.DefaultSpace = "DefaultSpace"
	if err := d.Decode(&r); err != nil {
		t.Fatal(err)
	}
	if r.Hub() != "http://en.blog.wordpress.com/?pushpress=hub" {
		t.Error("bad hub")
	}
	if r.BaseLink() != "http://en.blog.wordpress.com" {
		t.Error("bad link")
	}
}

const WP_FEED = `
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
	xmlns:content="http://purl.org/rss/1.0/modules/content/"
	xmlns:wfw="http://wellformedweb.org/CommentAPI/"
	xmlns:dc="http://purl.org/dc/elements/1.1/"
	xmlns:atom="http://www.w3.org/2005/Atom"
	xmlns:sy="http://purl.org/rss/1.0/modules/syndication/"
	xmlns:slash="http://purl.org/rss/1.0/modules/slash/"
	xmlns:georss="http://www.georss.org/georss" xmlns:geo="http://www.w3.org/2003/01/geo/wgs84_pos#" xmlns:media="http://search.yahoo.com/mrss/"
	>

<channel>
	<title>WordPress.com News</title>
	<atom:link href="http://en.blog.wordpress.com/feed/" rel="self" type="application/rss+xml" />
	<link>http://en.blog.wordpress.com</link>
	<description>The latest news on WordPress.com and the WordPress community.</description>
	<lastBuildDate>Sat, 07 Dec 2013 04:07:23 +0000</lastBuildDate>
	<language>en</language>
		<sy:updatePeriod>hourly</sy:updatePeriod>
		<sy:updateFrequency>1</sy:updateFrequency>
	<generator>http://wordpress.com/</generator>
<cloud domain='en.blog.wordpress.com' port='80' path='/?rsscloud=notify' registerProcedure='' protocol='http-post' />
<image>
		<url>http://s2.wp.com/i/buttonw-com.png</url>
		<title></title>
		<link>http://en.blog.wordpress.com</link>
	</image>
	<atom:link rel="search" type="application/opensearchdescription+xml" href="http://en.blog.wordpress.com/osd.xml" title="WordPress.com News" />
	<atom:link rel='hub' href='http://en.blog.wordpress.com/?pushpress=hub'/>
</channel>
</rss>
`
