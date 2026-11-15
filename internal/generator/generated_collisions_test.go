package generator

// SR-12 (GO-183 security review): a page whose link: names a file ssg also
// writes — /robots.txt, /sitemap.xml, /search-index.json … — used to be
// replaced by the generated file without a word.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// claimSite builds a site whose one page has the given link:, and returns its
// output directory and the build's error.
func claimSite(t *testing.T, link string, apply func(cfg *Config)) (string, error) {
	t.Helper()
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{"categories":[],"exported_at":"","media":[],"users":[]}`)
	mustWrite(t, filepath.Join(content, "pages", "claim.md"), "---\ntitle: Claimed by a page\nslug: claim\nstatus: publish\ntype: page\nlink: "+link+"\n---\n\nbody\n")
	tmpl := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmpl)
	mustWrite(t, filepath.Join(tmpl, "page.html"), `{{define "page.html"}}PAGE {{.Title}}{{end}}`)
	cfg := Config{Source: "site", Template: "simple", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: filepath.Join(tmp, "output"),
		StaticDir: filepath.Join(tmp, "static"), Quiet: true}
	if apply != nil {
		apply(&cfg)
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var genErr error
	_, _ = capture(t, func() error { genErr = gen.Generate(); return nil })
	return cfg.OutputDir, genErr
}

// TestPageClaimingAGeneratedFileStopsTheBuild: before SR-12 the generated
// robots.txt, sitemap.xml and search index replaced the page silently (the
// page was written first, the generated file after it). Now the build stops
// and names both.
func TestPageClaimingAGeneratedFileStopsTheBuild(t *testing.T) {
	cases := []struct {
		link, generated string
		apply           func(cfg *Config)
	}{
		{"/robots.txt", "robots.txt", nil},
		{"/sitemap.xml", "sitemap.xml", nil},
		{"/search-index.json", "search-index.json", func(cfg *Config) { cfg.SearchIndex = true }},
	}
	for _, c := range cases {
		_, err := claimSite(t, c.link, c.apply)
		wantClash(t, err, "output collision at "+c.link, "claim.md", "the "+c.generated+" this build generates", "set a different link:")
	}
}

// TestPageWithAGeneratedNameStillBuilds: a site that writes its own robots.txt
// or sitemap as a page and turns the generated one off keeps building, and
// the page is what is served.
func TestPageWithAGeneratedNameStillBuilds(t *testing.T) {
	cases := []struct {
		link  string
		apply func(cfg *Config)
	}{
		{"/robots.txt", func(cfg *Config) { cfg.RobotsOff = true }},
		{"/sitemap.xml", func(cfg *Config) { cfg.SitemapOff = true }},
		{"/search-index.json", nil}, // no search index asked for
		{"/humans.txt", nil},        // nothing generated there
		{"/about.html", nil},        // HTML is not checked: the page owns it
	}
	for _, c := range cases {
		out, err := claimSite(t, c.link, c.apply)
		if err != nil {
			t.Fatalf("%s: %v", c.link, err)
		}
		if got := mustRead(t, filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(c.link, "/")))); !strings.Contains(got, "PAGE Claimed by a page") {
			t.Errorf("%s holds %q, not the page", c.link, got)
		}
	}
}

// TestPageClaimingAStaticFileNamesIt: a file from static/ copied over the
// page is named as such.
func TestPageClaimingAStaticFileNamesIt(t *testing.T) {
	_, err := claimSite(t, "/humans.txt", func(cfg *Config) {
		mustWrite(t, filepath.Join(cfg.StaticDir, "humans.txt"), "team")
	})
	wantClash(t, err, "output collision at /humans.txt", "claim.md", "static/humans.txt")
}

// TestClaimedFilesEdges: a page whose file was never written, and an output
// file that disappears before the check, are not collisions; a file that is
// replaced by one of a different size is.
func TestClaimedFilesEdges(t *testing.T) {
	g := publicGen(t)
	g.siteData = &models.SiteData{
		Pages: []models.Page{{Title: "Unwritten", Link: "/never.json"}, {Title: "Gone", Link: "/gone.xml"}, {Title: "Dir", Link: "/dir/"}},
		Posts: []models.Page{{Title: "Feed", Link: "/feed.xml", SourceFile: "posts/feed.md"}},
	}
	mustWrite(t, filepath.Join(g.config.OutputDir, "gone.xml"), "x")
	mustWrite(t, filepath.Join(g.config.OutputDir, "feed.xml"), "post")
	g.snapshotClaimedFiles()
	if len(g.claimedFiles) != 2 {
		t.Fatalf("claimed = %v", g.claimedFiles)
	}
	if err := os.Remove(filepath.Join(g.config.OutputDir, "gone.xml")); err != nil {
		t.Fatal(err)
	}
	if err := g.checkClaimedFiles(); err != nil {
		t.Fatalf("nothing replaced the post yet: %v", err)
	}
	mustWrite(t, filepath.Join(g.config.OutputDir, "feed.xml"), "<rss>generated</rss>")
	wantClash(t, g.checkClaimedFiles(), "output collision at /feed.xml", "posts/feed.md", "the feed.xml this build generates")
}
