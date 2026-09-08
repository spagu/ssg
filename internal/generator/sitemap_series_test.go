package generator

// Series archives were written, linked from every post in the series, and named
// in no sitemap at all (#261): the registry skips folded built-ins because they
// are meant to be listed by their legacy loop, and series had no legacy loop.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// buildSeriesSite builds two posts in one series and one in another.
func buildSeriesSite(t *testing.T, apply func(cfg *Config)) Config {
	t.Helper()
	files := map[string]string{}
	for i, spec := range []struct{ name, series string }{
		{"one", "Getting Started"},
		{"two", "Getting Started"},
		{"three", "Going Deeper"},
	} {
		files["posts/news/"+spec.name+".md"] = "---\ntitle: P\nslug: " + spec.name +
			"\nstatus: publish\ntype: post\ndate: 2024-01-0" + string(rune('1'+i)) +
			"\nseries: " + spec.series + "\n---\n\nBody.\n"
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, nil)
	if apply != nil {
		apply(&cfg)
	}
	buildSiteFixture(t, cfg)
	return cfg
}

// TestSeriesArchivesAreListed: the archive exists and the sitemap must say so —
// the invariant #228 set, applied to the one built-in that never had it.
func TestSeriesArchivesAreListed(t *testing.T) {
	cfg := buildSeriesSite(t, nil)
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))

	for _, slug := range []string{"getting-started", "going-deeper"} {
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, "series", slug, indexHTMLName)); err != nil {
			t.Fatalf("fixture did not render /series/%s/: %v", slug, err)
		}
		if !strings.Contains(sitemap, "https://example.com/series/"+slug+"/") {
			t.Errorf("/series/%s/ is written but not listed:\n%s", slug, sitemap)
		}
	}
	// One entry each, not one per post in the series.
	if n := strings.Count(sitemap, "/series/getting-started/"); n != 1 {
		t.Errorf("the series archive is listed %d times, want 1", n)
	}
	// There is no /series/ index page, so nothing may claim there is.
	if strings.Contains(sitemap, "<loc>https://example.com/series/</loc>") {
		t.Error("the sitemap names a series index that the build does not write")
	}
}

// TestSeriesSitemapCanBeTurnedOff: series is a registry taxonomy like the
// others, so the switch #259 fixed applies to it too.
func TestSeriesSitemapCanBeTurnedOff(t *testing.T) {
	off := false
	cfg := buildSeriesSite(t, func(cfg *Config) {
		cfg.Taxonomies = map[string]taxonomy.DefinitionConfig{"series": {Sitemap: &off}}
	})
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))

	if strings.Contains(sitemap, "/series/") {
		t.Errorf("sitemap: false was ignored for series:\n%s", sitemap)
	}
	// Still written: `archive` is the switch that decides that.
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "series", "getting-started", indexHTMLName)); err != nil {
		t.Errorf("the archive should still be rendered: %v", err)
	}
}

// TestSeriesArchiveSuppressedByAPageIsNotListed: an explicit document owning the
// URL wins (GO-050), and an archive that was never written must not be
// advertised — the same rule the tag map follows.
func TestSeriesArchiveSuppressedByAPageIsNotListed(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"posts/news/one.md": "---\ntitle: P\nslug: one\nstatus: publish\ntype: post\n" +
			"date: 2024-01-01\nseries: Getting Started\n---\n\nBody.\n",
		"pages/hand.md": "---\ntitle: Hand written\nslug: getting-started\nstatus: publish\n" +
			"type: page\nlink: \"/series/getting-started/\"\n---\n\nMine.\n",
	}, nil)
	cfg.Quiet = true
	buildSiteFixture(t, cfg)
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))

	// Listed once — as the page that owns the URL, never twice.
	if n := strings.Count(sitemap, "/series/getting-started/"); n > 1 {
		t.Errorf("the suppressed archive was advertised as well:\n%s", sitemap)
	}
}

// TestSeriesIsSelectableByName: series is a kind like any other, so a declared
// sitemap can gather the archives into a file of their own.
func TestSeriesIsSelectableByName(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Sitemaps = []models.SitemapSpec{{Path: "/sitemap-series.xml", Include: []string{"series"}}}
	if err := g.validateSitemapSpecs(); err != nil {
		t.Fatalf("series is not a valid include name: %v", err)
	}
	if !sitemapSpecMatches(g.config.Sitemaps[0], sitemapEntry{kind: kindSeries}) {
		t.Error("a series entry is not claimed by a spec including it")
	}
}

// TestSeriesWithNoUsableSlug: a series whose name slugifies to nothing has no
// URL to write, so it is skipped rather than recorded — the sitemap may only
// name archives the build produced.
func TestSeriesWithNoUsableSlug(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"posts/news/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: post\n" +
			"date: 2024-01-01\nseries: \"!!!\"\n---\n\nBody.\n",
		"posts/news/b.md": "---\ntitle: B\nslug: b\nstatus: publish\ntype: post\n" +
			"date: 2024-01-02\nseries: Real One\n---\n\nBody.\n",
	}, nil)
	buildSiteFixture(t, cfg)
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))

	if !strings.Contains(sitemap, "/series/real-one/") {
		t.Errorf("the usable series is missing:\n%s", sitemap)
	}
	// Nothing was written for the unusable one, so nothing may name it.
	if strings.Contains(sitemap, "<loc>https://example.com/series//</loc>") {
		t.Errorf("an empty series slug reached the sitemap:\n%s", sitemap)
	}
	if entries, err := os.ReadDir(filepath.Join(cfg.OutputDir, "series")); err == nil {
		if len(entries) != 1 {
			t.Errorf("series archives written: %d, want 1", len(entries))
		}
	}
}
