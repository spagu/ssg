package externalsource

// Syndication feed parser (#89). SSG could already fetch a feed — `format: xml`
// parses one fine — but it could not *understand* one: Atom puts its entries at
// .feed.entry, RSS at .rss.channel.item and JSON Feed at .items, with the title,
// link and date in different places and dates in different encodings. A template
// therefore had to know which format was on the other end, and swapping a source
// from RSS to Atom broke it even though the content was identical.
//
// `format: feed` accepts any of the three and returns one shape:
//
//	{title, home_page_url, items: [{id, url, title, summary, content_html,
//	                                published, updated, tags, author}]}
//
// Dates are parsed into time.Time, so the template date helpers work and items
// from different feeds sort against each other — which is what makes aggregating
// several sources possible at all.

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// FeedItem is one normalized entry. Exported so the generator can aggregate
// several feeds without re-deriving the shape.
type FeedItem struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary,omitempty"`
	ContentHTML string    `json:"content_html,omitempty"`
	Published   time.Time `json:"published,omitempty"`
	Updated     time.Time `json:"updated,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Author      string    `json:"author,omitempty"`
	// Source and Label carry provenance once items from several feeds are mixed:
	// Source is the external-source name, Label the human name a config gives it.
	Source string `json:"source,omitempty"`
	Label  string `json:"label,omitempty"`
}

// ParsedFeed is a whole normalized feed.
type ParsedFeed struct {
	Title       string     `json:"title"`
	HomePageURL string     `json:"home_page_url,omitempty"`
	Items       []FeedItem `json:"items"`
}

// parseFeed detects the format from the payload itself rather than trusting a
// declaration: a URL ending in .xml may be either Atom or RSS, and a redirect can
// change what actually arrives.
func parseFeed(r io.Reader) (interface{}, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	feed, err := ParseFeedBytes(raw)
	if err != nil {
		return nil, err
	}
	return feedToTemplateValue(feed), nil
}

// ParseFeedBytes normalizes an Atom, RSS 2.0 or JSON Feed document.
func ParseFeedBytes(raw []byte) (*ParsedFeed, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		return parseJSONFeedDoc(raw)
	}
	// XML: the root element decides. Atom's is <feed>, RSS's is <rss>.
	if strings.Contains(trimmed, "<rss") || strings.Contains(trimmed, "<channel") {
		return parseRSSDoc(raw)
	}
	if strings.Contains(trimmed, "<feed") {
		return parseAtomDoc(raw)
	}
	return nil, fmt.Errorf("parsing feed: not Atom, RSS or JSON Feed")
}

// feedToTemplateValue renders a ParsedFeed as the map shape templates index into.
func feedToTemplateValue(f *ParsedFeed) map[string]interface{} {
	items := make([]interface{}, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, feedItemToMap(it))
	}
	return map[string]interface{}{
		"title":         f.Title,
		"home_page_url": f.HomePageURL,
		"items":         items,
	}
}

// feedItemToMap exposes one item to templates. Times stay time.Time so the date
// helpers and sorting work on them.
func feedItemToMap(it FeedItem) map[string]interface{} {
	m := map[string]interface{}{
		"id": it.ID, "url": it.URL, "title": it.Title,
		"summary": it.Summary, "content_html": it.ContentHTML,
		"tags": it.Tags, "author": it.Author,
	}
	if !it.Published.IsZero() {
		m["published"] = it.Published
	}
	if !it.Updated.IsZero() {
		m["updated"] = it.Updated
	}
	if it.Source != "" {
		m["source"] = it.Source
	}
	if it.Label != "" {
		m["label"] = it.Label
	}
	return m
}

// --- Atom -------------------------------------------------------------------

type atomDoc struct {
	Title   string     `xml:"title"`
	Links   []atomLink `xml:"link"`
	Entries []struct {
		ID        string     `xml:"id"`
		Title     string     `xml:"title"`
		Links     []atomLink `xml:"link"`
		Published string     `xml:"published"`
		Updated   string     `xml:"updated"`
		Summary   string     `xml:"summary"`
		Content   string     `xml:"content"`
		Author    struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Categories []struct {
			Term string `xml:"term,attr"`
		} `xml:"category"`
	} `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// alternateHref picks the link a reader would follow: rel="alternate", or the
// first link with no rel, which Atom defines as meaning alternate.
func alternateHref(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "alternate" || l.Rel == "" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

func parseAtomDoc(raw []byte) (*ParsedFeed, error) {
	var doc atomDoc
	if err := xmlUnmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing Atom feed: %w", err)
	}
	out := &ParsedFeed{Title: doc.Title, HomePageURL: alternateHref(doc.Links)}
	for _, e := range doc.Entries {
		it := FeedItem{
			ID: e.ID, URL: alternateHref(e.Links), Title: e.Title,
			Summary: strings.TrimSpace(e.Summary), ContentHTML: strings.TrimSpace(e.Content),
			Author:    strings.TrimSpace(e.Author.Name),
			Published: parseFeedTime(e.Published), Updated: parseFeedTime(e.Updated),
		}
		if it.ID == "" {
			it.ID = it.URL
		}
		for _, c := range e.Categories {
			if c.Term != "" {
				it.Tags = append(it.Tags, c.Term)
			}
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// --- RSS 2.0 ----------------------------------------------------------------

type rssDoc struct {
	Channel struct {
		Title string `xml:"title"`
		Link  string `xml:"link"`
		Items []struct {
			Title       string   `xml:"title"`
			Link        string   `xml:"link"`
			GUID        string   `xml:"guid"`
			PubDate     string   `xml:"pubDate"`
			Description string   `xml:"description"`
			Author      string   `xml:"creator"` // dc:creator, the common author field
			Categories  []string `xml:"category"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseRSSDoc(raw []byte) (*ParsedFeed, error) {
	var doc rssDoc
	if err := xmlUnmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing RSS feed: %w", err)
	}
	out := &ParsedFeed{Title: doc.Channel.Title, HomePageURL: doc.Channel.Link}
	for _, i := range doc.Channel.Items {
		it := FeedItem{
			ID: i.GUID, URL: i.Link, Title: i.Title,
			Summary: strings.TrimSpace(i.Description), Author: strings.TrimSpace(i.Author),
			Published: parseFeedTime(i.PubDate), Tags: nonEmpty(i.Categories),
		}
		if it.ID == "" {
			it.ID = it.URL
		}
		it.Updated = it.Published
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// --- JSON Feed --------------------------------------------------------------

func parseJSONFeedDoc(raw []byte) (*ParsedFeed, error) {
	var doc struct {
		Title       string `json:"title"`
		HomePageURL string `json:"home_page_url"`
		Items       []struct {
			ID            string   `json:"id"`
			URL           string   `json:"url"`
			Title         string   `json:"title"`
			Summary       string   `json:"summary"`
			ContentHTML   string   `json:"content_html"`
			ContentText   string   `json:"content_text"`
			DatePublished string   `json:"date_published"`
			DateModified  string   `json:"date_modified"`
			Tags          []string `json:"tags"`
			Author        struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing JSON feed: %w", err)
	}
	out := &ParsedFeed{Title: doc.Title, HomePageURL: doc.HomePageURL}
	for _, i := range doc.Items {
		summary := i.Summary
		if summary == "" {
			summary = i.ContentText
		}
		it := FeedItem{
			ID: i.ID, URL: i.URL, Title: i.Title, Summary: strings.TrimSpace(summary),
			ContentHTML: strings.TrimSpace(i.ContentHTML), Author: strings.TrimSpace(i.Author.Name),
			Published: parseFeedTime(i.DatePublished), Updated: parseFeedTime(i.DateModified),
			Tags: nonEmpty(i.Tags),
		}
		if it.ID == "" {
			it.ID = it.URL
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// --- shared -----------------------------------------------------------------

// feedTimeLayouts are the encodings feeds actually use. RSS requires RFC 1123Z
// and Atom RFC 3339, but real feeds carry both and several near-misses, so a
// date that fails to parse must not lose the item — it simply has no timestamp.
var feedTimeLayouts = []string{
	time.RFC3339,
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func parseFeedTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range feedTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// xmlUnmarshal decodes an XML document, tolerating the namespace prefixes real
// feeds carry (dc:creator, content:encoded) which strict decoding rejects.
func xmlUnmarshal(raw []byte, v interface{}) error {
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	// Strict=false and HTMLEntity tolerate the namespace prefixes and stray
	// entities real feeds carry. AutoClose is deliberately NOT set: its HTML list
	// includes <link>, and RSS writes the item URL as <link>https://…</link>,
	// which would then be read as an empty self-closing element and take the
	// whole <item> with it.
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	return dec.Decode(v)
}
