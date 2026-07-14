package images

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestWebPEncode exercises the cwebp path (skipped when the tool is absent) and
// the descriptive error when it is missing.
func TestWebPEncode(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 60, 40, true)

	if _, err := exec.LookPath("cwebp"); err == nil {
		res, werr := p.ResizeDict("img.png", map[string]any{"width": 30, "format": "webp", "quality": 70})
		if werr != nil || res.Format != "webp" || !strings.HasSuffix(res.URL, ".webp") {
			t.Errorf("webp encode = %+v, %v", res, werr)
		}
	}
	// Force the missing-tool branch regardless of the host.
	t.Setenv("PATH", t.TempDir())
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 20, "format": "webp"}); err == nil ||
		!strings.Contains(err.Error(), "cwebp") {
		t.Errorf("missing cwebp must be a descriptive error, got: %v", err)
	}
}

// TestOrientations normalizes every EXIF orientation (2–8) and checks the
// resulting upright dimensions.
func TestOrientations(t *testing.T) {
	p, src := testEnv(t)
	for o := 2; o <= 8; o++ {
		name := filepath.Join(src, "o.jpg")
		writeJPEGOriented(t, name, 40, 20, uint16(o)) // #nosec G115 -- 2..8
		wantW, wantH := 40, 20
		if o >= 5 { // 90/270° rotations swap dimensions
			wantW, wantH = 20, 40
		}
		res, err := p.ResizeDict("o.jpg", map[string]any{"width": wantW, "height": wantH, "mode": "scale", "upscale": true})
		if err != nil || res.Width != wantW || res.Height != wantH {
			t.Errorf("orientation %d = %+v, %v (want %dx%d)", o, res, err, wantW, wantH)
		}
	}
	// Corrupt EXIF variants fall back to orientation 1.
	if got := exifOrientation(bytes.NewReader([]byte{0x00})); got != 1 {
		t.Errorf("non-JPEG = %d", got)
	}
	if got := exifOrientation(bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xD9})); got != 1 {
		t.Errorf("EOI-only = %d", got)
	}
	if got := orientationFromTIFF([]byte("XX")); got != 1 {
		t.Errorf("bad TIFF = %d", got)
	}
	if got := orientationFromTIFF([]byte("MM\x00\x2a\x00\x00\x00\x08\x00\x00")); got != 1 {
		t.Errorf("empty IFD = %d", got)
	}
}

// TestClampFillAndFitDefaults covers no-upscale fill shrinking and fit with a
// single bound.
func TestClampFillAndFitDefaults(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)

	// fill 400x100 from 100x50 without upscale → shrunk to 4:1 inside source.
	res, err := p.ResizeDict("img.png", map[string]any{"width": 400, "height": 100, "mode": "fill"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 100 || res.Height != 25 {
		t.Errorf("clamped fill = %dx%d, want 100x25", res.Width, res.Height)
	}
	// scale asking beyond source without upscale → unchanged.
	same, err := p.ResizeDict("img.png", map[string]any{"width": 500, "height": 400, "mode": "scale"})
	if err != nil || same.Width != 100 || same.Height != 50 {
		t.Errorf("no-upscale scale = %+v, %v", same, err)
	}
	// fit with only height.
	fh, err := p.ResizeDict("img.png", map[string]any{"height": 25, "mode": "fit"})
	if err != nil || fh.Height != 25 || fh.Width != 50 {
		t.Errorf("fit height-only = %+v, %v", fh, err)
	}
	if w, h := clampFill(100, 50, 40, 20); w != 40 || h != 20 {
		t.Errorf("clampFill within source = %dx%d", w, h)
	}
	if w, h := clampFill(2, 2, 4000, 1); w < 1 || h < 1 {
		t.Errorf("clampFill floor = %dx%d", w, h)
	}
}

// TestOptionCoercions covers numeric type flexibility and type errors across
// the dict parsers.
func TestOptionCoercions(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 50, 50, false)

	// int64/float64 numbers and []int widths all accepted.
	if _, err := p.ResizeDict("img.png", map[string]any{"width": int64(20)}); err != nil {
		t.Errorf("int64 width: %v", err)
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 20.0}); err != nil {
		t.Errorf("float width: %v", err)
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []int{10, 20}}); err != nil {
		t.Errorf("[]int widths: %v", err)
	}
	// Type errors.
	if _, err := p.ResizeDict("img.png", map[string]any{"width": "big"}); err == nil {
		t.Error("string width must error")
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"mode": 5}); err == nil {
		t.Error("numeric mode must error")
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"upscale": "yes"}); err == nil {
		t.Error("string upscale must error")
	}
	if _, err := p.CropDict("img.png", map[string]any{"width": 10, "height": 10, "focusX": "left"}); err == nil {
		t.Error("string focusX must error")
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": "480"}); err == nil {
		t.Error("string widths must error")
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{"480"}}); err == nil {
		t.Error("string width element must error")
	}
	if _, err := p.FilterDict("img.png", []any{"grayscale"}, nil); err == nil {
		t.Error("non-dict filter must error")
	}
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": 5}}, nil); err == nil {
		t.Error("numeric filter name must error")
	}
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "blur", "radius": 2.0}}, nil); err == nil {
		t.Error("unknown filter key must error")
	}
	if _, err := p.FilterDict("img.png", nil, map[string]any{"speed": 5}); err == nil {
		t.Error("unknown encode key must error")
	}
	if _, err := p.ProcessList("img.png", []any{"resize"}); err == nil {
		t.Error("non-dict op must error")
	}
	if _, err := p.CropDict("img.png", map[string]any{"x": -1, "y": 0, "width": 5, "height": 5}); err == nil {
		t.Error("negative rect origin must error")
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"width": -3}); err == nil {
		t.Error("negative width must error")
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 10, "lossless": true, "format": "png"}); err != nil {
		t.Errorf("lossless flag should parse: %v", err)
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{10, 20}, "defaultWidth": 10}); err != nil {
		t.Errorf("defaultWidth should parse: %v", err)
	}
}

// TestFormatHelpers covers formatFromPath/extFor/finalFormat fallbacks.
func TestFormatHelpers(t *testing.T) {
	cases := map[string]string{"a.jpg": "jpeg", "b.JPEG": "jpeg", "c.png": "png", "d.webp": "webp", "e.gif": "gif", "f.txt": ""}
	for in, want := range cases {
		if got := formatFromPath(in); got != want {
			t.Errorf("formatFromPath(%q) = %q, want %q", in, got, want)
		}
	}
	if extFor("jpeg") != "jpg" || extFor("png") != "png" {
		t.Error("extFor mapping broken")
	}
	if finalFormat(nil, "") != "png" {
		t.Error("finalFormat fallback should be png")
	}
	if finalFormat([]request{{Format: "jpg"}}, "png") != "jpeg" {
		t.Error("jpg alias should normalize to jpeg")
	}
	if safeImgArg("rel.png") != "./rel.png" || safeImgArg("/abs.png") != "/abs.png" || safeImgArg("") != "" {
		t.Error("safeImgArg hardening broken")
	}
}

// TestGeneratorImageHelpersIntegration is exercised from the generator package;
// here we cover the GC empty-dir edge and the unsupported-encode format guard.
func TestMiscEdges(t *testing.T) {
	p := New(Config{SourceDirs: []string{t.TempDir()}, OutputDir: t.TempDir(), CacheDir: filepath.Join(t.TempDir(), "nope")})
	if files, bytes, err := p.GC(false); err != nil || files != 0 || bytes != 0 {
		t.Errorf("gc on missing cache dir = %d/%d, %v", files, bytes, err)
	}
	var buf bytes.Buffer
	_ = buf
	f, err := os.CreateTemp(t.TempDir(), "x-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := p.encode(f, img, "heic", 80); err == nil {
		t.Error("unsupported encode format must error")
	}
	if err := p.encode(f, img, "jpeg", 0); err != nil { // default quality path
		t.Errorf("jpeg default quality: %v", err)
	}
	// jpeg default helper sanity: encode wrote something.
	st, _ := f.Stat()
	if st.Size() == 0 {
		t.Error("jpeg encode produced no bytes")
	}
	_ = jpeg.Decode // keep import for fixture parity
}

// TestErrorInjection drives the remaining I/O and corrupt-input guards.
func TestErrorInjection(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 30, 30, false)

	// cacheKey: unreadable source.
	if _, err := p.cacheKey(filepath.Join(t.TempDir(), "absent.png"), nil); err == nil {
		t.Error("cacheKey on missing file must error")
	}
	// withinRoot: nonexistent root errors.
	if _, err := withinRoot(filepath.Join(t.TempDir(), "noroot"), "x"); err == nil {
		t.Error("withinRoot missing root must error")
	}
	// copyFile: missing source; parent blocked by a regular file.
	if err := copyFile(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "out.png")); err == nil {
		t.Error("copyFile missing src must error")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(src, "img.png"), filepath.Join(blocker, "sub", "out.png")); err == nil {
		t.Error("copyFile blocked parent must error")
	}
	// publish: cache dir path occupied by a regular file → MkdirAll fails.
	pBad := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: blocker})
	if _, err := pBad.ResizeDict("img.png", map[string]any{"width": 10}); err == nil {
		t.Error("publish with blocked cache dir must error")
	}
	// cached: corrupt cache entry → treated as miss and reprocessed fine.
	res, err := p.ResizeDict("img.png", map[string]any{"width": 10})
	if err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(p.cfg.CacheDir, filepath.Base(res.StaticPath))
	if err := os.WriteFile(cachePath, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := p.ResizeDict("img.png", map[string]any{"width": 10})
	if err != nil || res2.Width != 10 {
		t.Errorf("corrupt cache entry must reprocess: %+v, %v", res2, err)
	}
	// encodeWebP: fake failing cwebp.
	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "cwebp")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), 0o755); err != nil { // #nosec G306 -- test executable
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir)
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 12, "format": "webp"}); err == nil ||
		!strings.Contains(err.Error(), "boom") {
		t.Errorf("failing cwebp must surface stderr: %v", err)
	}
}

// TestCorruptEXIFVariants walks nextJPEGSegment/orientationFromTIFF edge cases.
func TestCorruptEXIFVariants(t *testing.T) {
	cases := [][]byte{
		{0xFF, 0xD8, 0x00, 0xE1},                         // marker byte not 0xFF
		{0xFF, 0xD8, 0xFF, 0xE1},                         // truncated size
		{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01},             // size < 2
		{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 0x41},       // truncated payload
		{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0xFF, 0xDA}, // APP0 then scan start
		append([]byte{0xFF, 0xD8}, exifAPP1(0)...),       // orientation 0 → fallback 1
		append([]byte{0xFF, 0xD8}, truncatedExif()...),   // TIFF too short
	}
	for i, c := range cases {
		if got := exifOrientation(bytes.NewReader(c)); got != 1 {
			t.Errorf("case %d: orientation = %d, want fallback 1", i, got)
		}
	}
	// IFD entry beyond buffer + non-orientation tag then exhaustion.
	tiff := []byte("II\x2a\x00\x08\x00\x00\x00\x02\x00")
	if got := orientationFromTIFF(tiff); got != 1 {
		t.Errorf("short entries = %d", got)
	}
	full := make([]byte, 26)
	copy(full, "II")
	binary.LittleEndian.PutUint16(full[2:], 42)
	binary.LittleEndian.PutUint32(full[4:], 8)
	binary.LittleEndian.PutUint16(full[8:], 1)
	binary.LittleEndian.PutUint16(full[10:], 0x0100) // width tag, not orientation
	if got := orientationFromTIFF(full); got != 1 {
		t.Errorf("no orientation tag = %d", got)
	}
}

// truncatedExif returns an APP1 whose TIFF payload is too short.
func truncatedExif() []byte {
	payload := []byte("Exif\x00\x00II")
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	return append(seg, payload...)
}

// TestFocalClampExtremes drives clampInt's both clamps.
func TestFocalClampExtremes(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 100, false)
	for _, f := range []float64{0, 1} {
		res, err := p.CropDict("img.png", map[string]any{"width": 90, "height": 90, "focusX": f, "focusY": f})
		if err != nil || res.Width != 90 || res.Height != 90 {
			t.Errorf("focal %v = %+v, %v", f, res, err)
		}
	}
}

// TestIntAmountFilter covers optFloat's int/int64 acceptance.
func TestIntAmountFilter(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 20, 20, false)
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "gamma", "amount": 2}}, nil); err != nil {
		t.Errorf("int amount: %v", err)
	}
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "gamma", "amount": int64(2)}}, nil); err != nil {
		t.Errorf("int64 amount: %v", err)
	}
}

// TestValidationGaps sweeps the remaining per-mode and per-parser guards.
func TestValidationGaps(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)

	// Mode-specific missing dimensions.
	for _, c := range []map[string]any{
		{"mode": "fit_width"},
		{"mode": "fit_height"},
		{"mode": "fit"},
		{"mode": "scale", "width": 10},
	} {
		if _, err := p.ResizeDict("img.png", c); err == nil {
			t.Errorf("opts %v must error", c)
		}
	}
	// focusY range (focusX already covered).
	if _, err := p.CropDict("img.png", map[string]any{"width": 10, "height": 10, "focusY": -0.2}); err == nil {
		t.Error("focusY out of range must error")
	}
	// Parser type errors not yet hit.
	if _, err := p.CropDict("img.png", map[string]any{"width": "big", "height": 10}); err == nil {
		t.Error("crop width type must error")
	}
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "gamma", "amount": "big"}}, nil); err == nil {
		t.Error("filter amount type must error")
	}
	if _, err := p.ProcessList("img.png", []any{map[string]any{"op": "crop", "widht": 5}}); err == nil {
		t.Error("crop op unknown key must error")
	}
	if _, err := p.ProcessList("img.png", []any{map[string]any{"op": "filter", "name": "blur", "amount": 999.0}}); err == nil {
		t.Error("filter op range must error")
	}
	if _, err := p.ProcessList("img.png", []any{map[string]any{"op": "encode", "quality": 999}}); err == nil {
		t.Error("encode op quality must error")
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{10}, "format": 5}); err == nil {
		t.Error("srcset format type must error")
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{10}, "defaultWidth": "big"}); err == nil {
		t.Error("srcset defaultWidth type must error")
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{10}, "mode": 7}); err == nil {
		t.Error("srcset mode type must error")
	}
	if _, err := p.FilterDict("img.png", nil, map[string]any{"quality": 999}); err == nil {
		t.Error("filter encode quality must error")
	}

	// Resize helpers: upscale fill/scale beyond source, fit width-only default.
	up, err := p.ResizeDict("img.png", map[string]any{"width": 200, "height": 100, "mode": "fill", "upscale": true})
	if err != nil || up.Width != 200 || up.Height != 100 {
		t.Errorf("upscaled fill = %+v, %v", up, err)
	}
	us, err := p.ResizeDict("img.png", map[string]any{"width": 200, "height": 100, "mode": "scale", "upscale": true})
	if err != nil || us.Width != 200 {
		t.Errorf("upscaled scale = %+v, %v", us, err)
	}
	fw, err := p.ResizeDict("img.png", map[string]any{"width": 50, "mode": "fit"})
	if err != nil || fw.Width != 50 || fw.Height != 25 {
		t.Errorf("fit width-only = %+v, %v", fw, err)
	}
}

// TestPipelineRuntimeGaps drives run/decode/cached/srcset runtime branches.
func TestPipelineRuntimeGaps(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)
	if err := os.WriteFile(filepath.Join(src, "junk.jpg"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Missing source through every helper front door.
	if _, err := p.ResizeDict("absent.png", map[string]any{"width": 10}); err == nil {
		t.Error("resize missing source must error")
	}
	if _, err := p.SrcSetDict("absent.png", map[string]any{"widths": []any{10}}); err == nil {
		t.Error("srcset missing source must error")
	}
	// Junk decodes fail descriptively.
	if _, err := p.ResizeDict("junk.jpg", map[string]any{"width": 10}); err == nil ||
		!strings.Contains(err.Error(), "not a supported image") {
		t.Errorf("junk decode err = %v", err)
	}
	// srcset: all widths invalid; per-width resize failure; skipped defaultWidth.
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{-1, 0}}); err == nil {
		t.Error("all-invalid widths must error")
	}
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{10}, "mode": "stretch"}); err == nil {
		t.Error("bad srcset mode must fail at width")
	}
	set, err := p.SrcSetDict("img.png", map[string]any{"widths": []any{20, 40}, "defaultWidth": 2000})
	if err != nil || set.Default.Width != 40 {
		t.Errorf("skipped defaultWidth should fall back to largest: %+v, %v", set.Default, err)
	}

	// cached(): output removed between builds → copy-through from cache.
	res, err := p.ResizeDict("img.png", map[string]any{"width": 30})
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(p.cfg.OutputDir, res.StaticPath)
	if err := os.Remove(outPath); err != nil {
		t.Fatal(err)
	}
	res2, err := p.ResizeDict("img.png", map[string]any{"width": 30})
	if err != nil || res2.URL != res.URL {
		t.Fatalf("copy-through failed: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Error("cached variant must be republished into the output")
	}

	// copyFile: destination is an existing directory → Create fails.
	if err := copyFile(filepath.Join(src, "img.png"), t.TempDir()); err == nil {
		t.Error("copyFile onto a directory must error")
	}
	// decodeConfigAt: missing file.
	if _, _, err := decodeConfigAt(filepath.Join(t.TempDir(), "none.png")); err == nil {
		t.Error("decodeConfigAt missing must error")
	}
	// resolver: first root has a DIRECTORY named like the file; second root wins.
	root2 := t.TempDir()
	writePNG(t, filepath.Join(root2, "twin.png"), 8, 8, false)
	if err := os.MkdirAll(filepath.Join(src, "twin.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	multi := New(Config{SourceDirs: []string{src, root2}, OutputDir: t.TempDir(), CacheDir: t.TempDir()})
	if _, err := multi.Info("twin.png"); err != nil {
		t.Errorf("second-root resolution failed: %v", err)
	}
	// withinRoot: existing root, missing path → EvalSymlinks error.
	if _, err := withinRoot(src, filepath.Join(src, "ghost.png")); err == nil {
		t.Error("withinRoot missing path must error")
	}
	// New() honours explicit URLPrefix.
	pref := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir(), URLPrefix: "imgs"})
	r3, err := pref.ResizeDict("img.png", map[string]any{"width": 12})
	if err != nil || !strings.HasPrefix(r3.URL, "/imgs/") {
		t.Errorf("custom URLPrefix = %+v, %v", r3, err)
	}
}

// TestNewDefaultsAndDotDot covers New's zero-value defaults and the exact ".."
// rejection plus a file-as-root fallthrough in resolve.
func TestNewDefaultsAndDotDot(t *testing.T) {
	d := New(Config{})
	if d.cfg.URLPrefix != "processed_images" || d.cfg.CacheDir != ".ssg-cache/images" ||
		d.cfg.JPEGQuality != 82 || d.cfg.WebPQuality != 82 ||
		d.cfg.MaxSourcePixels != 80_000_000 || d.cfg.MaxOutputPixels != 40_000_000 ||
		d.cfg.MaxDimension != 20_000 || d.cfg.MaxVariants != 20 {
		t.Errorf("defaults = %+v", d.cfg)
	}
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 8, 8, false)
	if _, err := p.resolve(".."); err == nil {
		t.Error("bare .. must be rejected")
	}
	// A FILE listed as a source root: candidates below it fail Stat → next root.
	fileRoot := filepath.Join(t.TempDir(), "rootfile")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mixed := New(Config{SourceDirs: []string{fileRoot, src}, OutputDir: t.TempDir(), CacheDir: t.TempDir()})
	if _, err := mixed.Info("img.png"); err != nil {
		t.Errorf("file-root fallthrough failed: %v", err)
	}
	// Empty-string root entries are skipped.
	sparse := New(Config{SourceDirs: []string{"", src}, OutputDir: t.TempDir(), CacheDir: t.TempDir()})
	if _, err := sparse.Info("img.png"); err != nil {
		t.Errorf("empty-root skip failed: %v", err)
	}
}

// TestUnreadableSourcePaths covers open-permission guards (skipped as root).
func TestUnreadableSourcePaths(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	p, src := testEnv(t)
	locked := filepath.Join(src, "locked.png")
	writePNG(t, locked, 8, 8, false)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Info("locked.png"); err == nil {
		t.Error("Info on unreadable file must error")
	}
	if _, err := p.ResizeDict("locked.png", map[string]any{"width": 4}); err == nil {
		t.Error("Resize on unreadable file must error (cacheKey/open)")
	}
}

// TestFillOneSidedClamp covers the fill branch where only one dimension exceeds
// the source (clamp scales by the tighter side).
func TestFillOneSidedClamp(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)
	res, err := p.ResizeDict("img.png", map[string]any{"width": 200, "height": 20, "mode": "fill"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 100 || res.Height != 10 {
		t.Errorf("one-sided clamp = %dx%d, want 100x10", res.Width, res.Height)
	}
	// Height-driven clamp direction as well.
	res2, err := p.ResizeDict("img.png", map[string]any{"width": 20, "height": 200, "mode": "fill"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Height != 50 || res2.Width != 5 {
		t.Errorf("height clamp = %dx%d, want 5x50", res2.Width, res2.Height)
	}
}

// TestLastGuards: encode-only pipeline, junk GIF probe, scalar widths type,
// and CreateTemp failure on a read-only cache dir.
func TestLastGuards(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 20, 10, false)

	// Pipeline consisting of a single encode op (applyOp passthrough).
	res, err := p.ProcessList("img.png", []any{map[string]any{"op": "encode", "format": "jpeg", "quality": 60}})
	if err != nil || res.Format != "jpeg" || res.Width != 20 {
		t.Errorf("encode-only pipeline = %+v, %v", res, err)
	}
	// isAnimatedGIF on junk input.
	if isAnimatedGIF(bytes.NewReader([]byte("junk"))) {
		t.Error("junk must not report animated")
	}
	// widths as a scalar (neither []any nor []int).
	if _, err := p.SrcSetDict("img.png", map[string]any{"widths": 480}); err == nil {
		t.Error("scalar widths must error")
	}
	// Read-only cache dir → CreateTemp fails inside publish.
	if os.Getuid() != 0 {
		roCache := filepath.Join(t.TempDir(), "ro")
		if err := os.MkdirAll(roCache, 0o555); err != nil {
			t.Fatal(err)
		}
		pro := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: roCache})
		if _, err := pro.ResizeDict("img.png", map[string]any{"width": 5}); err == nil {
			t.Error("read-only cache dir must error at CreateTemp")
		}
	}
}

// TestPublishRenameBlocked: the deterministic cache name is occupied by a
// DIRECTORY, so the atomic rename fails and surfaces as an error.
func TestPublishRenameBlocked(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 20, 10, false)
	res, err := p.ResizeDict("img.png", map[string]any{"width": 10})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(res.StaticPath)
	cachePath := filepath.Join(p.cfg.CacheDir, name)
	outPath := filepath.Join(p.cfg.OutputDir, res.StaticPath)
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(outPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil { // a dir squatting the name
		t.Fatal(err)
	}
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 10}); err == nil {
		t.Error("rename onto a directory must error")
	}
}
