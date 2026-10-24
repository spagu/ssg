package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestPageSitemapHints(t *testing.T) {
	g := &Generator{config: Config{Quiet: true}}
	for in, want := range map[string]string{"": "monthly", "Daily": "daily", " never ": "never", "fortnightly": "monthly"} {
		if got := g.pageChangefreq(models.Page{SitemapChangefreq: in}, "monthly"); got != want {
			t.Errorf("changefreq %q = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"": "0.8", "0.3": "0.3", "1": "1.0", "0": "0.0", "0.85": "0.85", "1.5": "0.8", "high": "0.8"} {
		if got := g.pagePriority(models.Page{SitemapPriority: in}, "0.8"); got != want {
			t.Errorf("priority %q = %q, want %q", in, got, want)
		}
	}
	// Not quiet: the warning names the file and does not stop anything.
	loud := &Generator{}
	if loud.pageChangefreq(models.Page{SourceFile: "a.md", SitemapChangefreq: "x"}, "weekly") != "weekly" ||
		loud.pagePriority(models.Page{Link: "/a/", SitemapPriority: "x"}, "0.5") != "0.5" {
		t.Error("invalid hints keep the default")
	}
}

// TestFrontmatterSitemapHints: front matter reaches the sitemap.
func TestFrontmatterSitemapHints(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/freshness.md": "---\ntitle: Freshness\nslug: freshness\nstatus: publish\ntype: page\n" +
			"sitemap_changefreq: daily\nsitemap_priority: 0.9\n---\n\nRecounted daily.\n",
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}, nil)
	buildSiteFixture(t, cfg)
	sm := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	fresh := sm[strings.Index(sm, "/freshness/"):]
	fresh = fresh[:strings.Index(fresh, "</url>")]
	if !strings.Contains(fresh, "<changefreq>daily</changefreq>") || !strings.Contains(fresh, "<priority>0.9</priority>") {
		t.Errorf("freshness entry:\n%s", fresh)
	}
	about := sm[strings.Index(sm, "/about/"):]
	about = about[:strings.Index(about, "</url>")]
	if !strings.Contains(about, "<changefreq>monthly</changefreq>") {
		t.Errorf("about keeps the default:\n%s", about)
	}
}
