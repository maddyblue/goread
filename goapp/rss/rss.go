// Copyright 2012 Evan Farrer. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/* Package rss provides a basic interface for processing RSS version 2.0 feeds
   as defined by http://cyber.law.harvard.edu/rss/rss.html with some additions.
*/
package rss

type Rss struct {
	XMLName string `xml:"rss"`

	// Required. Value should be rssgo.Version.
	Version string `xml:"version,attr"`

	// Required. The title of your channel.
	Title string `xml:"channel>title"`

	// Required. The URL of your website.
	Link string `xml:"DefaultSpace channel>link"`

	// Required. The description of the channel.
	Description string `xml:"channel>description"`

	// Optional. If present allowable values are found at
	// http://cyber.law.harvard.edu/rss/languages.html
	Language string `xml:"channel>language,omitempty"`

	// Optional. Copyright notice for channel content
	Copyright string `xml:"channel>copyright,omitempty"`

	// Optional. Email address for the managing editor
	ManagingEditor string `xml:"channel>managingEditor,omitempty"`

	// Optional. Email address for the channel's web master
	WebMaster string `xml:"channel>webMaster,omitempty"`

	// Optional. Publication date of the channel. See rssgo.ComposeRssDate and
	// rssgo.ParseRssDate
	PubDate string `xml:"channel>pubDate,omitempty"`

	// Optional. Date of last change to the channel content. See
	// rssgo.ComposeRssDate and rssgo.ParseRssDate
	LastBuildDate string `xml:"channel>lastBuildDate,omitempty"`

	// Optional. The hierarchical categorizations.
	Categories []Category `xml:"channel>category"`

	// Optional. The program used to generate the RSS
	Generator string `xml:"channel>generator,omitempty"`

	// Optional. The URL for the document describing the RSS format. Should be
	// the DocsURL constant
	Docs string `xml:"channel>docs,omitempty"`

	// Optional. A web service that supports the rssCloud interface
	Cloud *Cloud `xml:"channel>cloud"`

	// Optional. The number of minutes the channel can be cached
	Ttl int `xml:"channel>ttl,omitempty"`

	// Optional. An image that represents the channel.
	Image *Image `xml:"channel>image"`

	// Optional. The PICS rating for this channel. See http://www.w3.org/PICS/
	Rating string `xml:"channel>rating,omitempty"`

	// Optional. The channel's text input box
	TextInput *TextInput `xml:"channel>textInput"`

	// Optional. The hours when aggregators may not read the channel
	SkipHours *Hours `xml:"channel>skipHours,omitempty"`

	// Optional. The days when aggregators may not read the channel
	SkipDays *Days `xml:"channel>skipDays,omitempty"`

	// Optional. The RSS feed's items
	Items []Item `xml:"channel>item"`
}

// A RSS feeds item
type Item struct {
	// Either the title or the description are required. The title of the item.
	Title string `xml:"title,omitempty"`

	// Optional. The URL of the item
	Link string `xml:"link,omitempty"`

	// Either the title or the description are required. The item description.
	Description string `xml:"description,omitempty"`

	// Optional. The authors email address
	Author string `xml:"author,omitempty"`

	// Optional. The items hierarchical categorizations.
	Categories []Category `xml:"category"`

	// Optional. The URL for the page containing the items comments.
	Comments string `xml:"comments,omitempty"`

	// Optional. A media object attached to the item.
	Enclosure *Enclosure `xml:"enclosure"`

	// Optional. A unique identifier for the item
	Guid *Guid `xml:"guid"`

	// Optional. Publication date of the item. See rssgo.ComposeRssDate and
	// rssgo.ParseRssDate
	PubDate string `xml:"pubDate,omitempty"`

	// Optional. The RSS channel the item came from.
	Source *Source `xml:"source"`

	// Not listed in the spec.

	// The content of the item.
	// Tagged as "content:encoded".
	Content string `xml:"encoded,omitempty"`

	// Alternate dates
	Date      string `xml:"date,omitempty"`
	Published string `xml:"published,omitempty"`

	Media *MediaContent `xml:"content"`
}

type MediaContent struct {
	XMLBase string `xml:"http://search.yahoo.com/mrss/ content"`
	URL     string `xml:"url,attr"`
	Type    string `xml:"type,attr"`
}

// The RSS channel the item came from.
type Source struct {
	// Required. The title of the channel where the item came from.
	Source string `xml:",chardata"`

	// Required. The URL of the channel where the item came from.
	Url string `xml:"url,attr"`
}

// A unique identifier for the item
type Guid struct {

	// Required. The items GUID
	Guid string `xml:",chardata"`

	// Optional. If set to true the Guid must be a URL
	IsPermaLink bool `xml:"isPermaLink,attr,omitempty"`
}

// A media object for an item
type Enclosure struct {
	// Required. The enclosures URL.
	Url string `xml:"url,attr"`

	// Required. The enclosures size.
	Length int64 `xml:"length,attr,omitempty"`

	// Required. The enclosures MIME type.
	Type string `xml:"type,attr"`
}

// A day when an aggregator may not read the channel
type Days struct {
	// Required. The day
	Days []string `xml:"day"`
}

// An hour when an aggregator may not read the channel
type Hours struct {
	// Required. The hour
	Hours []int `xml:"hour"`
}

// The Rsa channel's text input box
type TextInput struct {
	// Required. The text input's Submit button text
	Title string `xml:"title,omitempty"`

	// Required. The text input's desxcription.
	Description string `xml:"description,omitempty"`

	// Required. The text input objects name
	Name string `xml:"name,omitempty"`

	// Required. The URL of the CGI script that processes the text input request.
	Link string `xml:"link,omitempty"`
}

// An RSS channel's image
type Image struct {
	// Required. The URL to the GIF, JPEG, or PNG image
	Url string `xml:"url"`

	// Required. The image title (should probably match the channels title)
	Title string `xml:"title"`

	// Required. The image link (should probably match the channels link)
	Link string `xml:"link"`

	// Optional. The image width.
	// Note: If the element is missing from the XML this field will have a value
	// of 0. The field value should be treated as having a value of DefaultWidth
	Width int `xml:"width,omitempty"`

	// Optional. The image height.
	// Note: If the element is missing from the XML this field will have a value
	// of 0. The field value should be treated as having a value of DefaultHeight
	Height int `xml:"height,omitempty"`
}

// The rssCloud interface parameters
type Cloud struct {
	// Required. The rssCloud domain
	Domain string `xml:"domain,attr"`

	// Required. The rssCloud port 0-65535
	Port int `xml:"port,attr,omitempty"`

	// Required. The rssCloud path
	Path string `xml:"path,attr"`

	// Required. The name of the rssCloud register procedure
	RegisterProcedure string `xml:"registerProcedure,attr"`

	// Required. The protocol. Must be xml-rpc, soap, or http-post
	Protocol string `xml:"protocol,attr"`
}

// A hierarchical categorization type
type Category struct {
	// Required. A hierarchical categorizations
	Category string `xml:",chardata"`

	// Optional. The domain URL
	Domain string `xml:"domain,attr,omitempty"`
}
