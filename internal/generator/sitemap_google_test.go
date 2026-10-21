package generator

// The sitemap against Google's "Build and submit a sitemap" and sitemaps.org:
// the size limits, UTF-8, entity escaping, absolute URLs, an accurate
// lastmod, and the robots.txt reference.

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSitemapLimitsAreTheProtocols: 50,000 URLs and 50 MB (52,428,800 bytes)
// per file, 50,000 files per index.
func TestSitemapLimitsAreTheProtocols(t *testing.T) {
	if sitemapURLLimit != 50000 || sitemapByteLimit != 52428800 || sitemapIndexLimit != 50000 {
		t.Errorf("limits = %d URLs, %d bytes, %d files", sitemapURLLimit, sitemapByteLimit, sitemapIndexLimit)
	}
}

// TestEverySitemapFileStaysUnderBothLimits: a set over 50 MB but under 50,000
// URLs — long URLs, as a site with deep paths has — is split by size, and no
// file it writes exceeds either limit.
func TestEverySitemapFileStaysUnderBothLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("renders ~60 MB")
	}
	g := newTestGen(t, "")
	g.config.Quiet = true
	long := strings.Repeat("segment/", 250) // ~2 KB per URL
	entries := make([]sitemapEntry, 30000)
	for i := range entries {
		entries[i] = sitemapEntry{loc: "https://example.com/" + long + strings.Repeat("x", i%7) + "/", changefreq: "weekly", priority: "0.5"}
	}
	files := g.splitSitemap(sitemapFile{path: "sitemap.xml", entries: entries})
	if len(files) < 2 {
		t.Fatalf("~60 MB of URLs must be split by size, got %d file(s)", len(files))
	}
	total := 0
	for _, f := range files {
		size := len(g.renderURLSet(f.entries))
		if size > sitemapByteLimit || len(f.entries) > sitemapURLLimit {
			t.Errorf("%s: %d bytes, %d URLs", f.path, size, len(f.entries))
		}
		total += len(f.entries)
	}
	if total != len(entries) {
		t.Errorf("split lost URLs: %d of %d", total, len(entries))
	}
}

// TestChunkSitemapByCountAndBytes: the two limits, each on its own.
func TestChunkSitemapByCountAndBytes(t *testing.T) {
	e := sitemapEntry{loc: "https://example.com/a/", changefreq: "weekly", priority: "0.5"}
	var sb strings.Builder
	e.writeTo(&sb)
	one := sb.Len()
	entries := []sitemapEntry{e, e, e, e, e}
	if c := chunkSitemap(entries, 2, 1<<30); len(c) != 3 || len(c[0]) != 2 || len(c[2]) != 1 {
		t.Errorf("by count: %d chunks", len(c))
	}
	if c := chunkSitemap(entries, 100, sitemapFrameBytes+2*one); len(c) != 3 || len(c[0]) != 2 {
		t.Errorf("by bytes: %d chunks", len(c))
	}
	if c := chunkSitemap(entries, 100, 1); len(c) != 5 {
		t.Errorf("an entry bigger than the limit still gets a file of its own: %d chunks", len(c))
	}
}

// TestSitemapLocIsEscapedAndASCII: "&" in a query and a non-ASCII path are
// written so the file is XML and the URL is ASCII.
func TestSitemapLocIsEscapedAndASCII(t *testing.T) {
	cases := map[string]string{
		"https://example.com/search/?a=1&b=2": "https://example.com/search/?a=1&amp;b=2",
		"https://example.com/usługi/":         "https://example.com/us%C5%82ugi/",
		"https://example.com/a b/'x'":         "https://example.com/a%20b/&#39;x&#39;",
		"https://example.com/%C5%82/":         "https://example.com/%C5%82/",
	}
	for in, want := range cases {
		if got := sitemapLoc(in); got != want {
			t.Errorf("sitemapLoc(%q) = %q, want %q", in, got, want)
		}
	}
}

// urlset is enough of a sitemap to check it parses.
type urlset struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc     string `xml:"loc"`
		Lastmod string `xml:"lastmod"`
	} `xml:"url"`
}

// TestBuiltSitemapFollowsTheGuidelines: a whole build's sitemap is UTF-8 XML
// that parses, lists only absolute URLs, escapes a query, encodes a
// non-ASCII path, dates its hubs from what they list (#339), and robots.txt
// points at it with an absolute URL.
func TestBuiltSitemapFollowsTheGuidelines(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	writeTaxonomyPost(t, postsDir, "one", "One", "technology: [Go]\ntags: [cli]\nmodified: 2025-03-04\n")
	writeTaxonomyPost(t, postsDir, "two", "Two", "technology: [go]\nmodified: 2024-06-01\n")
	pages := filepath.Join(tmp, "content", "site", "pages")
	mustWrite(t, filepath.Join(pages, "uslugi.md"),
		"---\ntitle: Usługi\nslug: uslugi\nlink: \"/usługi/\"\nstatus: publish\ntype: page\ndate: 2024-02-01\n---\nx\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, err := New(taxonomyTestConfig(tmp))
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	raw := mustRead(t, filepath.Join(out, "sitemap.xml"))
	if !strings.HasPrefix(raw, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Error("the sitemap must declare UTF-8")
	}
	var set urlset
	if err := xml.Unmarshal([]byte(raw), &set); err != nil {
		t.Fatalf("the sitemap is not XML: %v", err)
	}
	locs := map[string]string{}
	for _, u := range set.URLs {
		if !strings.HasPrefix(u.Loc, "https://example.com/") {
			t.Errorf("not an absolute URL: %q", u.Loc)
		}
		if u.Lastmod != "" {
			if _, err := time.Parse(sitemapDateLayout, u.Lastmod); err != nil {
				t.Errorf("lastmod %q is not W3C", u.Lastmod)
			}
		}
		locs[u.Loc] = u.Lastmod
	}
	if _, ok := locs["https://example.com/us%C5%82ugi/"]; !ok {
		t.Errorf("the non-ASCII path is not percent-encoded: %v", locs)
	}
	// Hubs take the newest lastmod of what they list: post one was modified
	// 2025-03-04, later than post two's 2024-06-01.
	for _, hub := range []string{"https://example.com/", "https://example.com/technology/", "https://example.com/technology/go/", "https://example.com/tag/cli/"} {
		if got := locs[hub]; got != "2025-03-04" {
			t.Errorf("%s lastmod = %q, want 2025-03-04", hub, got)
		}
	}
	robots := mustRead(t, filepath.Join(out, "robots.txt"))
	if !strings.Contains(robots, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("robots.txt must name the sitemap by absolute URL:\n%s", robots)
	}
}

// TestSitemapIndexOmitsAnUnknownLastmod: an index entry carries the newest
// date of its file, and no date rather than the build time.
func TestSitemapIndexOmitsAnUnknownLastmod(t *testing.T) {
	g := newTestGen(t, "")
	dated := sitemapFile{path: "a.xml", entries: []sitemapEntry{{lastmod: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)}}}
	undated := sitemapFile{path: "b&c.xml", entries: []sitemapEntry{{}}}
	idx := string(g.renderSitemapIndex([]sitemapFile{dated, undated}))
	if strings.Count(idx, "<lastmod>") != 1 || !strings.Contains(idx, "<lastmod>2025-01-02</lastmod>") {
		t.Errorf("index:\n%s", idx)
	}
	if !strings.Contains(idx, "<loc>https://example.com/b&amp;c.xml</loc>") {
		t.Errorf("an index <loc> is escaped too:\n%s", idx)
	}
}

// TestTooManySitemapsFails: more files than one index may name is an error,
// not an index crawlers reject, and nothing is written.
func TestTooManySitemapsFails(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Quiet = true
	g.config.SitemapMaxURLs = 1
	entries := make([]sitemapEntry, sitemapIndexLimit+1)
	for i := range entries {
		entries[i] = sitemapEntry{loc: "https://example.com/p/"}
	}
	err := g.writeSitemaps(entries)
	if err == nil || !strings.Contains(err.Error(), "more than the 50000") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(g.config.OutputDir, "sitemap.xml")); statErr == nil {
		t.Error("nothing may be written when the set cannot be indexed")
	}
}
