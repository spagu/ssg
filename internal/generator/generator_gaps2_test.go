package generator

// generator.go gaps, part 2: static_sources failure modes (#84), built-in feed
// write failures (BLOG-002), JSON-LD derivation guards (#61/#110) and the env
// export / hook edge cases.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// TestCopyStaticSourcesGuards: blank entries are skipped; every filesystem
// refusal — unreadable source, blocked destination directory or file — is a
// build error, because a passthrough that half-copies published URLs breaks
// links that "already worked".
func TestCopyStaticSourcesGuards(t *testing.T) {
	// A blank path is skipped without touching the output.
	g := newTestGen(t, "")
	g.config.StaticSources = []models.StaticSource{{Path: "   "}}
	if err := g.copyStaticSources(); err != nil {
		t.Errorf("blank path must be skipped: %v", err)
	}

	// A stat failure that is NOT "does not exist" propagates.
	if os.Geteuid() != 0 {
		locked := filepath.Join(t.TempDir(), "locked")
		mustWrite(t, filepath.Join(locked, "f.txt"), "x")
		if err := os.Chmod(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		g = newTestGen(t, "")
		g.config.StaticSources = []models.StaticSource{{Path: filepath.Join(locked, "f.txt")}}
		if err := g.copyStaticSources(); err == nil {
			t.Error("an unreadable source must error, not vanish")
		}
	}

	// Directory source, output root replaced by a file: copyDir fails.
	srcDir := t.TempDir()
	mustWrite(t, filepath.Join(srcDir, "spec.xml"), "<spec/>")
	outFile := filepath.Join(t.TempDir(), "out")
	mustWrite(t, outFile, "not a dir")
	g = newTestGen(t, "")
	g.config.OutputDir = outFile
	g.config.StaticSources = []models.StaticSource{{Path: srcDir, Dest: "xml"}}
	if err := g.copyStaticSources(); err == nil {
		t.Error("a blocked destination for a directory source must error")
	}

	// File source, parent directory blocked: MkdirAll fails.
	srcFile := filepath.Join(t.TempDir(), "spec.xsd")
	mustWrite(t, srcFile, "<xs/>")
	g = newTestGen(t, "")
	mustWrite(t, filepath.Join(g.config.OutputDir, "sub"), "not a dir")
	g.config.StaticSources = []models.StaticSource{{Path: srcFile, Dest: "sub/spec.xsd"}}
	if err := g.copyStaticSources(); err == nil {
		t.Error("a blocked parent directory for a file source must error")
	}

	// File source, destination is an existing directory: copyFile fails.
	g = newTestGen(t, "")
	mustMkdir(t, filepath.Join(g.config.OutputDir, "spec.xsd"))
	g.config.StaticSources = []models.StaticSource{{Path: srcFile, Dest: "spec.xsd"}}
	if err := g.copyStaticSources(); err == nil {
		t.Error("a directory at the file destination must error")
	}
}

// TestGenerateFeedsWriteFailures: a blocked feed path fails the build from
// every branch — the root feed (i18n and not), category and tag feeds.
func TestGenerateFeedsWriteFailures(t *testing.T) {
	newFeedGen := func() *Generator {
		g := feedGen(t)
		g.config.Feed = true
		return g
	}
	cases := []struct {
		name  string
		setup func(g *Generator)
	}{
		{"root feed", func(g *Generator) {
			mustMkdir(t, filepath.Join(g.config.OutputDir, feedFileName))
		}},
		{"i18n root feed", func(g *Generator) {
			g.config.I18n.Enabled = true
			g.config.DefaultLanguage = "en"
			g.siteData.Languages = []ssgi18n.LanguageConfig{{Code: "en", Name: "English"}}
			mustMkdir(t, filepath.Join(g.config.OutputDir, feedFileName))
		}},
		{"category feed", func(g *Generator) {
			mustWrite(t, filepath.Join(g.config.OutputDir, "category"), "in the way")
		}},
		{"tag feed", func(g *Generator) {
			g.tagSlugs = map[string]string{"blog": "blog"}
			mustWrite(t, filepath.Join(g.config.OutputDir, "tag"), "in the way")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newFeedGen()
			c.setup(g)
			if err := g.generateFeeds(); err == nil {
				t.Fatal("a blocked feed path must fail the build")
			}
		})
	}
}

// TestGenerateFeedsSkipsEmptyCategorySlug: a category without a slug has no
// feed URL to write — skipped, while the rest of the feeds still emit.
func TestGenerateFeedsSkipsEmptyCategorySlug(t *testing.T) {
	g := feedGen(t)
	g.config.Feed = true
	g.siteData.Categories[3] = models.Category{ID: 3, Name: "NoSlug", Slug: ""}
	g.siteData.Posts[0].Categories = []int{3}
	if err := g.generateFeeds(); err != nil {
		t.Fatalf("generateFeeds: %v", err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, feedFileName)); err != nil {
		t.Error("root feed must still be written")
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "category")); err == nil {
		t.Error("a slug-less category must not emit a feed")
	}
}

// TestWriteFeedLimitsAndErrors: the item cap trims the oldest posts, and both
// path-safety and directory failures are errors.
func TestWriteFeedLimitsAndErrors(t *testing.T) {
	g := feedGen(t)
	if err := g.writeFeed("capped.xml", "T", "https://ex.com/", g.siteData.Posts, 1); err != nil {
		t.Fatal(err)
	}
	raw := mustRead(t, filepath.Join(g.config.OutputDir, "capped.xml"))
	if n := strings.Count(raw, "<entry>"); n != 1 {
		t.Errorf("limit 1 wrote %d entries", n)
	}
	if err := g.writeFeed("../evil.xml", "T", "https://ex.com/", nil, 5); err == nil {
		t.Error("a feed path escaping the output root must error")
	}
	mustWrite(t, filepath.Join(g.config.OutputDir, "sub"), "not a dir")
	if err := g.writeFeed("sub/feed.xml", "T", "https://ex.com/", nil, 5); err == nil {
		t.Error("a blocked feed directory must error")
	}
}

// TestJSONLDDerivationGuards: locale reaches inLanguage, a domain-less site
// emits no breadcrumb, and deepMergeLD tolerates a nil base.
func TestJSONLDDerivationGuards(t *testing.T) {
	g := newTestGen(t, "")
	ld := g.derivedLD(models.Page{Title: "T", Locale: "en-US"}, false, "https://ex.com/t/")
	if ld["inLanguage"] != "en-US" {
		t.Errorf("inLanguage = %v, want en-US", ld["inLanguage"])
	}
	g.config.Domain = ""
	if bc := g.breadcrumbLD(models.Page{Slug: "a", Type: "page"}); bc != nil {
		t.Error("a site without a domain cannot emit breadcrumb item URLs")
	}
	merged := deepMergeLD(nil, map[string]interface{}{"@type": "Article"})
	if merged["@type"] != "Article" {
		t.Errorf("nil base merge = %v", merged)
	}
}

// TestSectionSchemaPlacement: pages that cannot be placed get no section
// defaults; content outside the source root falls back to the content root.
func TestSectionSchemaPlacement(t *testing.T) {
	tmp := t.TempDir()
	g := newTestGen(t, "")
	g.config.ContentDir = filepath.Join(tmp, "content")
	g.config.Source = "site"
	g.config.SchemaDefaults = map[string]map[string]interface{}{
		"docs": {"@type": "TechArticle"},
	}
	// No source dir at all → no placement → nil.
	if got := g.sectionSchema(models.Page{Slug: "x", Link: "/x/"}); got != nil {
		t.Errorf("placeless page got section schema %v", got)
	}
	// content_sources content outside content/site still resolves via the
	// content root ("docs" key must match content/docs).
	p := models.Page{Slug: "d", Link: "/d/", SourceDir: filepath.Join(tmp, "content", "docs")}
	if got := g.sectionSchema(p); got == nil || got["@type"] != "TechArticle" {
		t.Errorf("content-root fallback schema = %v", got)
	}
	// Entirely outside the project: no section.
	if got := g.contentSection(models.Page{SourceDir: filepath.Join(t.TempDir(), "elsewhere")}); got != "" {
		t.Errorf("unplaceable dir mapped to section %q", got)
	}
}
