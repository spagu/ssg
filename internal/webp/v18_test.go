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
