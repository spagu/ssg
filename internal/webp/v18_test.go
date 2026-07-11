package webp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVariantPath(t *testing.T) {
	if got := variantPath("img/foo.webp", 480); got != "img/foo-480.webp" {
		t.Errorf("variantPath = %q, want img/foo-480.webp", got)
	}
}

// TestEmitSrcset covers ASSET-004: <img> tags gain srcset/sizes when variant
// files exist, and are left alone otherwise.
func TestEmitSrcset(t *testing.T) {
	dir := t.TempDir()
	// Create variant files that "exist" on disk.
	for _, w := range []int{480, 960} {
		if err := os.WriteFile(filepath.Join(dir, "hero-"+itoa(w)+".webp"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	html := `<html><body>` +
		`<img src="/hero.webp" alt="a">` +
		`<img src="/nover.webp" alt="b">` +
		`<img src="/hero.webp" srcset="existing">` +
		`</body></html>`
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EmitSrcset(dir, []int{480, 960}, "100vw"); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	s := string(out)

	if !strings.Contains(s, `srcset="/hero-480.webp 480w, /hero-960.webp 960w" sizes="100vw"`) {
		t.Errorf("expected srcset for hero image, got:\n%s", s)
	}
	// Image without variants keeps no srcset.
	if strings.Contains(s, `/nover.webp" srcset`) {
		t.Errorf("did not expect srcset for image without variants")
	}
	// Existing srcset must be preserved untouched (only one occurrence of "existing").
	if strings.Count(s, "existing") != 1 {
		t.Errorf("existing srcset should be preserved once, got:\n%s", s)
	}
}

// TestEmitSrcsetNoSizes is a no-op when no sizes are configured.
func TestEmitSrcsetNoSizes(t *testing.T) {
	if err := EmitSrcset(t.TempDir(), nil, ""); err != nil {
		t.Errorf("EmitSrcset with no sizes should be a no-op, got %v", err)
	}
}

// TestImageWidth reads dimensions without a full decode.
func TestImageWidth(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 120, 40))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "x.png")
	if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	w, ok := imageWidth(p)
	if !ok || w != 120 {
		t.Errorf("imageWidth = (%d,%v), want (120,true)", w, ok)
	}
	if _, ok := imageWidth(filepath.Join(dir, "missing.png")); ok {
		t.Errorf("imageWidth on missing file should be false")
	}
}

// TestGenerateResponsiveVariants exercises the cwebp resize path (ASSET-004).
func TestGenerateResponsiveVariants(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not installed")
	}
	dir := t.TempDir()
	// A 1000px-wide source image.
	img := image.NewRGBA(image.Rect(0, 0, 1000, 200))
	src := filepath.Join(dir, "hero.png")
	f, _ := os.Create(src)
	_ = png.Encode(f, img)
	_ = f.Close()

	webpPath := filepath.Join(dir, "hero.webp")
	opts := ConvertOptions{Quality: 60, Quiet: true, Sizes: []int{480, 960, 2000}}
	generateResponsiveVariants(src, webpPath, opts)

	for _, w := range []int{480, 960} {
		if _, err := os.Stat(variantPath(webpPath, w)); err != nil {
			t.Errorf("expected variant %dw: %v", w, err)
		}
	}
	// 2000 >= original width 1000 → no upscaled variant.
	if _, err := os.Stat(variantPath(webpPath, 2000)); err == nil {
		t.Errorf("did not expect upscaled 2000w variant")
	}
}

// TestEmitSrcsetRelativeSrc covers rewriteImgTag resolving a same-directory src.
func TestEmitSrcsetRelativeSrc(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "post")
	_ = os.MkdirAll(sub, 0755)
	_ = os.WriteFile(filepath.Join(sub, "pic-480.webp"), []byte("x"), 0644)
	html := `<img src="pic.webp" alt="rel">`
	_ = os.WriteFile(filepath.Join(sub, "index.html"), []byte(html), 0644)

	if err := EmitSrcset(dir, []int{480}, ""); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(sub, "index.html"))
	if !strings.Contains(string(out), `srcset="pic-480.webp 480w" sizes="100vw"`) {
		t.Errorf("relative srcset not emitted: %s", out)
	}
}

// itoa avoids importing strconv just for the test helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestUpdateReferencesWebp(t *testing.T) {
	dir := t.TempDir()
	writeWebpFixtures(t, dir, "a.webp", "b.webp", "c.webp", "d.webp", "e.webp")
	html := `<img src="a.jpg"><img src='b.png'><link href="c.jpeg"><style>background:url(d.png)</style><img srcset="e.jpg 1x">`
	p := filepath.Join(dir, "index.html")
	if err := os.WriteFile(p, []byte(html), 0644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateReferences(dir); err != nil {
		t.Fatalf("UpdateReferences: %v", err)
	}
	out, _ := os.ReadFile(p)
	s := string(out)
	for _, bad := range []string{".jpg", ".png", ".jpeg"} {
		if strings.Contains(s, bad) {
			t.Errorf("reference %q not rewritten to .webp: %s", bad, s)
		}
	}
}

func TestConvertImageResizedError(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not installed")
	}
	// A non-existent source makes cwebp fail → exercises the error branch.
	if err := convertImageResized(filepath.Join(t.TempDir(), "nope.png"), filepath.Join(t.TempDir(), "o.webp"), 60, 100); err == nil {
		t.Error("expected error for missing source image")
	}
}

// TestImageWidthNonImage covers the DecodeConfig error branch (existing but not an image).
func TestImageWidthNonImage(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notimage.png")
	if err := os.WriteFile(p, []byte("this is not a png"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := imageWidth(p); ok {
		t.Error("imageWidth on a non-image should return ok=false")
	}
}

// TestGenerateResponsiveVariantsBadSource covers the early return when the source
// is not a decodable image (imageWidth !ok).
func TestGenerateResponsiveVariantsBadSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(src, []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}
	// Must not panic and must not create variants.
	generateResponsiveVariants(src, filepath.Join(dir, "bad.webp"),
		ConvertOptions{Quality: 60, Quiet: true, Sizes: []int{480}})
	if _, err := os.Stat(variantPath(filepath.Join(dir, "bad.webp"), 480)); err == nil {
		t.Error("no variant should be produced from an undecodable source")
	}
}

// TestEmitSrcsetDefaultSizesAttr covers the sizesAttr=="" default branch (→ 100vw).
func TestEmitSrcsetDefaultSizesAttr(t *testing.T) {
	dir := t.TempDir()
	for _, w := range []int{480, 960} {
		if err := os.WriteFile(filepath.Join(dir, "pic-"+itoa(w)+".webp"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img src="/pic.webp" alt="a">`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EmitSrcset(dir, []int{480, 960}, ""); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if !strings.Contains(string(out), `sizes="100vw"`) {
		t.Errorf("expected default sizes=100vw, got:\n%s", out)
	}
}

// TestConvertDirectoryWithVariants covers the ConvertDirectory branch that emits
// responsive variants (opts.Sizes set) for a real image.
func TestConvertDirectoryWithVariants(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not installed")
	}
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 400, 100))
	f, _ := os.Create(filepath.Join(dir, "wide.png"))
	_ = png.Encode(f, img)
	_ = f.Close()

	converted, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 60, Quiet: true, Sizes: []int{200}})
	if err != nil {
		t.Fatalf("ConvertDirectory: %v", err)
	}
	if converted != 1 {
		t.Errorf("converted = %d, want 1", converted)
	}
	if _, err := os.Stat(filepath.Join(dir, "wide-200.webp")); err != nil {
		t.Errorf("expected 200w variant: %v", err)
	}
}

// TestEmitSrcsetNoChange covers the no-op return path: an <img> whose variants do
// not exist and a non-webp <img> (imgSrcRe no match) leave the file untouched.
func TestEmitSrcsetNoChange(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "index.html")
	orig := `<img src="/novariant.webp" alt="a"><img src="/plain.png" alt="b">`
	if err := os.WriteFile(htmlPath, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EmitSrcset(dir, []int{480}, "100vw"); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if string(out) != orig {
		t.Errorf("file should be unchanged, got:\n%s", out)
	}
}

// buildWebpHeader assembles a minimal RIFF/WEBP container header for webpWidth
// tests, zero-padded to total bytes; the chunk-size field is unused by the parser.
func buildWebpHeader(fourcc string, payload []byte, total int) []byte {
	b := append([]byte("RIFF\x00\x00\x00\x00WEBP"), fourcc...)
	b = append(b, 0, 0, 0, 0)
	b = append(b, payload...)
	for len(b) < total {
		b = append(b, 0)
	}
	return b
}

// vp8lPayload encodes a VP8L (lossless) bitstream header for a given size.
func vp8lPayload(width, height int) []byte {
	bits := uint32(width-1) | uint32(height-1)<<14
	return []byte{0x2F, byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24)}
}

// TestWebpWidth covers GO-032: pixel width is read from all three WebP container
// layouts without a decoder dependency, and malformed headers are rejected.
func TestWebpWidth(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		want   int
		wantOK bool
	}{
		{"VP8L lossless", buildWebpHeader("VP8L", vp8lPayload(2000, 100), 30), 2000, true},
		{"VP8 lossy", buildWebpHeader("VP8 ", []byte{0, 0, 0, 0x9D, 0x01, 0x2A, 0x20, 0x03, 0x58, 0x02}, 30), 800, true},
		{"VP8X extended", buildWebpHeader("VP8X", []byte{0, 0, 0, 0, 0xD1, 0x04, 0x00, 19, 0, 0}, 30), 1234, true},
		{"not RIFF", append([]byte("JUNK"), make([]byte, 26)...), 0, false},
		{"RIFF but not WEBP", append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 18)...), 0, false},
		{"unknown chunk", buildWebpHeader("ABCD", nil, 30), 0, false},
		{"VP8 bad sync code", buildWebpHeader("VP8 ", []byte{0, 0, 0, 0xFF, 0x01, 0x2A, 0x20, 0x03}, 30), 0, false},
		{"VP8L bad signature", buildWebpHeader("VP8L", []byte{0x00, 1, 2, 3, 4}, 30), 0, false},
		{"truncated header", []byte("RIFF"), 0, false},
		{"VP8X too short", buildWebpHeader("VP8X", []byte{0, 0, 0, 0, 0xD1}, 25), 0, false},
		{"VP8 too short", buildWebpHeader("VP8 ", []byte{0, 0, 0, 0x9D, 0x01, 0x2A, 0x20}, 27), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := webpWidth(bytes.NewReader(tt.data))
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("webpWidth() = (%d,%v), want (%d,%v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestImageWidthWebpFallback covers imageWidth falling back to the WebP header
// parse for files the stdlib cannot decode (GO-032).
func TestImageWidthWebpFallback(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hero.webp")
	if err := os.WriteFile(p, buildWebpHeader("VP8L", vp8lPayload(1600, 10), 30), 0644); err != nil {
		t.Fatal(err)
	}
	if w, ok := imageWidth(p); !ok || w != 1600 {
		t.Errorf("imageWidth(webp) = (%d,%v), want (1600,true)", w, ok)
	}
}

// TestEmitSrcsetIncludesOriginalWidth covers GO-032: the srcset must list the
// full-size original with its real pixel width — with w descriptors browsers
// ignore src, so otherwise desktops would upscale the largest variant.
func TestEmitSrcsetIncludesOriginalWidth(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pic-480.webp"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// The "original": image.DecodeConfig sniffs content, so a tiny generated
	// PNG stored under the .webp name yields the width without a webp decoder.
	img := image.NewRGBA(image.Rect(0, 0, 1600, 10))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pic.webp"), buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img src="/pic.webp" alt="a">`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EmitSrcset(dir, []int{480}, "100vw"); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if !strings.Contains(string(out), `srcset="/pic-480.webp 480w, /pic.webp 1600w"`) {
		t.Errorf("srcset should include full-size original with width (GO-032), got:\n%s", out)
	}
}

// TestEmitSrcsetSelfClosing covers GO-038: injection into <img ... /> must keep
// the tag valid instead of leaving a stray slash mid-tag.
func TestEmitSrcsetSelfClosing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hero-480.webp"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img src="/hero.webp" alt="a" />`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EmitSrcset(dir, []int{480}, "100vw"); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	s := string(out)
	if !strings.Contains(s, `srcset="/hero-480.webp 480w" sizes="100vw" />`) {
		t.Errorf("srcset not injected before /> (GO-038), got: %s", s)
	}
	if strings.Contains(s, `/ srcset=`) {
		t.Errorf("self-closing tag corrupted (GO-038): %s", s)
	}
}

// TestEmitSrcsetDataSrcUntouched covers GO-038: lazy-load data-src attributes
// must not be mistaken for src, so no srcset is emitted for them.
func TestEmitSrcsetDataSrcUntouched(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lazy-480.webp"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(dir, "index.html")
	orig := `<img data-src="/lazy.webp" alt="lazy">`
	if err := os.WriteFile(htmlPath, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EmitSrcset(dir, []int{480}, "100vw"); err != nil {
		t.Fatalf("EmitSrcset: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if string(out) != orig {
		t.Errorf("data-src tag must stay untouched (GO-038), got:\n%s", out)
	}
}

// TestNewExistsCacheMemoizes covers PERF-011: each path is stat'ed once — later
// filesystem changes are invisible for the duration of the walk.
func TestNewExistsCacheMemoizes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "v.webp")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	exists := newExistsCache()
	if !exists(p) {
		t.Fatal("expected true for existing file")
	}
	_ = os.Remove(p)
	if !exists(p) {
		t.Error("positive result should be memoized, not re-stat'ed (PERF-011)")
	}
	missing := filepath.Join(dir, "gone.webp")
	if exists(missing) {
		t.Fatal("expected false for missing file")
	}
	if err := os.WriteFile(missing, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if exists(missing) {
		t.Error("negative result should be memoized too (PERF-011)")
	}
}

// TestNewWidthCacheMemoizes covers GO-032/PERF-011: the original's width is
// decoded once and served from the cache afterwards.
func TestNewWidthCacheMemoizes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "w.webp")
	if err := os.WriteFile(p, buildWebpHeader("VP8L", vp8lPayload(320, 10), 30), 0644); err != nil {
		t.Fatal(err)
	}
	width := newWidthCache()
	if w, ok := width(p); !ok || w != 320 {
		t.Fatalf("width = (%d,%v), want (320,true)", w, ok)
	}
	_ = os.Remove(p)
	if w, ok := width(p); !ok || w != 320 {
		t.Error("width should be memoized, not re-decoded (PERF-011)")
	}
	if _, ok := width(filepath.Join(dir, "missing.webp")); ok {
		t.Error("missing file must report no width")
	}
}
