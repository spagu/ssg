package generator

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// writeTestPNG drops a small PNG fixture for the image helpers.
func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: 200, A: 255})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// imageTestGen wires a Generator whose static dir holds pic.png.
func imageTestGen(t *testing.T) *Generator {
	t.Helper()
	g := newTestGen(t, "")
	staticDir := t.TempDir()
	writeTestPNG(t, filepath.Join(staticDir, "pic.png"), 64, 32)
	g.config.StaticDir = staticDir
	return g
}

// TestImageHelpersIntegration renders templates through the real FuncMap and
// verifies HTML output, the generated file, format and dimensions — and that a
// second render is served from the cache (same URL, untouched variant).
func TestImageHelpersIntegration(t *testing.T) {
	g := imageTestGen(t)
	render := func(src string) string {
		t.Helper()
		tmpl, err := template.New("t").Funcs(g.buildTemplateFuncs(map[string]string{})).Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var sb strings.Builder
		if err := tmpl.Execute(&sb, nil); err != nil {
			t.Fatalf("execute: %v", err)
		}
		return sb.String()
	}

	out := render(`{{ $i := imageResize "pic.png" (dict "width" 32) }}` +
		`<img src="{{ $i.URL }}" width="{{ $i.Width }}" height="{{ $i.Height }}">`)
	if !strings.Contains(out, `/processed_images/pic.`) ||
		!strings.Contains(out, `width="32" height="16"`) {
		t.Errorf("imageResize render = %q", out)
	}
	rel := strings.TrimPrefix(strings.Split(strings.Split(out, `src="`)[1], `"`)[0], "/")
	variant := filepath.Join(g.config.OutputDir, filepath.FromSlash(rel))
	st, err := os.Stat(variant)
	if err != nil {
		t.Fatalf("generated variant missing: %v", err)
	}

	// Second render: cache hit → identical URL, cache file untouched.
	out2 := render(`{{ (imageResize "pic.png" (dict "width" 32)).URL }}`)
	if !strings.Contains(out, out2) {
		t.Errorf("cache produced a different URL: %q vs %q", out, out2)
	}
	st2, _ := os.Stat(variant)
	if !st.ModTime().Equal(st2.ModTime()) {
		t.Error("cache hit must not rewrite the published variant")
	}

	// imageInfo + imageSrcSet through templates.
	info := render(`{{ $n := imageInfo "pic.png" }}{{ $n.Width }}x{{ $n.Height }} {{ $n.Format }}`)
	if info != "64x32 png" {
		t.Errorf("imageInfo render = %q", info)
	}
	set := render(`{{ $s := imageSrcSet "pic.png" (dict "widths" (slice 16 32)) }}{{ $s.SrcSet }}`)
	if !strings.Contains(set, "16w") || !strings.Contains(set, "32w") {
		t.Errorf("imageSrcSet render = %q", set)
	}

	// Errors surface as template errors (never panics).
	tmpl := template.Must(template.New("t").Funcs(g.buildTemplateFuncs(map[string]string{})).
		Parse(`{{ imageResize "pic.png" (dict "widht" 5) }}`))
	var sb strings.Builder
	if err := tmpl.Execute(&sb, nil); err == nil || !strings.Contains(err.Error(), `unknown option "widht"`) {
		t.Errorf("expected descriptive template error, got %v", err)
	}

	// Shortcode templates share the helpers.
	if _, ok := g.shortcodeFuncMap()["imageResize"]; !ok {
		t.Error("imageResize must be registered for shortcode templates")
	}

	// ImagesGC through the generator facade.
	if _, _, err := g.ImagesGC(true); err != nil {
		t.Errorf("ImagesGC: %v", err)
	}
}
