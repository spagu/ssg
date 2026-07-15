package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// DOC-013: bundled themes are extracted from the binary, unknown themes fall
// back to the generic scaffold.
func TestScaffoldEmbeddedTheme(t *testing.T) {
	t.Run("simple extracts full tree", func(t *testing.T) {
		dir := t.TempDir()
		ok, err := scaffoldEmbeddedTheme("simple", dir)
		if err != nil || !ok {
			t.Fatalf("scaffoldEmbeddedTheme(simple) = %v, %v; want true, nil", ok, err)
		}
		for _, f := range []string{"index.html", "post.html", "page.html", "category.html", filepath.Join("css", "style.css"), filepath.Join("js", "main.js")} {
			if fi, err := os.Stat(filepath.Join(dir, f)); err != nil || fi.Size() == 0 {
				t.Errorf("expected extracted file %s, got err=%v", f, err)
			}
		}
	})

	t.Run("krowy extracts assets", func(t *testing.T) {
		dir := t.TempDir()
		ok, err := scaffoldEmbeddedTheme("KROWY", dir) // case-insensitive
		if err != nil || !ok {
			t.Fatalf("scaffoldEmbeddedTheme(KROWY) = %v, %v; want true, nil", ok, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "images", "prairie.png")); err != nil {
			t.Errorf("expected krowy image asset: %v", err)
		}
	})

	t.Run("unknown theme reports false", func(t *testing.T) {
		ok, err := scaffoldEmbeddedTheme("no-such-theme", t.TempDir())
		if err != nil || ok {
			t.Fatalf("scaffoldEmbeddedTheme(no-such-theme) = %v, %v; want false, nil", ok, err)
		}
	})
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
