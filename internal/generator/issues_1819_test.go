package generator

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestGetOutputPathsRespectsExplicitExtension covers the generator half of #81:
// a sub-path that already names a file is written as-is, whatever page_format
// says. Before this it produced "validator.html.html" under flat and a directory
// literally named "validator.html/" under directory.
func TestGetOutputPathsRespectsExplicitExtension(t *testing.T) {
	g := newTestGen(t, "")
	for _, format := range []string{"flat", "directory", "both", ""} {
		g.config.PageFormat = format
		got := g.getOutputPaths("validator.html")
		if len(got) != 1 || filepath.Base(got[0]) != "validator.html" {
			t.Errorf("page_format=%q: got %v, want a single validator.html", format, got)
		}
		if strings.Contains(got[0], "validator.html.html") {
			t.Errorf("page_format=%q: extension appended twice: %v", format, got)
		}
	}
	// A path with no extension keeps the configured behaviour.
	g.config.PageFormat = "flat"
	if got := g.getOutputPaths("about"); !strings.HasSuffix(got[0], "about.html") {
		t.Errorf("flat should still suffix an extensionless path: %v", got)
	}
	g.config.PageFormat = "directory"
	if got := g.getOutputPaths("about"); !strings.HasSuffix(got[0], filepath.Join("about", indexHTMLName)) {
		t.Errorf("directory should still nest an extensionless path: %v", got)
	}
}

// TestTmplRaw covers #83: raw is a plain template.HTML cast, so markup coming
// from data reaches the page untouched. safeHTML is not interchangeable — in a
// page template it runs the Markdown pipeline, which wraps an SVG fragment in a
// <p> and silently stops it drawing.
func TestTmplRaw(t *testing.T) {
	const svg = `<path d="M18 6 6 18" /><path d="m6 6 12 12" />`
	if got := tmplRaw(svg); string(got) != svg {
		t.Errorf("raw altered its input: %q", got)
	}
	if got := tmplRaw(template.HTML(svg)); string(got) != svg {
		t.Errorf("raw must pass template.HTML through: %q", got)
	}
	if got := tmplRaw(nil); got != "" {
		t.Errorf("raw(nil) = %q, want empty", got)
	}
	if got := tmplRaw(42); string(got) != "42" {
		t.Errorf("raw(42) = %q, want 42", got)
	}
	// It is registered for page templates and for shortcodes alike.
	g := newTestGen(t, "")
	if _, ok := g.buildTemplateFuncs(nil)["raw"]; !ok {
		t.Error("raw must be registered in the page template funcs")
	}
	if _, ok := g.buildTemplateFuncs(nil)["html"]; !ok {
		t.Error("html alias must be registered")
	}
}

// TestCopyStaticSources covers #84: several verbatim roots, each keeping its own
// name so existing public URLs keep resolving, with dest for placement and "."
// for the static_dir-style splat at the output root.
func TestCopyStaticSources(t *testing.T) {
	g := newTestGen(t, "")
	src := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("schema.json", `{"v":1}`)
	write("xml/1.0/s.xsd", "<xsd/>")
	write("editor/index.html", "<html>e</html>")
	write("loose/a.txt", "a")

	g.config.StaticSources = []models.StaticSource{
		{Path: filepath.Join(src, "schema.json")},
		{Path: filepath.Join(src, "xml")},
		{Path: filepath.Join(src, "editor"), Dest: "app"},
		{Path: filepath.Join(src, "loose"), Dest: "."},
		{Path: filepath.Join(src, "missing")}, // skipped with a warning, not fatal
	}
	if err := g.copyStaticSources(); err != nil {
		t.Fatalf("copyStaticSources: %v", err)
	}

	for _, want := range []string{
		"schema.json",    // a file keeps its name at the root
		"xml/1.0/s.xsd",  // a directory keeps its own name — the URL must survive
		"app/index.html", // dest relocates it
		"a.txt",          // dest "." splats the contents, like static_dir
	} {
		if _, err := os.Stat(filepath.Join(g.config.OutputDir, filepath.FromSlash(want))); err != nil {
			t.Errorf("expected %s in the output: %v", want, err)
		}
	}
	// The directory name must NOT be flattened away.
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "1.0", "s.xsd")); err == nil {
		t.Error("xml/ contents were splatted at the root; the directory name must be kept")
	}
}
