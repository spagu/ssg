package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestNewSanitizer(t *testing.T) {
	if newSanitizer(false) != nil {
		t.Error("disabled sanitizer should be nil")
	}
	p := newSanitizer(true)
	if p == nil {
		t.Fatal("enabled sanitizer should be non-nil")
	}
	out := p.Sanitize(`<p>ok</p><script>alert(1)</script>`)
	if strings.Contains(out, "<script>") {
		t.Errorf("script not stripped: %s", out)
	}
	if !strings.Contains(out, "<p>ok</p>") {
		t.Errorf("safe content removed: %s", out)
	}
}

func TestSanitizeInPipeline(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SanitizeHTML = true
	g.sanitizer = newSanitizer(true)
	g.md = buildMarkdown(g.config)
	fn := g.tmplSafeHTML(map[string]string{}, map[string]map[string]string{})
	out := string(fn("hello <script>alert(1)</script> world"))
	if strings.Contains(out, "<script>") {
		t.Errorf("sanitizer not applied in pipeline: %s", out)
	}
}

func TestAuthorNameSlug(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Authors = map[int]models.Author{
		7: {ID: 7, Name: "Jan Kowalski", Slug: "jan-k"},
		8: {ID: 8, Name: "No Slug"},
	}
	if name, slug := g.authorNameSlug(7); name != "Jan Kowalski" || slug != "jan-k" {
		t.Errorf("author 7 = %q/%q", name, slug)
	}
	if _, slug := g.authorNameSlug(8); slug != "no-slug" {
		t.Errorf("author 8 slug from name = %q, want no-slug", slug)
	}
	if name, slug := g.authorNameSlug(99); name != "author-99" || slug != "author-99" {
		t.Errorf("unknown author = %q/%q", name, slug)
	}
}

func TestTocContext(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	if g.tocContext("## A") != "" {
		t.Error("tocContext should be empty when toc disabled")
	}
	g.config.TOC = true
	g.config.TOCDepth = 3
	g.md = buildMarkdown(g.config)
	if g.tocContext("## A") == "" {
		t.Error("tocContext should be populated when toc enabled")
	}
}

func TestSortedValues(t *testing.T) {
	got := sortedValues(map[string]string{"a": "z", "b": "a", "c": "a", "d": ""})
	// deduped ("a" once), sorted, empty dropped → [a z]
	if len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Errorf("sortedValues = %v, want [a z]", got)
	}
}

func TestLoadContentFromMddbUnavailable(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Mddb.Enabled = true
	g.config.Mddb.URL = "http://127.0.0.1:1" // unreachable
	g.config.Mddb.Protocol = "http"
	g.config.Mddb.Timeout = 1
	g.config.Mddb.Collection = "content"
	if err := g.loadContentFromMddb(); err == nil {
		t.Error("expected error when mddb server is unreachable")
	}
}

func TestLoadContentFromMddbGRPCBadAddr(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Mddb.Enabled = true
	g.config.Mddb.URL = "127.0.0.1:1"
	g.config.Mddb.Protocol = "grpc"
	g.config.Mddb.Timeout = 1
	g.config.Mddb.Collection = "content"
	// gRPC client creation may succeed lazily; Health() must then fail.
	if err := g.loadContentFromMddb(); err == nil {
		t.Error("expected error for unreachable gRPC mddb server")
	}
}

func TestRefResolves(t *testing.T) {
	dir := t.TempDir()
	g := newTestGen(t, "")
	g.config.OutputDir = dir
	// /page/ dir with index.html
	if err := os.MkdirAll(filepath.Join(dir, "page"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "page", "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		ref, htmlDir string
		want         bool
	}{
		{"#frag", dir, true},           // pure fragment
		{"", dir, true},                // empty after strip
		{"/file.css", dir, true},       // absolute to file
		{"/page/", dir, true},          // absolute dir → index.html
		{"/page", dir, true},           // dir without slash but has index.html
		{"/empty/", dir, false},        // dir without index.html
		{"/missing.html", dir, false},  // missing
		{"file.css?v=1", dir, true},    // query stripped, relative
		{"../nope/x.html", dir, false}, // relative missing
	}
	for _, c := range cases {
		if got := g.refResolves(c.ref, c.htmlDir); got != c.want {
			t.Errorf("refResolves(%q) = %v, want %v", c.ref, got, c.want)
		}
	}
}

func TestWriteBundleMissingSource(t *testing.T) {
	dir := t.TempDir()
	g := newTestGen(t, "")
	g.config.OutputDir = dir
	// One present source, one missing → exercises the warn-and-continue branch.
	if err := os.WriteFile(filepath.Join(dir, "a.css"), []byte("a{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.writeBundle("bundle.css", []string{"a.css", "missing.css"}); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "bundle.css"))
	if err != nil {
		t.Fatalf("bundle not written: %v", err)
	}
	if !strings.Contains(string(out), "a{}") {
		t.Errorf("bundle missing present source: %s", out)
	}
}

func TestAtImportCount(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.css")
	if err := os.WriteFile(p, []byte("@import 'a';\n@import 'b';\nbody{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := atImportCount(p); n != 2 {
		t.Errorf("atImportCount = %d, want 2", n)
	}
	if n := atImportCount(filepath.Join(dir, "nope.css")); n != 0 {
		t.Errorf("missing file count = %d, want 0", n)
	}
}
