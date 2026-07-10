package generator

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

// --- AX-002: tocHTML / nodeText -------------------------------------------

func TestTocHTMLAndNodeText(t *testing.T) {
	g := &Generator{config: Config{TOCDepth: 3}}
	g.md = buildMarkdown(g.config)
	toc := string(g.tocHTML("## First Section\n\ntext\n\n### Sub\n\n## Second"))
	if !strings.Contains(toc, `class="toc"`) {
		t.Fatalf("no toc list: %s", toc)
	}
	for _, want := range []string{`href="#first-section"`, `>First Section<`, `href="#sub"`, `href="#second"`} {
		if !strings.Contains(toc, want) {
			t.Errorf("toc missing %q in %s", want, toc)
		}
	}
	// depth limit excludes deeper headings
	g.config.TOCDepth = 2
	g.md = buildMarkdown(g.config)
	toc2 := string(g.tocHTML("## A\n\n### TooDeep"))
	if strings.Contains(toc2, "toodeep") {
		t.Errorf("depth 2 should exclude H3: %s", toc2)
	}
	// no headings → empty
	if g.tocHTML("just text") != "" {
		t.Errorf("expected empty toc for no headings")
	}
}

// --- GO-007: alt-engine template loading + rendering ----------------------

func TestEnginePipeline(t *testing.T) {
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "templates", "m")
	_ = os.MkdirAll(themeDir, 0755)
	for _, name := range []string{"index", "post", "page", "category"} {
		_ = os.WriteFile(filepath.Join(themeDir, name+".html"),
			[]byte("<html><body>"+name+" {{Title}} {{{Content}}}</body></html>"), 0644)
	}
	g := &Generator{
		config: Config{Engine: "mustache", Template: "m", TemplatesDir: filepath.Join(dir, "templates"),
			OutputDir: filepath.Join(dir, "out")},
		siteData: &models.SiteData{Categories: map[int]models.Category{}, Media: map[int]models.MediaItem{}},
	}
	g.md = buildMarkdown(g.config)
	if err := g.loadTemplates(); err != nil {
		t.Fatalf("loadTemplates(mustache): %v", err)
	}
	if g.engine == nil || len(g.engineTmpls) != 4 {
		t.Fatalf("engine templates not loaded: %v", g.engineTmpls)
	}
	_ = os.MkdirAll(g.config.OutputDir, 0755)
	out := filepath.Join(g.config.OutputDir, "index.html")
	// Content is raw markdown template.HTML → prepAltData pre-renders it.
	data := map[string]interface{}{"Title": "Hello", "Content": template.HTML("# Heading")}
	if err := g.renderTemplate("post.html", out, data); err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	got, _ := os.ReadFile(out)
	if !strings.Contains(string(got), "post Hello") {
		t.Errorf("engine render missing title: %s", got)
	}
	if !strings.Contains(string(got), "Heading</h1>") {
		t.Errorf("prepAltData did not pre-render markdown Content: %s", got)
	}
	// Missing template → error (fallback signal).
	if err := g.renderTemplate("nope.html", out, data); err == nil {
		t.Errorf("expected error for missing engine template")
	}
}

// --- old code: loadMetadataFromMddb via a fake client ---------------------

type fakeMddb struct{ byCollection map[string][]mddb.Document }

func (f *fakeMddb) Get(mddb.GetRequest) (*mddb.Document, error) { return nil, nil }
func (f *fakeMddb) Search(mddb.SearchRequest) ([]mddb.Document, int, error) {
	return nil, 0, nil
}
func (f *fakeMddb) GetAll(collection, _ string, _ int) ([]mddb.Document, error) {
	return f.byCollection[collection], nil
}
func (f *fakeMddb) GetByType(_, _, _ string) ([]mddb.Document, error) { return nil, nil }
func (f *fakeMddb) Health() error                                     { return nil }
func (f *fakeMddb) Checksum(string) (*mddb.ChecksumResponse, error) {
	return &mddb.ChecksumResponse{}, nil
}
func (f *fakeMddb) Close() error { return nil }

func TestLoadMetadataFromMddb(t *testing.T) {
	g := &Generator{
		config:   Config{},
		siteData: &models.SiteData{Categories: map[int]models.Category{}, Media: map[int]models.MediaItem{}, Authors: map[int]models.Author{}},
	}
	client := &fakeMddb{byCollection: map[string][]mddb.Document{
		"categories": {{Key: "tech", Metadata: map[string]any{"id": float64(3), "name": "Tech", "slug": "tech"}}},
		"media":      {{Key: "m1", Metadata: map[string]any{"id": float64(9), "media_type": "image"}}},
		"users":      {{Key: "u1", Metadata: map[string]any{"id": float64(5), "name": "Jan"}}},
	}}
	if err := g.loadMetadataFromMddb(client); err != nil {
		t.Fatalf("loadMetadataFromMddb: %v", err)
	}
	if g.siteData.Categories[3].Name != "Tech" {
		t.Errorf("category not loaded: %v", g.siteData.Categories)
	}
	if g.siteData.Media[9].MediaType != "image" {
		t.Errorf("media not loaded: %v", g.siteData.Media)
	}
	if g.siteData.Authors[5].Name != "Jan" {
		t.Errorf("author not loaded: %v", g.siteData.Authors)
	}
}
