package images

// Regression tests for issue #375: configured encoder qualities, AVIF cache
// hits, static GIF with `format: auto` and the <picture> fallback srcset.

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAVIF builds the smallest file the cache verifier accepts: an ftyp box
// with the avif brand followed by an ispe (image spatial extents) property.
func fakeAVIF(w, h uint32) []byte {
	b := []byte{0, 0, 0, 20}
	b = append(b, "ftypavif"...)
	b = append(b, 0, 0, 0, 0)
	b = append(b, "avif"...)
	b = append(b, 0, 0, 0, 20)
	b = append(b, "ispe"...)
	b = append(b, 0, 0, 0, 0)
	b = binary.BigEndian.AppendUint32(b, w)
	b = binary.BigEndian.AppendUint32(b, h)
	return b
}

// stubEncoder puts a fake `name` on PATH that logs its argv to the returned
// file and copies fixture to its last argument (the output path).
func stubEncoder(t *testing.T, name string, fixture []byte) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, name+".log")
	fixPath := filepath.Join(dir, name+".fixture")
	if err := os.WriteFile(fixPath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$@\" >> '" + logPath + "'\nfor last; do :; done\ncp '" + fixPath + "' \"$last\"\n"
	// #nosec G306 -- the stub must be executable
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// encoderCalls counts how many times a stub encoder ran.
func encoderCalls(t *testing.T, logPath string) int {
	t.Helper()
	data, err := os.ReadFile(logPath) // #nosec G304 -- test-owned log
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

// TestAVIFVariantHitsCacheOnSecondBuild: no AVIF decoder is registered, so the
// cached file used to fail verification and avifenc re-ran on every build.
func TestAVIFVariantHitsCacheOnSecondBuild(t *testing.T) {
	logPath := stubEncoder(t, "avifenc", fakeAVIF(30, 20))
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 60, 40, false)

	first, err := p.ResizeDict("img.png", map[string]any{"width": 30, "format": "avif"})
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	// A fresh processor over the same cache is the next build.
	next := New(p.cfg)
	second, err := next.ResizeDict("img.png", map[string]any{"width": 30, "format": "avif"})
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if n := encoderCalls(t, logPath); n != 1 {
		t.Fatalf("avifenc ran %d times, want 1 (second build must hit the cache)", n)
	}
	if second.URL != first.URL || second.Width != 30 || second.Height != 20 || second.Format != "avif" {
		t.Errorf("cached AVIF = %+v, first = %+v", second, first)
	}
}

// TestAVIFConfigAtRejectsBadFiles: a cached file that is not an AVIF with a
// readable ispe box is a cache miss, never a bogus result.
func TestAVIFConfigAtRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"short":      []byte("tiny"),
		"no-ftyp":    append([]byte{0, 0, 0, 8}, "free0000ispe"...),
		"no-ispe":    fakeAVIF(1, 1)[:20],
		"truncated":  fakeAVIF(1, 1)[:34],
		"zero-sized": fakeAVIF(0, 5),
	}
	for name, data := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := avifConfigAt(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := avifConfigAt(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file: expected an error")
	}
	good := filepath.Join(dir, "good.avif")
	if err := os.WriteFile(good, fakeAVIF(640, 480), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := avifConfigAt(good)
	if err != nil || cfg.Width != 640 || cfg.Height != 480 {
		t.Errorf("good AVIF = %+v, %v", cfg, err)
	}
}

// TestCachedAVIFCorruptIsMiss: a corrupt cached AVIF is re-encoded.
func TestCachedAVIFCorruptIsMiss(t *testing.T) {
	logPath := stubEncoder(t, "avifenc", []byte("not an avif"))
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 60, 40, false)
	for i := 0; i < 2; i++ {
		if _, err := p.ResizeDict("img.png", map[string]any{"width": 30, "format": "avif"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := encoderCalls(t, logPath); n != 2 {
		t.Fatalf("a corrupt cache entry must not count as a hit: avifenc ran %d times", n)
	}
}

// writeStaticGIF creates a single-frame GIF.
func writeStaticGIF(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White})
	mustMkParent(t, path)
	f, err := os.Create(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := gif.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

// TestStaticGIFAutoFormat: there is no GIF encoder, so `auto` on a GIF source
// publishes PNG (lossless, keeps transparency) instead of failing.
func TestStaticGIFAutoFormat(t *testing.T) {
	p, src := testEnv(t)
	writeStaticGIF(t, filepath.Join(src, "anim.gif"), 40, 20)
	for _, opts := range []map[string]any{
		{"width": 20, "format": "auto"},
		{"width": 20},
	} {
		res, err := p.ResizeDict("anim.gif", opts)
		if err != nil {
			t.Fatalf("%v: %v", opts, err)
		}
		if res.Format != "png" || !strings.HasSuffix(res.URL, ".png") {
			t.Errorf("%v: GIF auto = %+v, want png", opts, res)
		}
		// Second call is served from the cache under the same name.
		again, err := p.ResizeDict("anim.gif", opts)
		if err != nil || again.URL != res.URL || again.Format != "png" {
			t.Errorf("%v: cached GIF auto = %+v, %v", opts, again, err)
		}
	}
}

// TestConfiguredQualityReachesEncoder: with no per-call quality the configured
// WebP/AVIF quality is what the encoder receives.
func TestConfiguredQualityReachesEncoder(t *testing.T) {
	webpLog := stubEncoder(t, "cwebp", []byte("RIFF"))
	avifLog := stubEncoder(t, "avifenc", fakeAVIF(30, 20))
	src := t.TempDir()
	writePNG(t, filepath.Join(src, "img.png"), 60, 40, false)
	p := New(Config{
		SourceDirs: []string{src}, OutputDir: t.TempDir(),
		CacheDir: filepath.Join(t.TempDir(), "cache"), Quiet: true,
		WebPQuality: 37, AVIFQuality: 29,
	})
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 30, "format": "webp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 30, "format": "avif"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(webpLog); !strings.Contains(string(data), "-q 37") { // #nosec G304 -- test log
		t.Errorf("cwebp args = %q, want -q 37", data)
	}
	if data, _ := os.ReadFile(avifLog); !strings.Contains(string(data), "-q 29") { // #nosec G304 -- test log
		t.Errorf("avifenc args = %q, want -q 29", data)
	}
}

// TestCacheKeyIncludesConfiguredQuality: changing webp_quality/avif_quality
// must produce new variants instead of serving ones encoded at the old setting;
// an explicit per-call quality keeps the historical key.
func TestCacheKeyIncludesConfiguredQuality(t *testing.T) {
	src := t.TempDir()
	path := filepath.Join(src, "img.png")
	writePNG(t, path, 8, 8, false)
	low := New(Config{SourceDirs: []string{src}, WebPQuality: 40, AVIFQuality: 30})
	high := New(Config{SourceDirs: []string{src}, WebPQuality: 90, AVIFQuality: 70})
	key := func(p *Processor, ops []request) string {
		t.Helper()
		k, err := p.cacheKey(path, ops)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	for _, format := range []string{"webp", "avif"} {
		implicit := []request{{Op: "resize", Width: 4, Format: format}}
		if key(low, implicit) == key(high, implicit) {
			t.Errorf("%s: configured quality must be part of the cache key", format)
		}
		explicit := []request{{Op: "resize", Width: 4, Format: format, Quality: 50}}
		if key(low, explicit) != key(high, explicit) {
			t.Errorf("%s: explicit quality must not depend on the configured one", format)
		}
	}
	// Lossless/JPEG output keys are unaffected by the WebP/AVIF settings.
	for _, format := range []string{"png", "jpeg", ""} {
		ops := []request{{Op: "resize", Width: 4, Format: format}}
		if key(low, ops) != key(high, ops) {
			t.Errorf("%q: key must not depend on WebP/AVIF quality", format)
		}
	}
}

// TestDefaultQualitiesMatchBuildPipeline: unset qualities fall back to the
// same defaults as the --webp/image_formats pipeline (WebP 60, AVIF 45).
func TestDefaultQualitiesMatchBuildPipeline(t *testing.T) {
	d := New(Config{})
	if d.cfg.WebPQuality != 60 || d.cfg.AVIFQuality != 45 {
		t.Errorf("defaults WebP=%d AVIF=%d, want 60/45", d.cfg.WebPQuality, d.cfg.AVIFQuality)
	}
}

// TestPictureFallbackCarriesSrcSet: the <img> fallback gets its own format's
// srcset, so a browser without <source> support still picks a fitting width.
func TestPictureFallbackCarriesSrcSet(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "hero.png"), 200, 100, false)
	pic, err := p.PictureDict("hero.png", map[string]any{
		"formats": []any{"jpeg"},
		"widths":  []any{100, 200},
		"sizes":   "50vw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pic.FallbackSrcSet, "100w") || !strings.Contains(pic.FallbackSrcSet, "200w") {
		t.Fatalf("FallbackSrcSet = %q", pic.FallbackSrcSet)
	}
	if !strings.Contains(pic.HTML, `<img src="`+pic.Fallback.URL+`"`) ||
		!strings.Contains(pic.HTML, ` srcset="`+pic.FallbackSrcSet+`" sizes="50vw"`) {
		t.Errorf("fallback <img> lacks srcset: %s", pic.HTML)
	}
}
