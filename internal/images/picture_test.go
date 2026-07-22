package images

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAVIFEncode exercises the avifenc path (skipped when the tool is absent)
// and the descriptive error when it is missing (issue #43).
func TestAVIFEncode(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 60, 40, false)

	if _, err := exec.LookPath("avifenc"); err == nil {
		res, aerr := p.ResizeDict("img.png", map[string]any{"width": 30, "format": "avif", "quality": 70})
		if aerr != nil || res.Format != "avif" || !strings.HasSuffix(res.URL, ".avif") {
			t.Errorf("avif encode = %+v, %v", res, aerr)
		}
	}
	// Force the missing-tool branch regardless of the host.
	t.Setenv("PATH", t.TempDir())
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 20, "format": "avif"}); err == nil ||
		!strings.Contains(err.Error(), "avifenc") {
		t.Errorf("missing avifenc must be a descriptive error, got: %v", err)
	}
}

// TestAVIFEncodeSurfacesToolError checks that a failing avifenc reports its stderr.
func TestAVIFEncodeSurfacesToolError(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 40, 30, false)
	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "avifenc")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir)
	_, err := p.ResizeDict("img.png", map[string]any{"width": 20, "format": "avif"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected the avifenc stderr surfaced, got: %v", err)
	}
}

// TestFormatEncodable covers the availability probe used for skip-with-warning.
func TestFormatEncodable(t *testing.T) {
	if !formatEncodable("jpeg") || !formatEncodable("png") || !formatEncodable("") {
		t.Fatal("stdlib formats must always be encodable")
	}
	if formatEncodable("tiff") {
		t.Fatal("unknown format must not be encodable")
	}
	// Force webp/avif tools absent, then both must report unavailable.
	t.Setenv("PATH", t.TempDir())
	if formatEncodable("webp") || formatEncodable("avif") {
		t.Fatal("webp/avif must be unavailable without their encoders")
	}
}

func TestFormatMIME(t *testing.T) {
	cases := map[string]string{"avif": "image/avif", "webp": "image/webp", "jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png", "gif": ""}
	for f, want := range cases {
		if got := formatMIME(f); got != want {
			t.Fatalf("formatMIME(%q) = %q, want %q", f, got, want)
		}
	}
}

// TestPictureFallbackOrderAndSkip verifies that unavailable formats are skipped
// with a record, and the last available format becomes the <img> fallback with
// the earlier formats emitted as ordered <source> elements.
func TestPictureFallbackOrderAndSkip(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "hero.png"), 200, 100, false)
	// Force webp/avif encoders absent so only jpeg survives.
	t.Setenv("PATH", t.TempDir())

	pic, err := p.PictureDict("hero.png", map[string]any{
		"formats": []any{"avif", "webp", "jpeg"},
		"widths":  []any{100, 200},
		"sizes":   "(min-width: 40rem) 50vw, 100vw",
		"alt":     "Hero & headline",
	})
	if err != nil {
		t.Fatalf("PictureDict: %v", err)
	}
	if len(pic.Skipped) != 2 {
		t.Fatalf("expected avif+webp skipped, got %v", pic.Skipped)
	}
	if len(pic.Sources) != 0 {
		t.Fatalf("only jpeg remains, so there should be no <source>, got %d", len(pic.Sources))
	}
	if pic.Fallback.Format != "jpeg" {
		t.Fatalf("fallback should be jpeg, got %q", pic.Fallback.Format)
	}
	if pic.Fallback.Width == 0 || pic.Fallback.Height == 0 {
		t.Fatal("fallback must carry width/height for zero CLS")
	}
	if !strings.Contains(pic.HTML, "loading=\"lazy\"") || !strings.Contains(pic.HTML, "sizes=\"(min-width: 40rem) 50vw, 100vw\"") {
		t.Fatalf("unexpected HTML: %s", pic.HTML)
	}
	if !strings.Contains(pic.HTML, "alt=\"Hero &amp; headline\"") {
		t.Fatalf("alt not escaped in HTML: %s", pic.HTML)
	}
}

// TestPictureWebpSourceWhenAvailable checks that, with cwebp present, webp is
// emitted as a <source> and jpeg is the <img> fallback.
func TestPictureWebpSourceWhenAvailable(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not installed")
	}
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "hero.png"), 200, 100, false)
	pic, err := p.PictureDict("hero.png", map[string]any{
		"formats": []any{"webp", "jpeg"},
		"widths":  []any{100, 200},
	})
	if err != nil {
		t.Fatalf("PictureDict: %v", err)
	}
	if len(pic.Sources) != 1 || pic.Sources[0].Type != "image/webp" {
		t.Fatalf("expected one webp <source>, got %+v", pic.Sources)
	}
	if pic.Fallback.Format != "jpeg" {
		t.Fatalf("fallback should be jpeg, got %q", pic.Fallback.Format)
	}
	if !strings.Contains(pic.HTML, "<source type=\"image/webp\"") {
		t.Fatalf("missing webp source in HTML: %s", pic.HTML)
	}
}

// TestRenderPictureHTML covers the <source> assembly deterministically,
// independent of which encoders the host has installed.
func TestRenderPictureHTML(t *testing.T) {
	pic := ImagePicture{
		Sources: []PictureSource{
			{Format: "avif", Type: "image/avif", SrcSet: "/a.avif 200w"},
			{Format: "webp", Type: "image/webp", SrcSet: "/a.webp 200w"},
		},
		Fallback: ImageResult{URL: "/a.jpg", Width: 200, Height: 100},
		Sizes:    "100vw",
		Alt:      "x",
	}
	html := renderPictureHTML(pic)
	for _, want := range []string{
		"<source type=\"image/avif\" srcset=\"/a.avif 200w\" sizes=\"100vw\">",
		"<source type=\"image/webp\" srcset=\"/a.webp 200w\" sizes=\"100vw\">",
		"<img src=\"/a.jpg\" width=\"200\" height=\"100\" sizes=\"100vw\" alt=\"x\" loading=\"lazy\" decoding=\"async\">",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in:\n%s", want, html)
		}
	}
}

func TestPictureNoWidths(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "x.png"), 50, 50, false)
	if _, err := p.PictureDict("x.png", map[string]any{"formats": []any{"jpeg"}}); err == nil {
		t.Fatal("expected an error when widths is missing")
	}
}

func TestPictureAllFormatsUnavailable(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "x.png"), 50, 50, false)
	t.Setenv("PATH", t.TempDir())
	if _, err := p.PictureDict("x.png", map[string]any{"formats": []any{"avif", "webp"}, "widths": []any{40}}); err == nil {
		t.Fatal("expected an error when no requested format can be encoded")
	}
}

func TestParsePictureRejectsFormat(t *testing.T) {
	if _, err := ParsePicture(map[string]any{"format": "webp", "widths": []any{40}}); err == nil {
		t.Fatal("expected ParsePicture to reject the singular \"format\" key")
	}
	if _, err := ParsePicture(map[string]any{"bogus": 1}); err == nil {
		t.Fatal("expected an unknown-option error")
	}
}

// TestPictureDefaultFormats confirms the default webp+jpeg pair and that cache
// keys stay per-format stable (jpeg fallback rendered twice = identical URL).
func TestPictureDefaultFormats(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "d.png"), 120, 80, false)
	t.Setenv("PATH", t.TempDir()) // webp unavailable → defaults collapse to jpeg
	pic, err := p.PictureDict("d.png", map[string]any{"widths": []any{120}})
	if err != nil {
		t.Fatalf("PictureDict: %v", err)
	}
	if pic.Fallback.Format != "jpeg" {
		t.Fatalf("default fallback should be jpeg, got %q", pic.Fallback.Format)
	}
	first := pic.Fallback.URL
	pic2, _ := p.PictureDict("d.png", map[string]any{"widths": []any{120}})
	if pic2.Fallback.URL != first {
		t.Fatalf("cache key unstable: %q vs %q", first, pic2.Fallback.URL)
	}
}
