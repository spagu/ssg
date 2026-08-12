package generator

// generator.go gaps, part 1: i18n content validation policies, output
// collision detection, and the archive/alias/category writers' guard and
// failure branches.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// TestValidateI18nContentPolicies: invalid_language and duplicate_translation
// each support warn (report, keep building) and fail (stop) — a migration
// needs the warn mode to see every problem in one run.
func TestValidateI18nContentPolicies(t *testing.T) {
	langs := []ssgi18n.LanguageConfig{{Code: "en"}}
	newGen := func(invalid, dup string) *Generator {
		g := newTestGen(t, "")
		g.config.I18n.Enabled = true
		g.config.I18n.InvalidLanguage = invalid
		g.config.I18n.DuplicateTranslation = dup
		return g
	}

	g := newGen("warn", "fail")
	g.siteData.Pages = []models.Page{{Slug: "a", Lang: "xx", SourceFile: "a.md"}}
	if err := g.validateI18nContent(langs); err != nil {
		t.Errorf("invalid_language: warn must not fail the build: %v", err)
	}

	g = newGen("fail", "fail")
	g.siteData.Pages = []models.Page{{Slug: "a", Lang: "xx", SourceFile: "a.md"}}
	if err := g.validateI18nContent(langs); err == nil || !strings.Contains(err.Error(), "unconfigured language") {
		t.Errorf("invalid_language: fail must stop the build, got %v", err)
	}

	g = newGen("fail", "warn")
	g.siteData.Pages = []models.Page{
		{Slug: "a", Lang: "en", TranslationKey: "k", SourceFile: "a.md"},
		{Slug: "b", Lang: "en", TranslationKey: "k", SourceFile: "b.md"},
	}
	if err := g.validateI18nContent(langs); err != nil {
		t.Errorf("duplicate_translation: warn must not fail the build: %v", err)
	}
}

// TestDetectContentCollisions: two pages emitting the same output path — or
// the same alias — under i18n is a hard error naming both source files; the
// language prefix scopes aliases per language.
func TestDetectContentCollisions(t *testing.T) {
	g := newTestGen(t, "")

	g.siteData.Pages = []models.Page{
		{Slug: "same", Type: "page", SourceFile: "a.md"},
		{Slug: "same", Type: "page", SourceFile: "b.md"},
	}
	if err := g.detectContentCollisions(); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Errorf("duplicate output path must error, got %v", err)
	}

	// Distinct pages sharing an alias in one language: alias collision.
	g.siteData.Pages = []models.Page{
		{Slug: "a", Type: "page", SourceFile: "a.md", LangPrefix: "en", Aliases: []string{"old"}},
		{Slug: "b", Type: "page", SourceFile: "b.md", LangPrefix: "en", Aliases: []string{"old"}},
	}
	err := g.detectContentCollisions()
	if err == nil || !strings.Contains(err.Error(), `alias "en/old"`) {
		t.Errorf("duplicate language-prefixed alias must error, got %v", err)
	}

	// The same alias under different language prefixes is fine.
	g.siteData.Pages = []models.Page{
		{Slug: "a", Type: "page", SourceFile: "a.md", LangPrefix: "en", Aliases: []string{"old"}},
		{Slug: "b", Type: "page", SourceFile: "b.md", LangPrefix: "pl", Aliases: []string{"old"}},
	}
	if err := g.detectContentCollisions(); err != nil {
		t.Errorf("language prefixes must scope aliases apart: %v", err)
	}
}

// TestRenderArchiveGuards: the skip branches — empty slug, a URL already owned
// by real content, and an output path outside the root — end the archive
// quietly; they must never write anything.
func TestRenderArchiveGuards(t *testing.T) {
	g := newTestGen(t, "")
	if err := g.renderArchive("tag", "X", "", nil, tagHTMLName, false); err != nil {
		t.Errorf("empty slug must be a silent skip: %v", err)
	}
	g.ownedURLs = map[string]string{"/tag/go/": "page tech"}
	if err := g.renderArchive("tag", "Go", "go", nil, tagHTMLName, false); err != nil {
		t.Errorf("owned URL must be a silent skip: %v", err)
	}
	if entries, _ := os.ReadDir(g.config.OutputDir); len(entries) != 0 {
		t.Errorf("guards must write nothing, got %v", entries)
	}
	// No usable output root: every archive path is out of bounds → skipped.
	g2 := newTestGen(t, "")
	g2.config.OutputDir = ""
	if err := g2.renderArchive("tag", "Go", "go", nil, tagHTMLName, false); err != nil {
		t.Errorf("unsafe output path must be a silent skip: %v", err)
	}
}

// TestRenderArchiveFailures: a blocked directory is a build error; a template
// failing on both the primary and the category.html fallback is reported but
// does not abort the whole build (matching the legacy archive contract).
func TestRenderArchiveFailures(t *testing.T) {
	g := newTestGen(t, "")
	mustWrite(t, filepath.Join(g.config.OutputDir, "tag"), "in the way")
	if err := g.renderArchive("tag", "Go", "go", nil, tagHTMLName, false); err == nil {
		t.Error("a blocked archive directory must error")
	}

	g2 := newTestGen(t,
		`{{define "tag.html"}}{{.Nope}}{{end}}{{define "category.html"}}{{.Nope}}{{end}}`)
	if err := g2.renderArchive("tag", "Go", "go", nil, tagHTMLName, false); err != nil {
		t.Errorf("a double template failure warns, it must not abort: %v", err)
	}
}

// TestWriteAliasStubsLangPrefix: under i18n an alias redirect is scoped into
// the page's language tree, so /en/old/ redirects rather than /old/.
func TestWriteAliasStubsLangPrefix(t *testing.T) {
	g := newTestGen(t, "")
	g.config.I18n.Enabled = true
	page := models.Page{Slug: "new", Type: "page", LangPrefix: "en", Aliases: []string{"old"}}
	g.writeAliasStubs(page)
	if len(g.aliasRedirects) != 1 || g.aliasRedirects[0].From != "/en/old" {
		t.Fatalf("aliasRedirects = %+v, want one rule from /en/old", g.aliasRedirects)
	}
}

// TestWriteAliasStubFailures: unsafe paths, collisions with real pages and
// filesystem refusals all skip the stub with a warning — an alias is a
// convenience and must never corrupt or overwrite real output.
func TestWriteAliasStubFailures(t *testing.T) {
	g := newTestGen(t, "")
	out := g.config.OutputDir

	g.writeAliasStub("esc", "../outside", "/t/")
	if _, err := os.Stat(filepath.Join(filepath.Dir(out), "outside")); err == nil {
		t.Error("an escaping alias must write nothing")
	}

	mustWrite(t, filepath.Join(out, "exists", "index.html"), "real page")
	g.writeAliasStub("clash", "exists", "/t/")
	if got := mustRead(t, filepath.Join(out, "exists", "index.html")); got != "real page" {
		t.Error("an alias must never overwrite an existing page")
	}

	mustWrite(t, filepath.Join(out, "blocked"), "not a dir")
	g.writeAliasStub("mk", "blocked/sub", "/t/") // MkdirAll failure → warn, no panic

	mustMkdir(t, filepath.Join(out, "d", "index.html"))
	g.writeAliasStub("wr", "d", "/t/") // WriteFile failure → warn, no panic
}

// TestGenerateCategoriesGuards: an owned URL and an unsafe output root skip
// the category; a blocked directory is a hard error.
func TestGenerateCategoriesGuards(t *testing.T) {
	newCatGen := func() *Generator {
		g := newTestGen(t, "")
		g.siteData.Categories[2] = models.Category{ID: 2, Name: "News", Slug: "news"}
		g.siteData.Posts = []models.Page{{Slug: "p", Categories: []int{2}}}
		return g
	}

	g := newCatGen()
	g.ownedURLs = map[string]string{"/category/news/": "page news"}
	if err := g.generateCategories(); err != nil {
		t.Errorf("owned category URL must be a silent skip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "category")); err == nil {
		t.Error("skipped category must write nothing")
	}

	g = newCatGen()
	g.config.OutputDir = ""
	if err := g.generateCategories(); err != nil {
		t.Errorf("unsafe output path must be a silent skip: %v", err)
	}

	g = newCatGen()
	mustWrite(t, filepath.Join(g.config.OutputDir, "category"), "in the way")
	if err := g.generateCategories(); err == nil {
		t.Error("a blocked category directory must error")
	}
}
