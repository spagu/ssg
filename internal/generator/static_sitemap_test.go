package generator

// A verbatim HTML document never becomes a Page, so no branch of the sitemap
// could reach it however heavily the site linked it (#255).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// buildStaticSourceSite publishes an SPA the way schema-resume.org does: a
// single hand-authored file, declared verbatim.
func buildStaticSourceSite(t *testing.T, sources func(dir string) []models.StaticSource) (Config, string) {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`,
		map[string]string{
			"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nBody.\n",
		}, nil)

	app := filepath.Join(t.TempDir(), "editor")
	mustWrite(t, filepath.Join(app, indexHTMLName), `<html><head><title>Editor</title></head><body><p>app</p></body></html>`)
	mustWrite(t, filepath.Join(app, "app.js"), `console.log("x")`)
	cfg.StaticSources = sources(app)
	buildSiteFixture(t, cfg)
	return cfg, mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
}

// TestStaticSourceEntersTheSitemapWhenAsked: the site declares the file already,
// so it can say which of them is a document.
func TestStaticSourceEntersTheSitemapWhenAsked(t *testing.T) {
	cfg, sitemap := buildStaticSourceSite(t, func(dir string) []models.StaticSource {
		return []models.StaticSource{
			{Path: filepath.Join(dir, indexHTMLName), Dest: "editor/" + indexHTMLName, Sitemap: true, Priority: 0.8},
			{Path: filepath.Join(dir, "app.js"), Dest: "editor/app.js"},
		}
	})

	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "editor", indexHTMLName)); err != nil {
		t.Fatalf("fixture did not publish the app: %v", err)
	}
	if !strings.Contains(sitemap, "<loc>https://example.com/editor/</loc>") {
		t.Errorf("the copied document is missing from the sitemap:\n%s", sitemap)
	}
	if !strings.Contains(sitemap, "<priority>0.8</priority>") {
		t.Error("the declared priority is not honoured")
	}
	// The asset beside it is not a document and must not be listed.
	if strings.Contains(sitemap, "app.js") {
		t.Error("an asset reached the sitemap")
	}
}

// TestStaticSourceStaysOutByDefault: most of what a site copies verbatim is an
// asset, so listing is opt-in.
func TestStaticSourceStaysOutByDefault(t *testing.T) {
	_, sitemap := buildStaticSourceSite(t, func(dir string) []models.StaticSource {
		return []models.StaticSource{{Path: filepath.Join(dir, indexHTMLName), Dest: "editor/" + indexHTMLName}}
	})

	if strings.Contains(sitemap, "/editor/") {
		t.Errorf("a copied file was listed without being asked for:\n%s", sitemap)
	}
}

// TestStaticSourceDirectoryUsesItsIndex: a whole directory published verbatim
// is addressed by the document at its root.
func TestStaticSourceDirectoryUsesItsIndex(t *testing.T) {
	_, sitemap := buildStaticSourceSite(t, func(dir string) []models.StaticSource {
		return []models.StaticSource{{Path: dir, Dest: "editor", Sitemap: true}}
	})

	if !strings.Contains(sitemap, "<loc>https://example.com/editor/</loc>") {
		t.Errorf("a directory entry did not resolve to its index:\n%s", sitemap)
	}
	// An unset priority is the 0.8 an ordinary page gets.
	if !strings.Contains(sitemap, "<priority>0.8</priority>") {
		t.Error("the default priority is not the page default")
	}
}

// TestStaticSourceHonoursNoindex: a copied document that marks itself noindex
// keeps itself out, the rule every other entry follows.
func TestStaticSourceHonoursNoindex(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, nil, nil)
	app := filepath.Join(t.TempDir(), "editor")
	mustWrite(t, filepath.Join(app, indexHTMLName),
		`<html><head><meta name="robots" content="noindex"></head><body><p>app</p></body></html>`)
	cfg.StaticSources = []models.StaticSource{
		{Path: filepath.Join(app, indexHTMLName), Dest: "editor/" + indexHTMLName, Sitemap: true},
	}
	buildSiteFixture(t, cfg)

	if sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml")); strings.Contains(sitemap, "/editor/") {
		t.Errorf("a noindex document was advertised:\n%s", sitemap)
	}
}

// TestStaticSourceSitemapOnANonDocument: asking to list a stylesheet is a
// mistake, and saying so beats publishing a URL no crawler should index.
func TestStaticSourceSitemapOnANonDocument(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com"
	css := filepath.Join(g.config.OutputDir, "app.css")
	mustWrite(t, css, "body{}")

	out := captureBuildOutput(t, func() {
		g.recordStaticSitemapEntry(models.StaticSource{Path: "src/app.css", Sitemap: true}, css)
	})
	if !strings.Contains(out, "not an HTML document") {
		t.Errorf("no warning for a non-document:\n%s", out)
	}
	if len(g.staticSitemap) != 0 {
		t.Errorf("a stylesheet was recorded: %v", g.staticSitemap)
	}

	// And a path nothing was written to.
	out = captureBuildOutput(t, func() {
		g.recordStaticSitemapEntry(models.StaticSource{Path: "src/missing", Sitemap: true},
			filepath.Join(g.config.OutputDir, "missing"))
	})
	if !strings.Contains(out, "no HTML document was written") {
		t.Errorf("no warning for a missing document:\n%s", out)
	}
}

// TestStaticSitemapEdges covers the branches an ordinary build does not reach:
// a document another section already claimed, lastmod asked for outside a git
// checkout, and a build with nothing recorded.
func TestStaticSitemapEdges(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com"
	g.config.LastmodFromGit = true
	page := filepath.Join(g.config.OutputDir, "editor", indexHTMLName)
	mustWrite(t, page, `<html><head><title>E</title></head><body><p>x</p></body></html>`)
	g.recordStaticSitemapEntry(models.StaticSource{Path: page, Sitemap: true},
		filepath.Join(g.config.OutputDir, "editor"))

	// Claimed by an earlier section: listed once, not twice (#219).
	var sb strings.Builder
	claimed := map[string]bool{"https://example.com/editor/": true}
	g.writeSitemapStaticSources(&sb, claimed)
	if sb.String() != "" {
		t.Errorf("a claimed URL was listed again:\n%s", sb.String())
	}

	// Unclaimed: listed, and lastmod is simply absent outside a git checkout
	// rather than invented.
	sb.Reset()
	g.writeSitemapStaticSources(&sb, map[string]bool{})
	if !strings.Contains(sb.String(), "<loc>https://example.com/editor/</loc>") {
		t.Errorf("the entry is missing:\n%s", sb.String())
	}

	// Nothing recorded, nothing written.
	g.resetStaticSitemap()
	sb.Reset()
	g.writeSitemapStaticSources(&sb, map[string]bool{})
	if sb.String() != "" {
		t.Errorf("an empty record wrote %q", sb.String())
	}
	if _, ok := g.gitLastModForFile(""); ok {
		t.Error("an empty path has no commit date")
	}
}
