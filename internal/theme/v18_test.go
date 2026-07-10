package theme

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConvertHugoTheme covers GO-010: layouts/static/assets are flattened into output.
func TestConvertHugoThemeFlatten(t *testing.T) {
	src := t.TempDir()
	for _, d := range []string{"layouts", "static", "assets"} {
		_ = os.MkdirAll(filepath.Join(src, d), 0755)
	}
	_ = os.WriteFile(filepath.Join(src, "layouts", "index.html"), []byte("<html></html>"), 0644)
	_ = os.WriteFile(filepath.Join(src, "static", "style.css"), []byte("a{}"), 0644)
	_ = os.WriteFile(filepath.Join(src, "assets", "app.js"), []byte("x"), 0644)

	out := t.TempDir()
	if err := ConvertHugoTheme(src, out); err != nil {
		t.Fatalf("ConvertHugoTheme: %v", err)
	}
	for _, f := range []string{"index.html", "style.css", "app.js"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected %s copied to output: %v", f, err)
		}
	}
}
