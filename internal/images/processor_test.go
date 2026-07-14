package images

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ── fixtures ────────────────────────────────────────────────────────────────

// testEnv builds a processor with one source root and isolated cache/output.
func testEnv(t *testing.T) (*Processor, string) {
	t.Helper()
	src := t.TempDir()
	p := New(Config{
		SourceDirs: []string{src},
		OutputDir:  t.TempDir(),
		CacheDir:   filepath.Join(t.TempDir(), "cache"),
		Quiet:      true,
	})
	return p, src
}

// writePNG creates a w×h PNG; alpha=true uses NRGBA with transparency.
func writePNG(t *testing.T, path string, w, h int, alpha bool) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x%2 == 0 {
				a = 128
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 40, A: a}) // #nosec G115 -- test pattern
		}
	}
	mustMkParent(t, path)
	f, err := os.Create(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// writeJPEG creates a plain baseline JPEG.
func writeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	mustMkParent(t, path)
	f, err := os.Create(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
}

// exifAPP1 builds an APP1 segment carrying only the orientation tag.
func exifAPP1(orientation uint16) []byte {
	tiff := make([]byte, 26)
	copy(tiff[0:], "II")                            // little endian
	binary.LittleEndian.PutUint16(tiff[2:], 42)     // TIFF magic
	binary.LittleEndian.PutUint32(tiff[4:], 8)      // IFD0 offset
	binary.LittleEndian.PutUint16(tiff[8:], 1)      // 1 entry
	binary.LittleEndian.PutUint16(tiff[10:], 0x112) // Orientation
	binary.LittleEndian.PutUint16(tiff[12:], 3)     // SHORT
	binary.LittleEndian.PutUint32(tiff[14:], 1)     // count
	binary.LittleEndian.PutUint16(tiff[18:], orientation)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2)) // #nosec G115 -- bounded test payload
	return append(seg, payload...)
}

// writeJPEGOriented splices an EXIF orientation into a real JPEG.
func writeJPEGOriented(t *testing.T, path string, w, h int, orientation uint16) {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	out := append([]byte{0xFF, 0xD8}, exifAPP1(orientation)...)
	out = append(out, raw[2:]...)
	mustMkParent(t, path)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeAnimatedGIF creates a two-frame GIF.
func writeAnimatedGIF(t *testing.T, path string) {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	frame := func() *image.Paletted { return image.NewPaletted(image.Rect(0, 0, 4, 4), pal) }
	g := &gif.GIF{Image: []*image.Paletted{frame(), frame()}, Delay: []int{10, 10}}
	mustMkParent(t, path)
	f, err := os.Create(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := gif.EncodeAll(f, g); err != nil {
		t.Fatal(err)
	}
}

func mustMkParent(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
}

func hashFile(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// ── metadata ────────────────────────────────────────────────────────────────

func TestInfo(t *testing.T) {
	p, src := testEnv(t)
	writeJPEG(t, filepath.Join(src, "photo.jpg"), 320, 200)
	writePNG(t, filepath.Join(src, "alpha.png"), 60, 40, true)
	writeJPEGOriented(t, filepath.Join(src, "rot.jpg"), 100, 50, 6)
	writeAnimatedGIF(t, filepath.Join(src, "anim.gif"))
	if err := os.WriteFile(filepath.Join(src, "junk.jpg"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := p.Info("photo.jpg")
	if err != nil || info.Width != 320 || info.Height != 200 || info.Format != "jpeg" {
		t.Errorf("jpeg info = %+v, %v", info, err)
	}
	if info.AspectRatio < 1.59 || info.AspectRatio > 1.61 {
		t.Errorf("aspect = %f", info.AspectRatio)
	}
	pngInfo, err := p.Info("alpha.png")
	if err != nil || !pngInfo.HasAlpha || pngInfo.Format != "png" {
		t.Errorf("png info = %+v, %v", pngInfo, err)
	}
	rot, err := p.Info("rot.jpg")
	if err != nil || rot.Orientation != 6 || rot.Width != 50 || rot.Height != 100 {
		t.Errorf("oriented info = %+v, %v (want swapped 50x100, orientation 6)", rot, err)
	}
	anim, err := p.Info("anim.gif")
	if err != nil || !anim.Animated {
		t.Errorf("animated info = %+v, %v", anim, err)
	}
	if _, err := p.Info("junk.jpg"); err == nil {
		t.Error("invalid file must error")
	}
	if _, err := p.Info("absent.png"); err == nil {
		t.Error("missing file must error")
	}
}

func TestResolveSecurity(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "ok.png"), 4, 4, false)

	if _, err := p.resolve("../secret.png"); err == nil {
		t.Error("path traversal must be rejected")
	}
	if _, err := p.resolve("/etc/passwd"); err == nil {
		t.Error("absolute paths must be rejected")
	}
	// Symlink escape: link inside root → file outside root.
	outside := filepath.Join(t.TempDir(), "outside.png")
	writePNG(t, outside, 4, 4, false)
	link := filepath.Join(src, "escape.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := p.resolve("escape.png"); err == nil {
		t.Error("symlink escape must be rejected")
	}
	if _, err := p.resolve("ok.png"); err != nil {
		t.Errorf("legit path rejected: %v", err)
	}
}

// ── resize ──────────────────────────────────────────────────────────────────

func TestResizeModes(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 400, 200, false)
	before := hashFile(t, filepath.Join(src, "img.png"))

	cases := []struct {
		name       string
		opts       map[string]any
		w, h       int
		expectFail string
	}{
		{"fit_width", map[string]any{"width": 200, "mode": "fit_width"}, 200, 100, ""},
		{"fit_height", map[string]any{"height": 50, "mode": "fit_height"}, 100, 50, ""},
		{"fit", map[string]any{"width": 100, "height": 100, "mode": "fit"}, 100, 50, ""},
		{"fill", map[string]any{"width": 100, "height": 100, "mode": "fill"}, 100, 100, ""},
		{"scale", map[string]any{"width": 120, "height": 30, "mode": "scale"}, 120, 30, ""},
		{"no upscale", map[string]any{"width": 800, "mode": "fit_width"}, 400, 200, ""},
		{"upscale on", map[string]any{"width": 800, "mode": "fit_width", "upscale": true}, 800, 400, ""},
		{"nearest resample", map[string]any{"width": 200, "mode": "fit_width", "resample": "nearest"}, 200, 100, ""},
		{"missing dims", map[string]any{"mode": "fill"}, 0, 0, "requires width and height"},
		{"bad mode", map[string]any{"width": 10, "mode": "stretch"}, 0, 0, `unsupported mode "stretch"`},
		{"bad quality", map[string]any{"width": 10, "quality": 500}, 0, 0, "quality must be"},
		{"unknown option", map[string]any{"widht": 10}, 0, 0, `unknown option "widht"`},
		{"bad format", map[string]any{"width": 10, "format": "heic"}, 0, 0, `unsupported output format "heic"`},
		{"bad resample", map[string]any{"width": 10, "resample": "bicubic"}, 0, 0, "unsupported resample"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := p.ResizeDict("img.png", c.opts)
			if c.expectFail != "" {
				if err == nil || !strings.Contains(err.Error(), c.expectFail) {
					t.Fatalf("err = %v, want contains %q", err, c.expectFail)
				}
				return
			}
			if err != nil {
				t.Fatalf("resize: %v", err)
			}
			if res.Width != c.w || res.Height != c.h {
				t.Errorf("dims = %dx%d, want %dx%d", res.Width, res.Height, c.w, c.h)
			}
			if _, err := os.Stat(filepath.Join(p.cfg.OutputDir, res.StaticPath)); err != nil {
				t.Errorf("published output missing: %v", err)
			}
		})
	}
	if hashFile(t, filepath.Join(src, "img.png")) != before {
		t.Error("source file was modified")
	}
}

// ── crop ────────────────────────────────────────────────────────────────────

func TestCropVariants(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 200, 100, false)

	for _, anchor := range []string{"center", "top", "bottom_left", "southeast"} {
		res, err := p.CropDict("img.png", map[string]any{"width": 50, "height": 40, "anchor": anchor})
		if err != nil || res.Width != 50 || res.Height != 40 {
			t.Errorf("anchor %s = %+v, %v", anchor, res, err)
		}
	}
	// Explicit rect + out-of-bounds clamping.
	res, err := p.CropDict("img.png", map[string]any{"x": 180, "y": 90, "width": 100, "height": 100})
	if err != nil {
		t.Fatalf("rect crop: %v", err)
	}
	if res.Width > 100 || res.Height > 100 || res.Width == 0 {
		t.Errorf("clamped rect = %dx%d", res.Width, res.Height)
	}
	// Focal point.
	if _, err := p.CropDict("img.png", map[string]any{"width": 60, "height": 60, "focusX": 0.9, "focusY": 0.1}); err != nil {
		t.Errorf("focal crop: %v", err)
	}
	// Validation errors.
	if _, err := p.CropDict("img.png", map[string]any{"width": 50, "height": 50, "focusX": 1.5}); err == nil {
		t.Error("focusX out of range must error")
	}
	if _, err := p.CropDict("img.png", map[string]any{"height": 50}); err == nil {
		t.Error("missing width must error")
	}
	if _, err := p.CropDict("img.png", map[string]any{"width": 50, "height": 50, "anchor": "middle"}); err == nil {
		t.Error("bad anchor must error")
	}
	// EXIF-normalized source: crop operates on upright pixels.
	writeJPEGOriented(t, filepath.Join(src, "rot.jpg"), 100, 50, 6)
	rres, err := p.CropDict("rot.jpg", map[string]any{"width": 50, "height": 100})
	if err != nil || rres.Width != 50 || rres.Height != 100 {
		t.Errorf("oriented crop = %+v, %v", rres, err)
	}
}

// ── formats & filters ───────────────────────────────────────────────────────

func TestFormatsAndFilters(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 80, 60, true)
	writeJPEG(t, filepath.Join(src, "photo.jpg"), 80, 60)

	// auto keeps the source format.
	res, err := p.ResizeDict("img.png", map[string]any{"width": 40})
	if err != nil || res.Format != "png" || !strings.HasSuffix(res.URL, ".png") {
		t.Errorf("auto png = %+v, %v", res, err)
	}
	jres, err := p.ResizeDict("photo.jpg", map[string]any{"width": 40, "quality": 70})
	if err != nil || jres.Format != "jpeg" || !strings.HasSuffix(jres.URL, ".jpg") {
		t.Errorf("auto jpeg = %+v, %v", jres, err)
	}
	// Explicit conversion png → jpeg.
	conv, err := p.ResizeDict("img.png", map[string]any{"width": 40, "format": "jpg"})
	if err != nil || conv.Format != "jpeg" {
		t.Errorf("png→jpeg = %+v, %v", conv, err)
	}

	// Every filter runs; deterministic across two calls (same cache key).
	filters := []any{
		map[string]any{"name": "grayscale"},
		map[string]any{"name": "invert"},
		map[string]any{"name": "sepia"},
		map[string]any{"name": "brightness", "amount": 0.2},
		map[string]any{"name": "contrast", "amount": 1.1},
		map[string]any{"name": "saturation", "amount": 0.5},
		map[string]any{"name": "gamma", "amount": 1.2},
		map[string]any{"name": "blur", "amount": 1.5},
		map[string]any{"name": "sharpen", "amount": 0.4},
		map[string]any{"name": "opacity", "amount": 0.9},
	}
	f1, err := p.FilterDict("img.png", filters, map[string]any{"format": "png"})
	if err != nil {
		t.Fatalf("filters: %v", err)
	}
	f2, err := p.FilterDict("img.png", filters, map[string]any{"format": "png"})
	if err != nil || f1.URL != f2.URL || f1.CacheKey != f2.CacheKey {
		t.Errorf("filters not deterministic: %v vs %v (%v)", f1.URL, f2.URL, err)
	}
	// Order participates in the key.
	reordered := append([]any{filters[1]}, filters[0])
	f3, err := p.FilterDict("img.png", reordered, map[string]any{"format": "png"})
	if err != nil {
		t.Fatal(err)
	}
	if f3.CacheKey == f1.CacheKey {
		t.Error("operation order must change the cache key")
	}
	// Invalid filter params.
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "blur", "amount": 500.0}}, nil); err == nil {
		t.Error("blur out of range must error")
	}
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "vortex"}}, nil); err == nil {
		t.Error("unknown filter must error")
	}
}

// ── pipeline, cache, concurrency ────────────────────────────────────────────

func TestProcessPipelineAndCache(t *testing.T) {
	p, src := testEnv(t)
	imgPath := filepath.Join(src, "img.png")
	writePNG(t, imgPath, 300, 200, false)

	ops := []any{
		map[string]any{"op": "crop", "width": 200, "height": 100, "anchor": "center"},
		map[string]any{"op": "resize", "width": 100, "mode": "fit_width"},
		map[string]any{"op": "filter", "name": "sharpen", "amount": 0.4},
		map[string]any{"op": "encode", "format": "png"},
	}
	r1, err := p.ProcessList("img.png", ops)
	if err != nil || r1.Width != 100 || r1.Height != 50 {
		t.Fatalf("pipeline = %+v, %v", r1, err)
	}
	// Cache hit: cached file untouched (mtime preserved).
	cachePath := filepath.Join(p.cfg.CacheDir, filepath.Base(r1.StaticPath))
	st1, _ := os.Stat(cachePath)
	r2, err := p.ProcessList("img.png", ops)
	if err != nil || r2.URL != r1.URL {
		t.Fatalf("second run = %+v, %v", r2, err)
	}
	st2, _ := os.Stat(cachePath)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Error("cache hit must not re-process")
	}
	// Source-content change → new key.
	writePNG(t, imgPath, 300, 200, true)
	r3, err := p.ProcessList("img.png", ops)
	if err != nil || r3.CacheKey == r1.CacheKey {
		t.Errorf("content change must change the key: %v vs %v (%v)", r3.CacheKey, r1.CacheKey, err)
	}
	// Quality change → new key.
	ops2 := append(ops[:3:3], map[string]any{"op": "encode", "format": "png", "quality": 50})
	r4, err := p.ProcessList("img.png", ops2)
	if err != nil || r4.CacheKey == r3.CacheKey {
		t.Errorf("quality change must change the key (%v)", err)
	}
	// Failing op is identified by index; no partial output.
	bad := []any{map[string]any{"op": "resize", "mode": "stretch", "width": 10}}
	if _, err := p.ProcessList("img.png", bad); err == nil || !strings.Contains(err.Error(), "operation 0") {
		t.Errorf("failing op index missing: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(p.cfg.CacheDir, "tmp-*")); len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
	// Unknown op.
	if _, err := p.ProcessList("img.png", []any{map[string]any{"op": "warp"}}); err == nil {
		t.Error("unknown op must error")
	}
}

func TestConcurrentIdenticalRequests(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 200, 200, false)
	opts := map[string]any{"width": 64, "mode": "fit_width"}

	var wg sync.WaitGroup
	results := make([]ImageResult, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r, err := p.ResizeDict("img.png", opts)
			if err != nil {
				t.Errorf("goroutine %d: %v", n, err)
				return
			}
			results[n] = r
		}(i)
	}
	wg.Wait()
	for _, r := range results {
		if r.URL != results[0].URL {
			t.Errorf("divergent results: %v vs %v", r.URL, results[0].URL)
		}
	}
	if leftovers, _ := filepath.Glob(filepath.Join(p.cfg.CacheDir, "tmp-*")); len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestGC(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 100, false)
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 50}); err != nil {
		t.Fatal(err)
	}
	// Plant an orphan and a stale temp file.
	if err := os.WriteFile(filepath.Join(p.cfg.CacheDir, "orphan.old.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.cfg.CacheDir, "tmp-stale.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, bytes, err := p.GC(true) // dry run
	if err != nil || files != 2 || bytes == 0 {
		t.Errorf("dry-run gc = %d files/%d bytes, %v", files, bytes, err)
	}
	if _, err := os.Stat(filepath.Join(p.cfg.CacheDir, "orphan.old.png")); err != nil {
		t.Error("dry run must not delete")
	}
	if _, _, err := p.GC(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.cfg.CacheDir, "orphan.old.png")); !os.IsNotExist(err) {
		t.Error("gc must remove orphans")
	}
	entries, _ := os.ReadDir(p.cfg.CacheDir)
	if len(entries) != 1 {
		t.Errorf("expected only the referenced variant to survive, got %d", len(entries))
	}
}

// ── srcset ──────────────────────────────────────────────────────────────────

func TestSrcSet(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "hero.png"), 1000, 500, false)

	set, err := p.SrcSetDict("hero.png", map[string]any{
		"widths": []any{480, 480, 1600, 240, -5}, // dupes, unsorted, invalid, too-big
	})
	if err != nil {
		t.Fatalf("srcset: %v", err)
	}
	if len(set.Images) != 2 { // 240 + 480; 1600 skipped (no upscale); -5 dropped
		t.Fatalf("variants = %d, want 2 (%+v)", len(set.Images), set.Images)
	}
	if set.Images[0].Width != 240 || set.Images[1].Width != 480 {
		t.Errorf("widths not sorted/deduped: %+v", set.Images)
	}
	if set.Default.Width != 480 {
		t.Errorf("default = %d, want largest 480", set.Default.Width)
	}
	if !strings.Contains(set.SrcSet, "240w") || !strings.Contains(set.SrcSet, "480w") ||
		!strings.Contains(set.SrcSet, ", ") {
		t.Errorf("srcset text = %q", set.SrcSet)
	}
	// Upscale allows bigger-than-source widths.
	up, err := p.SrcSetDict("hero.png", map[string]any{"widths": []any{1600}, "upscale": true})
	if err != nil || up.Default.Width != 1600 {
		t.Errorf("upscaled srcset = %+v, %v", up.Default, err)
	}
	// All widths above source without upscale → descriptive error.
	if _, err := p.SrcSetDict("hero.png", map[string]any{"widths": []any{2000}}); err == nil {
		t.Error("all-skipped widths must error")
	}
	// Errors: empty widths, variant limit, unknown option.
	if _, err := p.SrcSetDict("hero.png", map[string]any{}); err == nil {
		t.Error("missing widths must error")
	}
	limited := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir(), MaxVariants: 1})
	if _, err := limited.SrcSetDict("hero.png", map[string]any{"widths": []any{100, 200}}); err == nil {
		t.Error("variant limit must error")
	}
	if _, err := p.SrcSetDict("hero.png", map[string]any{"widhts": []any{100}}); err == nil {
		t.Error("unknown option must error")
	}
}

// ── limits ──────────────────────────────────────────────────────────────────

func TestResourceLimits(t *testing.T) {
	src := t.TempDir()
	writePNG(t, filepath.Join(src, "img.png"), 200, 200, false)
	p := New(Config{
		SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir(),
		MaxSourcePixels: 100, // absurdly small → source rejected
	})
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 50}); err == nil ||
		!strings.Contains(err.Error(), "max_source_pixels") {
		t.Errorf("source-pixel limit not enforced: %v", err)
	}
	p2 := New(Config{
		SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir(),
		MaxOutputPixels: 100,
	})
	if _, err := p2.ResizeDict("img.png", map[string]any{"width": 150}); err == nil ||
		!strings.Contains(err.Error(), "max_output_pixels") {
		t.Errorf("output-pixel limit not enforced: %v", err)
	}
	// Animated GIF policy: error, never silently flattened.
	writeAnimatedGIF(t, filepath.Join(src, "anim.gif"))
	p3 := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir()})
	if _, err := p3.ResizeDict("anim.gif", map[string]any{"width": 2}); err == nil ||
		!strings.Contains(err.Error(), "animated") {
		t.Errorf("animated policy not enforced: %v", err)
	}
}

// ── benchmarks ──────────────────────────────────────────────────────────────

func benchEnv(b *testing.B, w, h int) (*Processor, string) {
	b.Helper()
	src := b.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	f, _ := os.Create(filepath.Join(src, "bench.jpg")) // #nosec G304 -- bench fixture
	_ = jpeg.Encode(f, img, nil)
	_ = f.Close()
	return New(Config{SourceDirs: []string{src}, OutputDir: b.TempDir(), CacheDir: b.TempDir(), Quiet: true}), src
}

func BenchmarkInfo(b *testing.B) {
	p, _ := benchEnv(b, 1920, 1080)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Info("bench.jpg"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResizeJPEG(b *testing.B) {
	p, _ := benchEnv(b, 1920, 1080)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.mu.Lock()
		p.manifest = map[string]bool{}
		p.mu.Unlock()
		_ = os.RemoveAll(p.cfg.CacheDir)
		if _, err := p.ResizeDict("bench.jpg", map[string]any{"width": 800}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCacheHit(b *testing.B) {
	p, _ := benchEnv(b, 1920, 1080)
	if _, err := p.ResizeDict("bench.jpg", map[string]any{"width": 800}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.ResizeDict("bench.jpg", map[string]any{"width": 800}); err != nil {
			b.Fatal(err)
		}
	}
}
