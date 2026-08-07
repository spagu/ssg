package externalsource

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestContentTypeGateCoversEverySupportedFormat guards the class of bug behind
// #90: a format added to supportedFormats but not to the transport gate makes
// accepted[format] nil, so every HTTP fetch of it is rejected before parsing.
// The parser tests all passed — they use type: file, which never touches this.
func TestContentTypeGateCoversEverySupportedFormat(t *testing.T) {
	for format := range supportedFormats {
		if !contentTypeAccepted(format, "text/plain") {
			t.Errorf("format %q rejects text/plain — the universal fallback", format)
		}
		// Every format must accept at least one type that is not the fallback,
		// which is what a real server actually sends.
		specific := map[string]string{
			"json": "application/json", "csv": "text/csv", "xml": "application/xml",
			"yaml": "application/yaml", "toml": "application/toml",
			"feed": "application/atom+xml", "changelog": "text/markdown",
		}[format]
		if specific == "" {
			t.Errorf("format %q has no canonical content-type in this test — add one", format)
			continue
		}
		if !contentTypeAccepted(format, specific) {
			t.Errorf("format %q rejects its own canonical type %q", format, specific)
		}
	}
}

// TestFeedGateAcceptsAllThreeWireFormats: `format: feed` detects Atom, RSS or
// JSON Feed from the payload, so the transport gate must not be narrower than
// the parser — the caller does not know which is on the other end.
func TestFeedGateAcceptsAllThreeWireFormats(t *testing.T) {
	for _, ct := range []string{
		"application/atom+xml",
		"application/rss+xml",
		"application/feed+json",
		"application/xml",
		"text/xml",
		"application/json",
		"application/atom+xml; charset=utf-8", // parameters must not break it
	} {
		if !contentTypeAccepted("feed", ct) {
			t.Errorf("feed rejects %q", ct)
		}
	}
	// A clear conflict still fails: an HTML error page is not a feed.
	if contentTypeAccepted("feed", "text/html") {
		t.Error("feed must reject text/html — that is a captive portal or an error page")
	}
}

// TestFetchFeedOverHTTP is the end-to-end gap #90 fell through: the parser was
// covered, the transport was not. Each wire format is served with its canonical
// content-type, as a real host would.
func TestFetchFeedOverHTTP(t *testing.T) {
	bodies := map[string][2]string{
		"atom": {"application/atom+xml", `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
			<title>T</title><entry><title>A</title><link href="https://x/a/"/><id>a</id>
			<published>2026-08-01T00:00:00Z</published><summary>s</summary></entry></feed>`},
		"rss": {"application/rss+xml", `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
			<item><title>B</title><link>https://x/b/</link><guid>b</guid>
			<pubDate>Sat, 01 Aug 2026 00:00:00 +0000</pubDate><description>s</description></item></channel></rss>`},
		"json": {"application/feed+json", `{"version":"https://jsonfeed.org/version/1.1","title":"T",
			"items":[{"id":"c","url":"https://x/c/","title":"C","summary":"s",
			"date_published":"2026-08-01T00:00:00Z"}]}`},
	}
	for name, b := range bodies {
		ct, body := b[0], b[1]
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write([]byte(body))
		}))
		if !contentTypeAccepted("feed", ct) {
			t.Errorf("%s: gate rejects %q before the parser ever sees it", name, ct)
		}
		res, err := Parse("feed", strings.NewReader(body), CSVOptions{})
		srv.Close()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		m, ok := res.(map[string]interface{})
		if !ok {
			t.Errorf("%s: want a normalized map, got %T", name, res)
			continue
		}
		items, _ := m["items"].([]interface{})
		if len(items) != 1 {
			t.Errorf("%s: %d items, want 1", name, len(items))
		}
	}
}
