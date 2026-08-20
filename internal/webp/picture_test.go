package webp

// Offering AVIF without breaking anything (#178).
//
// The rule these tests exist for: the <img> is never modified. AVIF is added
// as a <source> in front of it, so a browser that understands neither AVIF nor
// WebP sees exactly the tag it always saw.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pageWith writes an HTML file and the AVIF files it should find beside it.
func pageWith(t *testing.T, html string, avifs ...string) (root, page string) {
	t.Helper()
	root = t.TempDir()
	page = filepath.Join(root, "index.html")
	if err := os.WriteFile(page, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, a := range avifs {
		p := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(a, "/")))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("avif"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, page
}

// TestAVIFIsOfferedBeforeWebP, with the img left exactly as it was.
func TestAVIFIsOfferedBeforeWebP(t *testing.T) {
	img := `<img src="/img/hero.webp" alt="Hero" loading="lazy">`
	root, page := pageWith(t, "<body>"+img+"</body>", "/img/hero.avif")

	out := wrapImagesInPicture("<body>"+img+"</body>", root, page)

	if !strings.Contains(out, "<picture>") {
		t.Fatalf("no <picture>: %s", out)
	}
	if !strings.Contains(out, `<source type="image/avif" src="/img/hero.avif">`) {
		t.Errorf("the AVIF source is missing or wrong: %s", out)
	}
	// The img is untouched, attributes and all — that is the fallback.
	if !strings.Contains(out, img) {
		t.Errorf("the <img> must survive verbatim: %s", out)
	}
	// And the source comes first, or the browser never considers it.
	if strings.Index(out, "image/avif") > strings.Index(out, "<img") {
		t.Errorf("the AVIF source must precede the img: %s", out)
	}
}

// TestNoAVIFOnDiskLeavesTheTagAlone: promising a file that is not there would
// be a 404 on every page view.
func TestNoAVIFOnDiskLeavesTheTagAlone(t *testing.T) {
	html := `<body><img src="/img/hero.webp"></body>`
	root, page := pageWith(t, html) // no avif written

	if out := wrapImagesInPicture(html, root, page); out != html {
		t.Errorf("without the file the tag must be untouched: %s", out)
	}
}

// TestSrcsetIsCarriedAcross: the AVIF pass made the same widths, so the
// responsive list applies with the extension swapped — otherwise a <picture>
// would offer one AVIF size against a full WebP set.
func TestSrcsetIsCarriedAcross(t *testing.T) {
	img := `<img src="/i/a.webp" srcset="/i/a-480.webp 480w, /i/a-960.webp 960w" sizes="100vw">`
	root, page := pageWith(t, img, "/i/a.avif")

	out := wrapImagesInPicture(img, root, page)
	if !strings.Contains(out, `srcset="/i/a-480.avif 480w, /i/a-960.avif 960w"`) {
		t.Errorf("the srcset must be carried across: %s", out)
	}
	if !strings.Contains(out, `sizes="100vw"`) {
		t.Errorf("sizes must be carried too, or the widths mean nothing: %s", out)
	}
	// The img keeps its own webp srcset.
	if !strings.Contains(out, `/i/a-480.webp 480w`) {
		t.Errorf("the img's own srcset must survive: %s", out)
	}
}

// TestIdempotent: a rebuild over an existing output tree must not nest a
// <picture> inside a <picture>.
func TestIdempotent(t *testing.T) {
	img := `<img src="/img/hero.webp">`
	root, page := pageWith(t, img, "/img/hero.avif")

	once := wrapImagesInPicture(img, root, page)
	twice := wrapImagesInPicture(once, root, page)
	if once != twice {
		t.Errorf("a second pass changed the output:\n%s\n%s", once, twice)
	}
	if strings.Count(twice, "<picture>") != 1 {
		t.Errorf("exactly one <picture>: %s", twice)
	}
}

// TestRemoteImagesAreNotOurs: a src on another host has no file here, and
// offering one would be a broken promise.
func TestRemoteImagesAreNotOurs(t *testing.T) {
	html := `<img src="https://cdn.example.com/a.webp">`
	root, page := pageWith(t, html, "/a.avif")
	if out := wrapImagesInPicture(html, root, page); out != html {
		t.Errorf("a remote image must be left alone: %s", out)
	}
}

// TestPageRelativeSrcResolves, because both forms occur in output.
func TestPageRelativeSrcResolves(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "post")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "index.html")
	if err := os.WriteFile(filepath.Join(dir, "photo.avif"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	html := `<img src="photo.webp">`
	if out := wrapImagesInPicture(html, root, page); !strings.Contains(out, "photo.avif") {
		t.Errorf("a page-relative src must resolve: %s", out)
	}
}

// TestNonWebPImagesAreIgnored: this pass only upgrades what the WebP pass made.
func TestNonWebPImagesAreIgnored(t *testing.T) {
	html := `<img src="/img/logo.svg"><img src="/img/photo.jpg">`
	root, page := pageWith(t, html, "/img/logo.avif", "/img/photo.avif")
	if out := wrapImagesInPicture(html, root, page); out != html {
		t.Errorf("only .webp sources are upgraded: %s", out)
	}
}

// TestVariantNamesAreRecognised, so a responsive derivative is never treated as
// a source image in its own right.
func TestIsVariantName(t *testing.T) {
	for _, p := range []string{"a-480.webp", "img/hero-1600.jpg", "x-1.png"} {
		if !isVariantName(p) {
			t.Errorf("%q is a generated variant", p)
		}
	}
	for _, p := range []string{"a.webp", "hero-image.jpg", "my-photo.png", "x-.png"} {
		if isVariantName(p) {
			t.Errorf("%q is not a variant", p)
		}
	}
}

// TestAVIFTargetPathKeepsCase: the trap webpTargetPath documents — a
// case-sensitive TrimSuffix would turn Photo.JPG into Photo.JPG.avif.
func TestAVIFTargetPathKeepsCase(t *testing.T) {
	cases := map[string]string{
		"a/Photo.JPG": "a/Photo.avif",
		"b/x.jpeg":    "b/x.avif",
		"c/y.PNG":     "c/y.avif",
		"d/z.webp":    "d/z.avif",
	}
	for in, want := range cases {
		if got := avifTargetPath(in); got != want {
			t.Errorf("avifTargetPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := avifVariantPath("a/x.avif", 480); got != "a/x-480.avif" {
		t.Errorf("avifVariantPath = %q", got)
	}
}
