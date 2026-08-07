package externalsource

import (
	"strings"
	"testing"
)

const atomSample = `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>SSG</title>
  <link href="https://ssg.example.com/" rel="alternate"/>
  <entry>
    <title>Post A</title>
    <link href="https://ssg.example.com/a/"/>
    <id>urn:a</id>
    <published>2026-08-03T00:00:00Z</published>
    <updated>2026-08-04T00:00:00Z</updated>
    <author><name>Ed</name></author>
    <category term="performance"/>
    <summary>Summary A.</summary>
  </entry>
</feed>`

// RSS writes the item URL as <link>text</link>. An XML decoder with HTML
// auto-close treats <link> as self-closing and loses the whole <item> with it —
// which is exactly how the first version of this parser dropped every RSS entry
// while Atom kept working.
const rssSample = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title>MDDB</title><link>https://mddb.example.com/</link>
  <item>
    <title>Post B</title>
    <link>https://mddb.example.com/b/</link>
    <guid>https://mddb.example.com/b/</guid>
    <pubDate>Tue, 05 Aug 2026 10:00:00 +0000</pubDate>
    <category>database</category>
    <description>Summary B.</description>
  </item>
</channel></rss>`

const jsonSample = `{"version":"https://jsonfeed.org/version/1.1","title":"JF",
 "home_page_url":"https://jf.example.com/",
 "items":[{"id":"c","url":"https://jf.example.com/c/","title":"Post C",
           "summary":"Summary C.","date_published":"2026-08-06T00:00:00Z","tags":["go"]}]}`

// TestParseFeedDetectsFormat covers #89: the format is detected from the payload,
// not from a declaration — a .xml URL may be Atom or RSS, and a redirect can
// change what actually arrives.
func TestParseFeedDetectsFormat(t *testing.T) {
	for name, raw := range map[string]string{"atom": atomSample, "rss": rssSample, "json": jsonSample} {
		f, err := ParseFeedBytes([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(f.Items) != 1 {
			t.Fatalf("%s: %d items, want 1 — an RSS <link> eaten by auto-close looks exactly like this", name, len(f.Items))
		}
		it := f.Items[0]
		if it.Title == "" || it.URL == "" || it.Summary == "" {
			t.Errorf("%s: incomplete item %+v", name, it)
		}
		if it.Published.IsZero() {
			t.Errorf("%s: date not parsed — items from different feeds cannot sort against each other", name)
		}
		if len(it.Tags) != 1 {
			t.Errorf("%s: tags = %v, want one", name, it.Tags)
		}
	}
}

// TestParseFeedNormalizesAcrossFormats: the three formats produce one shape, so a
// template does not have to know which is on the other end.
func TestParseFeedNormalizesAcrossFormats(t *testing.T) {
	for _, raw := range []string{atomSample, rssSample, jsonSample} {
		v, err := Parse("feed", strings.NewReader(raw), CSVOptions{})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("want a map, got %T", v)
		}
		items, ok := m["items"].([]interface{})
		if !ok || len(items) != 1 {
			t.Fatalf("items = %v", m["items"])
		}
		first, _ := items[0].(map[string]interface{})
		for _, key := range []string{"title", "url", "summary", "published"} {
			if _, ok := first[key]; !ok {
				t.Errorf("normalized item missing %q: %v", key, first)
			}
		}
	}
	// Anything that is not a feed is an error, not an empty result.
	if _, err := ParseFeedBytes([]byte("<html><body>no</body></html>")); err == nil {
		t.Error("a non-feed document must error")
	}
}

// TestParseFeedTime accepts what feeds actually carry, and never loses an item to
// a date it cannot read.
func TestParseFeedTime(t *testing.T) {
	for _, s := range []string{
		"2026-08-03T00:00:00Z",
		"Tue, 05 Aug 2026 10:00:00 +0000",
		"2026-08-03",
	} {
		if parseFeedTime(s).IsZero() {
			t.Errorf("parseFeedTime(%q) failed", s)
		}
	}
	if !parseFeedTime("last thursday").IsZero() {
		t.Error("an unreadable date must yield the zero time, not a guess")
	}
}
