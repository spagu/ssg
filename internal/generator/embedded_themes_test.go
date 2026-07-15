package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustScaffold extracts an embedded theme and fails the test unless it reports
// success.
func mustScaffold(t *testing.T, theme, dir string) {
	t.Helper()
	ok, err := scaffoldEmbeddedTheme(theme, dir)
	if err != nil || !ok {
		t.Fatalf("scaffoldEmbeddedTheme(%s) = %v, %v; want true, nil", theme, ok, err)
	}
}

// wantNonEmptyFiles asserts each relative path exists under dir with content.
func wantNonEmptyFiles(t *testing.T, dir string, rel ...string) {
	t.Helper()
	for _, f := range rel {
		if fi, err := os.Stat(filepath.Join(dir, f)); err != nil || fi.Size() == 0 {
			t.Errorf("expected extracted file %s, got err=%v", f, err)
		}
	}
}

// DOC-013: bundled themes are extracted from the binary, unknown themes fall
// back to the generic scaffold.
func TestScaffoldEmbeddedThemeSimple(t *testing.T) {
	dir := t.TempDir()
	mustScaffold(t, "simple", dir)
	wantNonEmptyFiles(t, dir, "index.html", "post.html", "page.html",
		"category.html", filepath.Join("css", "style.css"), filepath.Join("js", "main.js"))
}

func TestScaffoldEmbeddedThemeKrowy(t *testing.T) {
	dir := t.TempDir()
	mustScaffold(t, "KROWY", dir) // case-insensitive
	wantNonEmptyFiles(t, dir, filepath.Join("images", "prairie.png"))
}

func TestScaffoldEmbeddedThemeUnknown(t *testing.T) {
	ok, err := scaffoldEmbeddedTheme("no-such-theme", t.TempDir())
	if err != nil || ok {
		t.Fatalf("scaffoldEmbeddedTheme(no-such-theme) = %v, %v; want false, nil", ok, err)
	}
}

func TestEnsureTemplatesPrefersEmbeddedTheme(t *testing.T) {
	dir := t.TempDir()
	tplPath := filepath.Join(dir, "simple")
	g := &Generator{config: Config{Template: "simple"}}
	if err := g.ensureTemplates(tplPath); err != nil {
		t.Fatalf("ensureTemplates: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tplPath, "index.html"))
	if err != nil {
		t.Fatalf("expected embedded index.html: %v", err)
	}
	if strings.Contains(string(data), "Latest articles and updates") {
		t.Error("embedded theme expected, generic scaffold written instead")
	}
	if _, err := os.Stat(filepath.Join(tplPath, "css", "style.css")); err != nil {
		t.Errorf("expected theme stylesheet extracted: %v", err)
	}
}

func TestEnsureTemplatesGenericFallback(t *testing.T) {
	dir := t.TempDir()
	tplPath := filepath.Join(dir, "custom")
	g := &Generator{config: Config{Template: "custom"}}
	if err := g.ensureTemplates(tplPath); err != nil {
		t.Fatalf("ensureTemplates: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tplPath, "base.html"))
	if err != nil {
		t.Fatalf("expected generic base.html: %v", err)
	}
	s := string(data)
	// FE-011: no external font CDN; DOC-013: neutral English scaffold.
	if strings.Contains(s, "fonts.googleapis.com") {
		t.Error("generic scaffold must not reference external font CDNs")
	}
	if !strings.Contains(s, `lang="en"`) {
		t.Error("generic scaffold should default to lang=\"en\"")
	}

	// Existing templates are never overwritten.
	if err := g.ensureTemplates(tplPath); err != nil {
		t.Fatalf("ensureTemplates (second run): %v", err)
	}
}
