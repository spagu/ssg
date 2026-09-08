package generator

// `taxonomies: { tag: { sitemap: false } }` parsed, validated and did nothing
// (#259): the folded built-ins are listed by the legacy loops, and only
// writeTaxonomySitemap consulted the flag — the one place that skips them.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/taxonomy"
)

// buildTaxonomyFlagSite builds one post carrying a tag and a category, with the
// taxonomy block the test wants.
func buildTaxonomyFlagSite(t *testing.T, taxonomies map[string]taxonomy.DefinitionConfig) string {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[{"id":2,"name":"News","slug":"news"}],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\n" +
				"date: 2024-01-02\ncategories: [News]\ntags: [alpha]\n---\n\nBody.\n",
		}, nil)
	cfg.Taxonomies = taxonomies
	buildSiteFixture(t, cfg)
	return mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
}

// TestTaxonomySitemapFalseIsHonoured: the archive is still written — `archive`
// is a separate switch — but it stops being advertised.
func TestTaxonomySitemapFalseIsHonoured(t *testing.T) {
	off := false
	sitemap := buildTaxonomyFlagSite(t, map[string]taxonomy.DefinitionConfig{
		"tag": {Sitemap: &off},
	})

	if strings.Contains(sitemap, "/tag/alpha/") {
		t.Errorf("sitemap: false was ignored for tag:\n%s", sitemap)
	}
	// Only the one that asked: the category archive is untouched.
	if !strings.Contains(sitemap, "/category/news/") {
		t.Errorf("the category archive disappeared with it:\n%s", sitemap)
	}
}

// TestCategorySitemapFalseIsHonoured: the same switch, the other legacy loop.
func TestCategorySitemapFalseIsHonoured(t *testing.T) {
	off := false
	sitemap := buildTaxonomyFlagSite(t, map[string]taxonomy.DefinitionConfig{
		"category": {Sitemap: &off},
	})

	if strings.Contains(sitemap, "/category/news/") {
		t.Errorf("sitemap: false was ignored for category:\n%s", sitemap)
	}
	if !strings.Contains(sitemap, "/tag/alpha/") {
		t.Errorf("the tag archive disappeared with it:\n%s", sitemap)
	}
}

// TestTaxonomySitemapDefaultsToListed: the flag is opt-out, so a site that says
// nothing keeps every archive it had.
func TestTaxonomySitemapDefaultsToListed(t *testing.T) {
	sitemap := buildTaxonomyFlagSite(t, nil)

	for _, want := range []string{"/tag/alpha/", "/category/news/"} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("%s is missing from the default sitemap:\n%s", want, sitemap)
		}
	}
}

// TestTaxonomySitemapEnabledForUnknownNames: author is driven outside the
// registry (#44), so it has no definition and keeps the behaviour it had.
func TestTaxonomySitemapEnabledForUnknownNames(t *testing.T) {
	g := newTestGen(t, "")
	if !g.taxonomySitemapEnabled("author") {
		t.Error("a name the registry does not define must stay listed")
	}
	if !g.taxonomySitemapEnabled("tag") {
		t.Error("without a registry at all, nothing may be suppressed")
	}
}

// TestTaxonomySitemapEnabledReadsTheDefinition: the flag straight from the
// registry, both ways.
func TestTaxonomySitemapEnabledReadsTheDefinition(t *testing.T) {
	g := newTestGen(t, "")
	off, on := false, true
	defs, names, err := taxonomy.Resolve(map[string]taxonomy.DefinitionConfig{
		"tag":      {Sitemap: &off},
		"category": {Sitemap: &on},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	g.taxonomies = taxonomy.NewRegistry(defs, names, nil)
	if g.taxonomySitemapEnabled("tag") {
		t.Error("sitemap: false must disable listing")
	}
	if !g.taxonomySitemapEnabled("category") {
		t.Error("sitemap: true must keep listing")
	}
}
