package generator

// One sitemap.xml holding everything answers neither question a growing site
// asks: above 50,000 URLs the protocol requires an index, and Search Console
// reports indexing coverage per submitted sitemap.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// planSite builds a site with four posts under two content roots, one page, a
// category and a tag, and returns its output directory.
func planSite(t *testing.T, apply func(cfg *Config)) Config {
	t.Helper()
	files := map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nB.\n",
	}
	for _, n := range []string{"1", "2", "3"} {
		files["posts/blog/b"+n+".md"] = "---\ntitle: B" + n + "\nslug: b" + n +
			"\nstatus: publish\ntype: post\ndate: 2024-01-0" + n + "\ncategories: [News]\ntags: [alpha]\n---\n\nB.\n"
	}
	files["posts/docs/d1.md"] = "---\ntitle: D1\nslug: d1\nstatus: publish\ntype: post\ndate: 2024-02-01\n---\n\nB.\n"

	cfg := newSiteFixture(t, `{"categories":[{"id":2,"name":"News","slug":"news"}],"media":[],"users":[]}`, files, nil)
	if apply != nil {
		apply(&cfg)
	}
	buildSiteFixture(t, cfg)
	return cfg
}

// locsIn returns the <loc> values of one written sitemap file.
func locsIn(t *testing.T, cfg Config, name string) []string {
	t.Helper()
	raw := mustRead(t, filepath.Join(cfg.OutputDir, name))
	var out []string
	for _, part := range strings.Split(raw, "<loc>")[1:] {
		out = append(out, part[:strings.Index(part, "</loc>")])
	}
	return out
}

// TestOneSitemapWhenNothingAsksOtherwise: the common case must not change —
// one file, no index, and no new files beside it.
func TestOneSitemapWhenNothingAsksOtherwise(t *testing.T) {
	cfg := planSite(t, nil)

	raw := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	if !strings.Contains(raw, "<urlset") || strings.Contains(raw, "<sitemapindex") {
		t.Errorf("a small site should still write one urlset:\n%s", raw)
	}
	matches, _ := filepath.Glob(filepath.Join(cfg.OutputDir, "sitemap-*.xml"))
	if len(matches) != 0 {
		t.Errorf("unexpected extra sitemaps: %v", matches)
	}
}

// TestDeclaredSitemapsPartitionTheURLs: an entry lands in the first spec that
// matches it, and whatever matches none is still published — no URL is listed
// twice, none is lost.
func TestDeclaredSitemapsPartitionTheURLs(t *testing.T) {
	cfg := planSite(t, func(cfg *Config) {
		cfg.Sitemaps = []models.SitemapSpec{
			{Path: "/sitemap-blog.xml", Source: "blog"},
			{Path: "/sitemap-archives.xml", Include: []string{"categories", "tags", "authors"}},
		}
	})

	blog := locsIn(t, cfg, "sitemap-blog.xml")
	if len(blog) != 3 {
		t.Errorf("the blog sitemap has %d URLs, want 3: %v", len(blog), blog)
	}
	archives := locsIn(t, cfg, "sitemap-archives.xml")
	if len(archives) != 2 {
		t.Errorf("the archive sitemap has %d URLs, want 2: %v", len(archives), archives)
	}
	main := locsIn(t, cfg, "sitemap-main.xml")
	for _, want := range []string{"https://example.com/", "https://example.com/about/"} {
		if !listsURL(main, want) {
			t.Errorf("%s fell out of every sitemap: %v", want, main)
		}
	}
	// The docs post matched no spec and belongs to the remainder, not the blog.
	if listsURL(blog, "https://example.com/2024/02/01/d1/") {
		t.Error("a post outside the declared source was claimed by it")
	}

	// Every URL appears exactly once across the set.
	seen := map[string]int{}
	for _, name := range []string{"sitemap-blog.xml", "sitemap-archives.xml", "sitemap-main.xml"} {
		for _, loc := range locsIn(t, cfg, name) {
			seen[loc]++
		}
	}
	for loc, n := range seen {
		if n != 1 {
			t.Errorf("%s is listed %d times", loc, n)
		}
	}
}

// TestSitemapIndexNamesEveryFile: sitemap.xml becomes the index, which is what
// robots.txt and Search Console already point at.
func TestSitemapIndexNamesEveryFile(t *testing.T) {
	cfg := planSite(t, func(cfg *Config) {
		cfg.Sitemaps = []models.SitemapSpec{{Path: "/sitemap-blog.xml", Source: "blog"}}
	})

	index := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	if !strings.Contains(index, "<sitemapindex") {
		t.Fatalf("sitemap.xml is not an index:\n%s", index)
	}
	for _, want := range []string{
		"https://example.com/sitemap-blog.xml",
		"https://example.com/sitemap-main.xml",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the index does not name %s:\n%s", want, index)
		}
	}
	if !strings.Contains(index, "<lastmod>") {
		t.Error("an index entry should carry a lastmod")
	}
	// robots.txt still points at the same address, which is the point of
	// keeping the index under the old name.
	robots := mustRead(t, filepath.Join(cfg.OutputDir, "robots.txt"))
	if !strings.Contains(robots, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("robots.txt moved:\n%s", robots)
	}
}

// TestSitemapSplitsAboveTheCeiling: a file over the limit is invalid, so it is
// split and indexed rather than written.
func TestSitemapSplitsAboveTheCeiling(t *testing.T) {
	cfg := planSite(t, func(cfg *Config) { cfg.SitemapMaxURLs = 2; cfg.Quiet = true })

	parts, _ := filepath.Glob(filepath.Join(cfg.OutputDir, "sitemap-*.xml"))
	if len(parts) < 2 {
		t.Fatalf("nothing was split: %v", parts)
	}
	total := 0
	for _, p := range parts {
		locs := locsIn(t, cfg, filepath.Base(p))
		if len(locs) > 2 {
			t.Errorf("%s holds %d URLs, over the ceiling", filepath.Base(p), len(locs))
		}
		total += len(locs)
	}
	index := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	if !strings.Contains(index, "<sitemapindex") {
		t.Error("a split set needs an index")
	}
	for _, p := range parts {
		if !strings.Contains(index, filepath.Base(p)) {
			t.Errorf("the index does not name %s", filepath.Base(p))
		}
	}
	// Nothing is lost in the split.
	one := planSite(t, nil)
	if want := len(locsIn(t, one, "sitemap.xml")); total != want {
		t.Errorf("split holds %d URLs, unsplit %d", total, want)
	}
}

// TestEmptyDeclaredSitemapIsNotWritten: an index entry pointing at a file with
// no URLs is a fetch that teaches a crawler nothing.
func TestEmptyDeclaredSitemapIsNotWritten(t *testing.T) {
	cfg := planSite(t, func(cfg *Config) {
		cfg.Sitemaps = []models.SitemapSpec{{Path: "/sitemap-nothing.xml", Source: "no-such-root"}}
	})

	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "sitemap-nothing.xml")); err == nil {
		t.Error("an empty sitemap was written")
	}
	// And with nothing else declared, the site is back to a single file.
	raw := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	if strings.Contains(raw, "<sitemapindex") {
		t.Errorf("one file needs no index:\n%s", raw)
	}
}

// listsURL reports whether a sitemap's <loc> values include one address.
func listsURL(locs []string, want string) bool {
	for _, loc := range locs {
		if loc == want {
			return true
		}
	}
	return false
}

// TestSplitSitemapEdges: the boundaries the e2e builds do not reach.
func TestSplitSitemapEdges(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Quiet = true
	entries := func(n int) []sitemapEntry {
		out := make([]sitemapEntry, n)
		for i := range out {
			out[i] = sitemapEntry{loc: "https://example.com/x/"}
		}
		return out
	}

	// Nothing to write is no file, not an empty one.
	if got := g.splitSitemap(sitemapFile{path: "a.xml"}); got != nil {
		t.Errorf("an empty file was planned: %v", got)
	}
	// Exactly at the ceiling stays whole: the limit is inclusive.
	g.config.SitemapMaxURLs = 3
	if got := g.splitSitemap(sitemapFile{path: "a.xml", entries: entries(3)}); len(got) != 1 || got[0].path != "a.xml" {
		t.Errorf("a file at the ceiling was split: %v", got)
	}
	// One over splits, and the parts are numbered from one.
	got := g.splitSitemap(sitemapFile{path: "a.xml", entries: entries(4)})
	if len(got) != 2 || got[0].path != "a-1.xml" || got[1].path != "a-2.xml" {
		t.Fatalf("split = %v", got)
	}
	if len(got[0].entries) != 3 || len(got[1].entries) != 1 {
		t.Errorf("uneven split: %d and %d", len(got[0].entries), len(got[1].entries))
	}
}

// TestSitemapFileLastmod: an index entry always carries a date — the newest of
// the file's own, or the build time when the file has none.
func TestSitemapFileLastmod(t *testing.T) {
	built := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	f := sitemapFile{entries: []sitemapEntry{{lastmod: older}, {lastmod: newer}}}
	if got := f.lastmod(built); !got.Equal(newer) {
		t.Errorf("lastmod = %v, want the newest entry %v", got, newer)
	}
	bare := sitemapFile{entries: []sitemapEntry{{}}}
	if got := bare.lastmod(built); !got.Equal(built) {
		t.Errorf("lastmod = %v, want the build time %v", got, built)
	}
}

// TestEmptyDeclaredSitemapIsReported: a spec that selected nothing is usually a
// typo, and the build says so rather than quietly writing one file fewer.
func TestEmptyDeclaredSitemapIsReported(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Sitemaps = []models.SitemapSpec{{Path: "/none.xml", Source: "nowhere"}}

	var files []sitemapFile
	out := captureBuildOutput(t, func() {
		files, _ = g.partitionSitemaps([]sitemapEntry{{loc: "https://example.com/", kind: kindHome}})
	})
	if len(files) != 0 {
		t.Errorf("an empty file was kept: %v", files)
	}
	if !strings.Contains(out, "none.xml") || !strings.Contains(out, "matched no URLs") {
		t.Errorf("the empty spec was not reported:\n%s", out)
	}
}
