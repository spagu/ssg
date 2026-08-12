package images

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestParsePictureOptions: defaultWidth/mode/formats parse into the request,
// including the pre-typed []string branch of the formats list.
func TestParsePictureOptions(t *testing.T) {
	got, err := ParsePicture(map[string]any{
		"formats":      []string{"jpeg"}, // template may hand a typed slice
		"widths":       []any{10, 20},
		"defaultWidth": 20,
		"mode":         "fit",
		"sizes":        "100vw",
		"alt":          "hero",
	})
	if err != nil {
		t.Fatalf("ParsePicture: %v", err)
	}
	if got.DefaultWidth != 20 || got.Base.Mode != "fit" || len(got.Formats) != 1 || got.Formats[0] != "jpeg" {
		t.Errorf("parsed options wrong: %+v", got)
	}
}

// TestParsePictureRejections: each parser guard yields a helper-prefixed error.
func TestParsePictureRejections(t *testing.T) {
	cases := []map[string]any{
		{"quality": "high"},                   // common option with a wrong type
		{"sizes": 7},                          // per-key type error
		{"widths": []any{10}, "quality": 500}, // parses, then validateCommon rejects
		{"formats": "webp"},                   // formats must be a list
		{"formats": []any{5}},                 // list element must be a string
	}
	for _, opts := range cases {
		if _, err := ParsePicture(opts); err == nil {
			t.Errorf("ParsePicture(%v) must error", opts)
		}
	}
}

// TestParseSrcSetValidateCommon: shared validation runs after parsing, so an
// unknown anchor is rejected even though it parsed as a string.
func TestParseSrcSetValidateCommon(t *testing.T) {
	if _, err := ParseSrcSet(map[string]any{"widths": []any{10}, "anchor": "weird"}); err == nil {
		t.Error("unknown anchor must fail srcset validation")
	}
}

// TestParseEncodeCommonTypeError: a common option with a wrong type surfaces
// from the encode-options parser.
func TestParseEncodeCommonTypeError(t *testing.T) {
	if _, err := ParseEncode("imageFilter", map[string]any{"quality": "high"}); err == nil {
		t.Error("string quality must fail the encode parser")
	}
}

// TestPictureDictParseError: the facade surfaces ParsePicture errors before any
// file I/O happens.
func TestPictureDictParseError(t *testing.T) {
	p, _ := testEnv(t)
	if _, err := p.PictureDict("img.png", map[string]any{"bogus": true}); err == nil {
		t.Error("unknown picture option must error")
	}
}

// TestPictureWarnsOnSkippedFormat: a non-quiet processor records and warns
// about a format whose encoder is missing, then falls back to the next format.
func TestPictureWarnsOnSkippedFormat(t *testing.T) {
	src := t.TempDir()
	writePNG(t, filepath.Join(src, "img.png"), 20, 10, false)
	loud := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir()}) // Quiet: false
	t.Setenv("PATH", t.TempDir())                                                                 // no cwebp/avifenc anywhere
	pic, err := loud.PictureDict("img.png", map[string]any{
		"formats": []any{"webp", "jpeg"}, "widths": []any{10},
	})
	if err != nil {
		t.Fatalf("PictureDict: %v", err)
	}
	if len(pic.Skipped) != 1 || pic.Skipped[0] != "webp" || pic.Fallback.Format != "jpeg" {
		t.Errorf("webp must be skipped with a jpeg fallback: %+v", pic)
	}
}

// TestPictureSrcSetErrorPropagates: a failing per-format variant set aborts the
// picture with the format named in the error.
func TestPictureSrcSetErrorPropagates(t *testing.T) {
	p, _ := testEnv(t)
	_, err := p.PictureDict("absent.png", map[string]any{
		"formats": []any{"jpeg"}, "widths": []any{10},
	})
	if err == nil || !strings.Contains(err.Error(), `format "jpeg"`) {
		t.Errorf("missing source must fail with the format named, got: %v", err)
	}
}
