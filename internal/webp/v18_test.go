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
