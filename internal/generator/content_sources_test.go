package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// CONTENT-002: extra Markdown roots merged into the site. The default (no
// content_sources) must behave exactly as before — covered by every other test
// in this package, which never sets the field.

// writeMd creates a Markdown file with optional frontmatter.
func writeMd(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// sourceGen returns a generator with the given extra sources and empty site data.
func sourceGen(t *testing.T, sources ...ContentSource) *Generator {
	t.Helper()
	g, err := New(Config{
		Domain:         "example.com",
		TemplatesDir:   t.TempDir(),
		ContentSources: sources,
		Quiet:          true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func TestContentSourcesLoadPagesAndPosts(t *testing.T) {
	dir := t.TempDir()
	writeMd(t, filepath.Join(dir, "guides", "install.md"), "---\ntitle: Install\nstatus: publish\n---\n\nHow to install.\n")
	writeMd(t, filepath.Join(dir, "guides", "nested", "deep.md"), "---\ntitle: Deep\nstatus: publish\n---\n\nNested.\n")
	writeMd(t, filepath.Join(dir, "news", "release.md"), "---\ntitle: Release\nstatus: publish\n---\n\nShipped.\n")

	g := sourceGen(t,
		ContentSource{Path: filepath.Join(dir, "guides"), Category: "Guides"},
		ContentSource{Path: filepath.Join(dir, "news"), Type: "post"},
	)
	if err := g.loadExtraContentSources(); err != nil {
		t.Fatalf("loadExtraContentSources: %v", err)
	}

	if len(g.siteData.Pages) != 2 {
		t.Errorf("pages = %d, want 2 (loading is recursive)", len(g.siteData.Pages))
	}
	if len(g.siteData.Posts) != 1 {
		t.Errorf("posts = %d, want 1", len(g.siteData.Posts))
	}

	assertCategoryApplied(t, g, "Guides", "guides")
}

// assertCategoryApplied checks that a source category was registered under the
// expected slug and applied to every page the source contributed.
func assertCategoryApplied(t *testing.T, g *Generator, name, slug string) {
	t.Helper()
	id := 0
	for catID, cat := range g.siteData.Categories {
		if cat.Name == name && cat.Slug == slug {
			id = catID
			break
		}
	}
	if id == 0 {
		t.Fatalf("category %q was not registered: %+v", name, g.siteData.Categories)
	}
	for _, p := range g.siteData.Pages {
		if len(p.Categories) != 1 || p.Categories[0] != id {
			t.Errorf("page %q categories = %v, want [%d]", p.Slug, p.Categories, id)
		}
	}
}

// TestContentSourcesFrontmatterCategoryWins: a per-file category is more
// specific than the per-source default and must not be overwritten.
func TestContentSourcesFrontmatterCategoryWins(t *testing.T) {
	dir := t.TempDir()
	writeMd(t, filepath.Join(dir, "a.md"), "---\ntitle: A\nstatus: publish\ncategories: [\"Own\"]\n---\n\nBody.\n")
	writeMd(t, filepath.Join(dir, "b.md"), "---\ntitle: B\nstatus: publish\n---\n\nBody.\n")

	g := sourceGen(t, ContentSource{Path: dir, Category: "Source"})
	if err := g.loadExtraContentSources(); err != nil {
		t.Fatalf("loadExtraContentSources: %v", err)
	}

	for _, p := range g.siteData.Pages {
		switch p.Slug {
		case "a":
			if len(p.CategoriesRaw) == 0 && len(p.Categories) == 0 {
				t.Error("page a lost its own category")
			}
		case "b":
			if len(p.Categories) != 1 {
				t.Errorf("page b categories = %v, want the source default", p.Categories)
			}
		}
	}
}

// TestContentSourcesReuseExistingCategory: a category already declared in
// metadata.json must be reused, not duplicated under a new ID.
func TestContentSourcesReuseExistingCategory(t *testing.T) {
	dir := t.TempDir()
	writeMd(t, filepath.Join(dir, "a.md"), "---\ntitle: A\nstatus: publish\n---\n\nBody.\n")

	g := sourceGen(t, ContentSource{Path: dir, Category: "Docs"})
	g.siteData.Categories[7] = models.Category{ID: 7, Name: "Docs", Slug: "docs"}
	if err := g.loadExtraContentSources(); err != nil {
		t.Fatalf("loadExtraContentSources: %v", err)
	}
	if len(g.siteData.Categories) != 1 {
		t.Errorf("categories = %+v, want the existing one reused", g.siteData.Categories)
	}
	if got := g.siteData.Pages[0].Categories; len(got) != 1 || got[0] != 7 {
		t.Errorf("page categories = %v, want [7]", got)
	}
}

func TestContentSourcesErrors(t *testing.T) {
	tests := []struct {
		name   string
		source ContentSource
		want   string
	}{
		{"empty path", ContentSource{}, "path is required"},
		{"missing dir", ContentSource{Path: "no/such/dir"}, "no/such/dir"},
		{"bad type", ContentSource{Path: ".", Type: "chapter"}, "unsupported type"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := sourceGen(t, tc.source)
			err := g.loadExtraContentSources()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("loadExtraContentSources = %v, want an error containing %q", err, tc.want)
			}
		})
	}

	// A file where a directory is expected.
	file := filepath.Join(t.TempDir(), "notes.md")
	writeMd(t, file, "# hi\n")
	g := sourceGen(t, ContentSource{Path: file})
	if err := g.loadExtraContentSources(); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("file as source = %v, want a not-a-directory error", err)
	}

	// An empty directory warns but does not fail the build.
	g = sourceGen(t, ContentSource{Path: t.TempDir()})
	if err := g.loadExtraContentSources(); err != nil {
		t.Errorf("empty source = %v, want nil", err)
	}
}

// TestNoContentSourcesIsANoOp pins the backward-compatible default.
func TestNoContentSourcesIsANoOp(t *testing.T) {
	g := sourceGen(t)
	if err := g.loadExtraContentSources(); err != nil {
		t.Fatalf("loadExtraContentSources: %v", err)
	}
	if len(g.siteData.Pages) != 0 || len(g.siteData.Posts) != 0 || len(g.siteData.Categories) != 0 {
		t.Error("an empty content_sources list must not touch the site")
	}
}
