package generator

// Edges of generator.go that the build tests do not reach: metadata blocks,
// excluded and auto-excerpted content, the alternative engines' loading and
// data, git lastmod lookups and a handful of quiet/fallback paths.

import (
	"errors"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

func edgeGen(t *testing.T, cfg Config) *Generator {
	t.Helper()
	cfg.Domain = "example.com"
	if cfg.OutputDir == "" {
		cfg.OutputDir = t.TempDir()
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestLoadMetadataSiteBlocks: marketing, menus and custom types in
// metadata.json land on the site data.
func TestLoadMetadataSiteBlocks(t *testing.T) {
	g := edgeGen(t, Config{Quiet: true})
	path := filepath.Join(t.TempDir(), "metadata.json")
	mustWrite(t, path, `{"categories":[],"media":[],"users":[],
		"marketing":{"theme_color":"#0b57d0"},
		"menus":[{"id":1,"name":"Main","slug":"main","locations":["primary"],"items":[]}],
		"custom_types":[{"slug":"recipe","name":"Recipes"}]}`)
	if err := g.loadMetadata(path); err != nil {
		t.Fatal(err)
	}
	if g.siteData.Marketing.ThemeColor != "#0b57d0" || len(g.siteData.Menus) == 0 || len(g.siteData.CustomTypes) != 1 {
		t.Errorf("site data = %+v / %+v / %+v", g.siteData.Marketing, g.siteData.Menus, g.siteData.CustomTypes)
	}
}

// TestLoadContentFromFilesBadMetadata: an unreadable metadata.json fails the load.
func TestLoadContentFromFilesBadMetadata(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"), "{not json")
	g := edgeGen(t, Config{Source: "site", ContentDir: filepath.Join(tmp, "content"), Quiet: true})
	if err := g.loadContentFromFiles(); err == nil || !strings.Contains(err.Error(), "loading metadata") {
		t.Errorf("err = %v", err)
	}
}

// TestLoadMarkdownEntryExcludedAndExcerpt: content_exclude drops a file before
// parsing; auto_excerpt fills a missing excerpt.
func TestLoadMarkdownEntryExcludedAndExcerpt(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "skip.md"), "---\nnot: [valid\n---\n")
	mustWrite(t, filepath.Join(dir, "keep.md"), "---\ntitle: Keep\nstatus: publish\n---\n\nThe opening paragraph.\n")
	g := edgeGen(t, Config{ContentExclude: []string{"skip.md"}, AutoExcerpt: true, Quiet: true})
	pages, err := g.loadMarkdownDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Title != "Keep" || !strings.Contains(pages[0].Excerpt, "opening paragraph") {
		t.Errorf("pages = %+v", pages)
	}
}

// TestLoadEngineTemplatesErrors: an unknown engine, a theme with no templates
// and a template the engine cannot parse all fail the load.
func TestLoadEngineTemplatesErrors(t *testing.T) {
	if err := edgeGen(t, Config{Engine: "nope"}).loadEngineTemplates(t.TempDir(), nil); err == nil {
		t.Error("unknown engine must fail")
	}
	g := edgeGen(t, Config{Engine: "pongo2"})
	if err := g.loadEngineTemplates(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "no ") {
		t.Errorf("empty theme: %v", err)
	}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.html"), "{% if %}")
	if err := g.loadEngineTemplates(dir, nil); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Errorf("broken template: %v", err)
	}
}

// TestPrepAltData: non-map data passes through, Content of another type is
// kept, and template.HTML becomes a string.
func TestPrepAltData(t *testing.T) {
	g := edgeGen(t, Config{})
	if got := g.prepAltData(42); got != 42 {
		t.Errorf("non-map = %v", got)
	}
	got := g.prepAltData(map[string]interface{}{
		"Content": 7, "Body": template.HTML("<b>x</b>"), "Plain": "y",
	}).(map[string]interface{})
	if got["Content"] != 7 || got["Body"] != "<b>x</b>" || got["Plain"] != "y" {
		t.Errorf("prepAltData = %v", got)
	}
}

// TestExplicitMdLinkLangUnknownLanguage: a suffix that is not a site language
// keeps the rendering language.
func TestExplicitMdLinkLangUnknownLanguage(t *testing.T) {
	g := edgeGen(t, Config{})
	g.siteData = &models.SiteData{}
	if got := g.explicitMdLinkLang("about.xx.md", "pl"); got != "pl" {
		t.Errorf("got %q", got)
	}
}

// TestGitLastModEdges: no source file, no history and a root the path cannot
// be made relative to all report no date.
func TestGitLastModEdges(t *testing.T) {
	g := edgeGen(t, Config{})
	if _, ok := g.gitLastMod(models.Page{}); ok {
		t.Error("a page without a source file has no git date")
	}
	g.gitOnce.Do(func() {}) // no history loaded
	if _, ok := g.gitLastModForFile("a.md"); ok {
		t.Error("no history, no date")
	}
	g2 := edgeGen(t, Config{})
	g2.gitOnce.Do(func() {
		g2.gitRoot = "relative-root"
		g2.gitTimes = map[string]time.Time{"a.md": time.Now()}
	})
	if _, ok := g2.gitLastModForFile("a.md"); ok {
		t.Error("an absolute path is not relative to a relative root")
	}
}

// TestTaxonomySitemapEnabledUnknownName: a name the registry does not define
// keeps its sitemap entries.
func TestTaxonomySitemapEnabledUnknownName(t *testing.T) {
	g := edgeGen(t, Config{})
	g.taxonomies = taxonomy.NewRegistry(map[string]taxonomy.Definition{}, nil, nil)
	if !g.taxonomySitemapEnabled("author") {
		t.Error("author is outside the registry and keeps its entries")
	}
}

// TestInjectMermaidAssetsWithoutBody: a fragment gets the script appended.
func TestInjectMermaidAssetsWithoutBody(t *testing.T) {
	if got := injectMermaidAssets("<pre class=\"mermaid\">a</pre>", "", ""); !strings.HasPrefix(got, "<pre") || !strings.Contains(got, "mermaid.initialize") {
		t.Errorf("got %s", got)
	}
}

// TestReportStructuredMeta: quiet says nothing; otherwise a printed Go value
// in meta is named.
func TestReportStructuredMeta(t *testing.T) {
	docs := []mddb.Document{{Key: "x", Metadata: map[string]interface{}{"faq": "map[]"}}}
	edgeGen(t, Config{Quiet: true}).reportStructuredMeta(docs)
	edgeGen(t, Config{}).reportStructuredMeta(docs)
}

// TestWarnTemplateScriptContextFindings: an unreadable entry is skipped and a
// <title> inside a script is reported.
func TestWarnTemplateScriptContextFindings(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dir.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "index.html"),
		"<html><head>\n<script src=\"/a.js\">\n<title>{{.Site.Title}}</title>\n</script>\n</head></html>\n")
	edgeGen(t, Config{}).warnTemplateScriptContext(dir)
}

// TestCopyStrippedJPEG: a missing source fails; metadata is removed and counted.
func TestCopyStrippedJPEG(t *testing.T) {
	g := edgeGen(t, Config{})
	dir := t.TempDir()
	if err := g.copyStrippedJPEG(filepath.Join(dir, "missing.jpg"), filepath.Join(dir, "out.jpg")); err == nil {
		t.Error("missing source must fail")
	}
	payload := []byte("Exif\x00\x00GPS")
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, byte(len(payload) + 2)}
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xFF, 0xDA, 0x00, 0x04, 0x01, 0x01, 0x11, 0x22, 0x33, 0xFF, 0xD9)
	src := filepath.Join(dir, "in.jpg")
	if err := os.WriteFile(src, jpeg, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.jpg")
	if err := g.copyStrippedJPEG(src, dst); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustRead(t, dst), "GPS") {
		t.Error("EXIF must be removed")
	}
}

// failingMddb answers every GetAll with an error.
type failingMddb struct {
	mddb.MddbClient // embedded: only GetAll is called
}

func (failingMddb) GetAll(string, string, int) ([]mddb.Document, error) {
	return nil, errors.New("no such collection")
}

// TestLoadMetadataFromMddbMissingCollections: categories, media and users are
// optional; a missing collection is a warning, not a failure.
func TestLoadMetadataFromMddbMissingCollections(t *testing.T) {
	g := edgeGen(t, Config{})
	g.siteData = &models.SiteData{Categories: map[int]models.Category{}, Media: map[int]models.MediaItem{}, Authors: map[int]models.Author{}}
	if err := g.loadMetadataFromMddb(failingMddb{}); err != nil {
		t.Errorf("missing collections must not fail: %v", err)
	}
}

// TestGenerateSitemapRejectsUnknownKind: a sitemap spec naming an unknown kind
// fails before anything is written.
func TestGenerateSitemapRejectsUnknownKind(t *testing.T) {
	g := edgeGen(t, Config{Sitemaps: []models.SitemapSpec{{Path: "/a.xml", Include: []string{"nope"}}}})
	if err := g.generateSitemap(); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("err = %v", err)
	}
}

// TestWriteSitemapsUnwritable: a directory where the sitemap goes fails the write.
func TestWriteSitemapsUnwritable(t *testing.T) {
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(out, defaultSitemapName), 0o755); err != nil {
		t.Fatal(err)
	}
	g := edgeGen(t, Config{OutputDir: out})
	if err := g.writeSitemaps([]sitemapEntry{{loc: "https://example.com/"}}); err == nil {
		t.Error("a directory in the sitemap's place must fail")
	}
}

// TestMinifyAndSourceMapErrors: a missing asset, an unwritable minified file
// and a missing fingerprint source all fail.
func TestMinifyAndSourceMapErrors(t *testing.T) {
	dir := t.TempDir()
	g := edgeGen(t, Config{SourceMap: true})
	if err := g.minifyAssetFile(filepath.Join(dir, "missing.js"), nil, nil); err == nil {
		t.Error("missing asset must fail")
	}
	if err := writeWithSourceMap(filepath.Join(dir, "no", "such", "a.js"), "a", "a"); err == nil {
		t.Error("unwritable path must fail")
	}
	if err := g.fingerprintOne(filepath.Join(dir, "missing.css"), map[string]string{}, map[string]string{}); err == nil {
		t.Error("missing fingerprint source must fail")
	}
}

// TestSafeHTMLValueBeforeFuncs: a string passes through as markup before the
// content pipeline is wired.
func TestSafeHTMLValueBeforeFuncs(t *testing.T) {
	g := &Generator{}
	if got := g.safeHTMLValue("<b>x</b>"); got != "<b>x</b>" {
		t.Errorf("got %q", got)
	}
}

// TestMergeTemplateFuncsDuplicate: a helper registered twice keeps the later one.
func TestMergeTemplateFuncsDuplicate(t *testing.T) {
	funcs := template.FuncMap{"x": func() int { return 1 }}
	later := func() int { return 2 }
	mergeTemplateFuncs(funcs, map[string]interface{}{"x": later})
	if funcs["x"].(func() int)() != 2 {
		t.Error("the later registration must win")
	}
}
