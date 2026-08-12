package externalsource

// Branch-level tests for the feed normalizer (coverage raise, 1.8.28): broken
// documents, id/summary fallbacks and the provenance fields, per format.

import (
	"strings"
	"testing"
)

// A reader that dies mid-body must surface its error, not an empty feed.
func TestParseFeedReaderError(t *testing.T) {
	if _, err := parseFeed(failingReader{}); err == nil {
		t.Fatal("reader error swallowed")
	}
}

// A payload that is none of Atom/RSS/JSON Feed errors through parseFeed (the
// `format: feed` entry point), not only through ParseFeedBytes.
func TestParseFeedRejectsNonFeed(t *testing.T) {
	if _, err := parseFeed(strings.NewReader("just some text")); err == nil {
		t.Fatal("non-feed payload accepted")
	}
}

// Truncated XML is a parse error attributed to the detected format, so the
// user knows which parser gave up.
func TestParseFeedTruncatedXML(t *testing.T) {
	cases := map[string]string{
		"<feed": "Atom",
		"<rss":  "RSS",
	}
	for in, format := range cases {
		_, err := ParseFeedBytes([]byte(in))
		if err == nil || !strings.Contains(err.Error(), format) {
			t.Errorf("ParseFeedBytes(%q) err = %v, want a %s parse error", in, err, format)
		}
	}
}

// Malformed JSON (a { prefix routes it to the JSON Feed parser) errors rather
// than yielding an empty feed.
func TestParseFeedMalformedJSON(t *testing.T) {
	if _, err := ParseFeedBytes([]byte("{ not json")); err == nil {
		t.Fatal("malformed JSON feed accepted")
	}
}

// An Atom entry without <id> falls back to its alternate link, so every item
// keeps a stable identifier.
func TestAtomEntryIDFallsBackToURL(t *testing.T) {
	raw := `<feed xmlns="http://www.w3.org/2005/Atom">
  <title>T</title>
  <entry><title>A</title><link href="https://e.example/a/"/></entry>
</feed>`
	f, err := ParseFeedBytes([]byte(raw))
	if err != nil || len(f.Items) != 1 {
		t.Fatalf("parse: %+v %v", f, err)
	}
	if f.Items[0].ID != "https://e.example/a/" {
		t.Errorf("ID = %q, want the entry URL", f.Items[0].ID)
	}
}

// An RSS item without <guid> falls back to its <link> the same way.
func TestRSSItemIDFallsBackToLink(t *testing.T) {
	raw := `<rss version="2.0"><channel><title>R</title>
  <item><title>B</title><link>https://e.example/b/</link></item>
</channel></rss>`
	f, err := ParseFeedBytes([]byte(raw))
	if err != nil || len(f.Items) != 1 {
		t.Fatalf("parse: %+v %v", f, err)
	}
	it := f.Items[0]
	if it.ID != "https://e.example/b/" || it.URL != it.ID {
		t.Errorf("item = %+v, want ID to fall back to the link", it)
	}
}

// JSON Feed: a missing summary borrows content_text and a missing id borrows
// the url — an item never comes out blank where the feed had the information.
func TestJSONFeedFieldFallbacks(t *testing.T) {
	raw := `{"title":"JF","items":[{"url":"https://e.example/c/","content_text":" plain body "}]}`
	f, err := ParseFeedBytes([]byte(raw))
	if err != nil || len(f.Items) != 1 {
		t.Fatalf("parse: %+v %v", f, err)
	}
	it := f.Items[0]
	if it.ID != "https://e.example/c/" {
		t.Errorf("ID = %q, want the item URL", it.ID)
	}
	if it.Summary != "plain body" {
		t.Errorf("Summary = %q, want the trimmed content_text", it.Summary)
	}
}

// alternateHref prefers rel=alternate or no rel; with neither present the
// first link still beats returning nothing.
func TestAlternateHrefFallsBackToFirstLink(t *testing.T) {
	links := []atomLink{
		{Href: "https://e.example/self", Rel: "self"},
		{Href: "https://e.example/hub", Rel: "hub"},
	}
	if got := alternateHref(links); got != "https://e.example/self" {
		t.Errorf("alternateHref = %q, want the first link", got)
	}
	if got := alternateHref(nil); got != "" {
		t.Errorf("alternateHref(nil) = %q, want empty", got)
	}
}

// Provenance: source and label appear in the template map only when set, so a
// single-feed template sees no phantom keys.
func TestFeedItemToMapProvenance(t *testing.T) {
	with := feedItemToMap(FeedItem{ID: "a", Source: "src", Label: "Blog"})
	if with["source"] != "src" || with["label"] != "Blog" {
		t.Errorf("provenance not exposed: %v", with)
	}
	without := feedItemToMap(FeedItem{ID: "a"})
	if _, ok := without["source"]; ok {
		t.Error("empty source must not appear in the map")
	}
	if _, ok := without["label"]; ok {
		t.Error("empty label must not appear in the map")
	}
}
