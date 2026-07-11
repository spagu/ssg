package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

func TestSortPostsByDate(t *testing.T) {
	in := []models.Page{
		{Slug: "a", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Slug: "b", Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{Slug: "c", Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}
	out := sortPostsByDate(in)
	if out[0].Slug != "b" || out[1].Slug != "c" || out[2].Slug != "a" {
		t.Errorf("newest-first order wrong: %v", []string{out[0].Slug, out[1].Slug, out[2].Slug})
	}
	if in[0].Slug != "a" {
		t.Errorf("input slice was mutated")
	}
}

func TestGenerateTagsAndAuthors(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<html><body>{{.Kind}}:{{.Name}}={{len .Posts}}</body></html>{{end}}`)
	g.siteData.Authors = map[int]models.Author{5: {ID: 5, Name: "Jan", Slug: "jan"}}
	g.siteData.Posts = []models.Page{
		{Title: "A", Slug: "a", Type: "post", Date: time.Now(), Tags: []string{"go", "web"}, Author: 5},
		{Title: "B", Slug: "b", Type: "post", Date: time.Now(), Tags: []string{"go"}, Author: 5},
	}
	tags, err := g.generateTags()
	if err != nil {
		t.Fatalf("generateTags: %v", err)
	}
	if tags["go"] != "go" || tags["web"] != "web" {
		t.Errorf("tag slugs = %v", tags)
	}
	if data, _ := os.ReadFile(filepath.Join(g.config.OutputDir, "tag", "go", "index.html")); !strings.Contains(string(data), "tag:go=2") {
		t.Errorf("tag/go page wrong: %s", data)
	}
	authors, err := g.generateAuthors()
	if err != nil {
		t.Fatalf("generateAuthors: %v", err)
	}
	if authors["jan"] != "jan" {
		t.Errorf("author slugs = %v", authors)
	}
	if data, _ := os.ReadFile(filepath.Join(g.config.OutputDir, "author", "jan", "index.html")); !strings.Contains(string(data), "author:Jan=2") {
		t.Errorf("author/jan page wrong: %s", data)
	}
}

func TestGenerateFeeds(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Feed = true
	g.config.FeedItems = 20
	g.siteData.Categories[2] = models.Category{ID: 2, Name: "News", Slug: "news"}
	g.siteData.Posts = []models.Page{
		{Title: "Post A", Slug: "a", Type: "post", Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			Categories: []int{2}, Tags: []string{"go"}, Excerpt: "excerpt A"},
	}
	g.tagSlugs = map[string]string{"go": "go"}
	if err := g.generateFeeds(); err != nil {
		t.Fatalf("generateFeeds: %v", err)
	}
	root, _ := os.ReadFile(filepath.Join(g.config.OutputDir, "feed.xml"))
	if !strings.Contains(string(root), `<feed xmlns="http://www.w3.org/2005/Atom">`) {
		t.Errorf("root feed not Atom: %s", root)
	}
	if !strings.Contains(string(root), "<entry>") || !strings.Contains(string(root), "Post A") {
		t.Errorf("root feed missing entry: %s", root)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "category", "news", "feed.xml")); err != nil {
		t.Errorf("category feed missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "tag", "go", "feed.xml")); err != nil {
		t.Errorf("tag feed missing: %v", err)
	}
}

func TestBuildOpenGraphAndInjectSEO(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com"
	g.config.Feed = true
	og := g.buildOpenGraph(models.Page{Title: "T", Description: "D", Slug: "s", Type: "post"}, true)
	if !strings.Contains(og, `property="og:title"`) || !strings.Contains(og, `"@type":"Article"`) {
		t.Errorf("open graph incomplete: %s", og)
	}

	out := g.config.OutputDir
	page := models.Page{Title: "T", Description: "D", Slug: "s", Type: "post"}
	path := filepath.Join(out, "index.html")
	_ = os.WriteFile(path, []byte("<html><head></head><body></body></html>"), 0644)
	g.config.SEO = true // opt-in (v1.8.2): injection only runs when enabled
	g.injectSEO(path, page, true)
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "og:title") || !strings.Contains(string(data), "application/atom+xml") {
		t.Errorf("injectSEO missing tags: %s", data)
	}

	// Opt-out (default): injection is a no-op unless SEO is enabled.
	g.config.SEO = false
	path2 := filepath.Join(out, "b.html")
	_ = os.WriteFile(path2, []byte("<html><head></head></html>"), 0644)
	g.injectSEO(path2, page, true)
	d2, _ := os.ReadFile(path2)
	if strings.Contains(string(d2), "og:title") {
		t.Errorf("SEO off (default) should disable injection")
	}
}

func TestCheckLinks(t *testing.T) {
	g := newTestGen(t, "")
	out := g.config.OutputDir
	_ = os.MkdirAll(filepath.Join(out, "good"), 0755)
	_ = os.WriteFile(filepath.Join(out, "good", "index.html"), []byte("<html></html>"), 0644)
	_ = os.WriteFile(filepath.Join(out, "index.html"),
		[]byte(`<a href="/good/">ok</a><a href="/missing/">bad</a><a href="https://x.com">ext</a><a href="#frag">f</a>`), 0644)

	broken, err := g.checkLinks()
	if err != nil {
		t.Fatalf("checkLinks: %v", err)
	}
	if len(broken) != 1 || broken[0].href != "/missing/" {
		t.Errorf("expected 1 broken (/missing/), got %v", broken)
	}
}

func TestIsInternalRef(t *testing.T) {
	internal := []string{"/x/", "foo.html", "img/a.png"}
	external := []string{"", "#top", "//cdn", "https://x", "http://x", "mailto:a@b", "data:xxx", "javascript:1"}
	for _, r := range internal {
		if !isInternalRef(r) {
			t.Errorf("isInternalRef(%q) = false, want true", r)
		}
	}
	for _, r := range external {
		if isInternalRef(r) {
			t.Errorf("isInternalRef(%q) = true, want false", r)
		}
	}
}

func TestBundleAndOutputsAndSearch(t *testing.T) {
	g := newTestGen(t, "")
	out := g.config.OutputDir
	_ = os.WriteFile(filepath.Join(out, "a.css"), []byte("a{}"), 0644)
	_ = os.WriteFile(filepath.Join(out, "b.css"), []byte("b{}"), 0644)
	g.config.Bundles = map[string][]string{"app.css": {"a.css", "b.css"}}
	if err := g.bundleIfRequested(); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	bundle, _ := os.ReadFile(filepath.Join(out, "app.css"))
	if !strings.Contains(string(bundle), "a{}") || !strings.Contains(string(bundle), "b{}") {
		t.Errorf("bundle not concatenated: %s", bundle)
	}

	// JSON output
	g.config.Outputs = []string{"html", "json"}
	page := models.Page{Title: "T", Slug: "s", Type: "post", Content: "hello world", Excerpt: "ex"}
	htmlPath := filepath.Join(out, "s", "index.html")
	_ = os.MkdirAll(filepath.Dir(htmlPath), 0755)
	_ = os.WriteFile(htmlPath, []byte("<html></html>"), 0644)
	g.writeJSONOutput(page, htmlPath)
	if _, err := os.Stat(filepath.Join(out, "s", "index.json")); err != nil {
		t.Errorf("index.json not written: %v", err)
	}

	// Search index
	g.config.SearchIndex = true
	g.siteData.Posts = []models.Page{page}
	if err := g.generateSearchIndex(); err != nil {
		t.Fatalf("searchIndex: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "search-index.json")); err != nil {
		t.Errorf("search-index.json not written: %v", err)
	}
}

func TestWantsOutput(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Outputs = []string{"html", "JSON"}
	if !g.wantsOutput("json") {
		t.Error("wantsOutput should be case-insensitive")
	}
	if g.wantsOutput("xml") {
		t.Error("xml not configured")
	}
}
