package generator

// Selection is a partition: the first spec that matches claims the entry.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestSitemapSpecMatches: each narrowing on its own, and combined with AND.
func TestSitemapSpecMatches(t *testing.T) {
	post := sitemapEntry{kind: kindPost, source: "content/site/posts/blog"}
	tag := sitemapEntry{kind: kindTag}

	cases := []struct {
		name  string
		spec  models.SitemapSpec
		entry sitemapEntry
		want  bool
	}{
		{"a spec with no narrowings claims anything", models.SitemapSpec{}, post, true},
		{"by kind", models.SitemapSpec{Include: []string{"posts"}}, post, true},
		{"by the wrong kind", models.SitemapSpec{Include: []string{"pages"}}, post, false},
		{"kind names are case-insensitive", models.SitemapSpec{Include: []string{"Posts"}}, post, true},
		{"any of several kinds", models.SitemapSpec{Include: []string{"tags", "categories"}}, tag, true},
		{"by source root", models.SitemapSpec{Source: "blog"}, post, true},
		{"by another source root", models.SitemapSpec{Source: "docs"}, post, false},
		{"AND: both must hold", models.SitemapSpec{Source: "blog", Include: []string{"pages"}}, post, false},
		{"AND: both do hold", models.SitemapSpec{Source: "blog", Include: []string{"posts"}}, post, true},
		{"an archive has no source to match", models.SitemapSpec{Source: "blog"}, tag, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sitemapSpecMatches(c.spec, c.entry); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestSitemapSpecPath: a declared path is sanitized into the output tree and
// always ends up an .xml file.
func TestSitemapSpecPath(t *testing.T) {
	cases := map[string]string{
		"/sitemap-blog.xml": "sitemap-blog.xml",
		"sitemap-blog.xml":  "sitemap-blog.xml",
		"sitemap-blog":      "sitemap-blog.xml",
		"maps/blog.xml":     "maps/blog.xml",
		"  ":                "sitemap.xml",
		"../escape.xml":     "escape.xml",
	}
	for in, want := range cases {
		if got := sitemapSpecPath(models.SitemapSpec{Path: in}); got != want {
			t.Errorf("path %q → %q, want %q", in, got, want)
		}
	}
}

// TestValidateSitemapSpecs: a typo in `include:` would otherwise write nothing
// and say only that it matched nothing.
func TestValidateSitemapSpecs(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Sitemaps = []models.SitemapSpec{{Path: "/a.xml", Include: []string{"posts", "artikles"}}}

	err := g.validateSitemapSpecs()
	if err == nil {
		t.Fatal("an unknown kind must be rejected")
	}
	if !strings.Contains(err.Error(), "artikles") || !strings.Contains(err.Error(), "categories") {
		t.Errorf("the error names neither the typo nor the valid set: %v", err)
	}

	g.config.Sitemaps = []models.SitemapSpec{{Path: "/a.xml", Include: []string{"posts", "TAGS"}}}
	if err := g.validateSitemapSpecs(); err != nil {
		t.Errorf("valid kinds were rejected: %v", err)
	}
}

// TestMaxSitemapURLs: configurable downwards, never above the protocol's cap —
// a larger file is invalid whatever the config says.
func TestMaxSitemapURLs(t *testing.T) {
	g := newTestGen(t, "")
	if got := g.maxSitemapURLs(); got != sitemapURLLimit {
		t.Errorf("default = %d, want %d", got, sitemapURLLimit)
	}
	g.config.SitemapMaxURLs = 100
	if got := g.maxSitemapURLs(); got != 100 {
		t.Errorf("configured = %d, want 100", got)
	}
	g.config.SitemapMaxURLs = sitemapURLLimit * 2
	if got := g.maxSitemapURLs(); got != sitemapURLLimit {
		t.Errorf("above the protocol cap = %d, want %d", got, sitemapURLLimit)
	}
}
