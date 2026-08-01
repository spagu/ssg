package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/taxonomy"
)

// TestRouteManifest covers #62: with route_manifest on, routes.json lists every
// post, page and taxonomy archive with metadata, deduped and sorted by path.
func TestRouteManifest(t *testing.T) {
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"exported_at":"","media":[],`+
			`"users":[{"id":1,"name":"Ed","slug":"ed"}]}`)
	mustWrite(t, filepath.Join(contentDir, "posts", "news", "one.md"),
		"---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\n"+
			"categories: [News]\ntags: [Go]\nauthor: 1\ntopic: [Alpha]\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n")
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))

	gen, err := New(Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:    filepath.Join(tmp, "content"),
		TemplatesDir:  filepath.Join(tmp, "templates"),
		OutputDir:     filepath.Join(tmp, "output"),
		RouteManifest: true, Quiet: true,
		Taxonomies: map[string]taxonomy.DefinitionConfig{"topic": {}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "output", "routes.json"))
	if err != nil {
		t.Fatalf("routes.json missing: %v", err)
	}
	var doc RouteManifest
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid routes.json: %v", err)
	}
	byPath := map[string]RouteEntry{}
	for _, r := range doc.Routes {
		byPath[r.Path] = r
	}
	if doc.Count != len(doc.Routes) || doc.Count == 0 {
		t.Fatalf("count %d vs %d routes", doc.Count, len(doc.Routes))
	}
	want := map[string]string{
		"/2024/01/02/one/": "post",
		"/about/":          "page",
		"/tag/go/":         "tag",
		"/category/news/":  "category",
		"/author/ed/":      "author",
		"/topic/":          "topic-index", // custom taxonomy renders an index page
		"/topic/alpha/":    "topic",
	}
	for path, typ := range want {
		r, ok := byPath[path]
		if !ok {
			t.Errorf("route %s (%s) missing from manifest; have %v", path, typ, doc.Routes)
			continue
		}
		if r.Type != typ {
			t.Errorf("route %s type = %q, want %q", path, r.Type, typ)
		}
	}
	if byPath["/2024/01/02/one/"].Source == "" {
		t.Errorf("post route must carry its source file")
	}
	// Sorted by path.
	for i := 1; i < len(doc.Routes); i++ {
		if doc.Routes[i-1].Path > doc.Routes[i].Path {
			t.Errorf("routes not sorted at %d: %s > %s", i, doc.Routes[i-1].Path, doc.Routes[i].Path)
		}
	}
}
