package webp

// The site-level AVIF pass (#178).
//
// The encoder is an optional binary, so the tests put a stand-in on PATH: what
// is under test is which files this pass decides to make and what it asks the
// encoder for, not libavif's output. The one thing a stand-in cannot prove —
// that avifenc accepts the input — is why the ordering rule below exists, and
// it has its own test.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEncoder puts a stand-in avifenc on PATH that records its arguments into
// the file it was asked to write.
func fakeEncoder(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nout=\"\"\nfor a in \"$@\"; do out=\"$a\"; done\nprintf 'ARGS:%s' \"$*\" > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(dir, "avifenc"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// noEncoder puts an empty directory on PATH so avifenc cannot be found.
func noEncoder(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// pngFixture writes a small real PNG, which the resizer must be able to decode.
func pngFixture(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, testPNG(t, w, h), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAVIFAvailableFollowsPATH, which is what decides between a warning and a
// conversion.
func TestAVIFAvailableFollowsPATH(t *testing.T) {
	noEncoder(t)
	if AVIFAvailable() {
		t.Error("no avifenc on PATH means not available")
	}
	fakeEncoder(t)
	if !AVIFAvailable() {
		t.Error("an avifenc on PATH means available")
	}
}

// TestMissingEncoderIsAnErrorTheCallerCanExplain: the build must be able to
// warn and carry on, so this reports rather than panics — and names the
// packages, because "not installed" without them sends the reader searching.
func TestMissingEncoderIsAnErrorTheCallerCanExplain(t *testing.T) {
	noEncoder(t)
	_, err := ConvertDirectoryAVIF(t.TempDir(), AVIFOptions{})
	if err == nil {
		t.Fatal("a missing encoder must be reported")
	}
	for _, want := range []string{"avifenc", "libavif"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error must mention %q: %v", want, err)
		}
	}
}

// TestConvertsOriginalsAndTheirWidths: one AVIF per image plus one per
// configured width, and never an upscale.
func TestConvertsOriginalsAndTheirWidths(t *testing.T) {
	fakeEncoder(t)
	root := t.TempDir()
	pngFixture(t, filepath.Join(root, "img", "hero.png"), 1200, 800)

	made, err := ConvertDirectoryAVIF(root, AVIFOptions{
		Sizes: []int{480, 960, 4000}, Quiet: true, Workers: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"img/hero.avif", "img/hero-480.avif", "img/hero-960.avif"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
			t.Errorf("missing %s", want)
		}
	}
	// 4000 is wider than the source: upscaling a photograph invents detail.
	if _, err := os.Stat(filepath.Join(root, "img", "hero-4000.avif")); err == nil {
		t.Error("a width above the original must be skipped, not upscaled")
	}
	if made != 3 {
		t.Errorf("made = %d, want 3", made)
	}
}

// TestQualityAndSpeedReachTheEncoder, which is the whole point of the settings.
func TestQualityAndSpeedReachTheEncoder(t *testing.T) {
	fakeEncoder(t)
	root := t.TempDir()
	pngFixture(t, filepath.Join(root, "a.png"), 40, 30)

	if _, err := ConvertDirectoryAVIF(root, AVIFOptions{Quality: 33, Speed: 8, Quiet: true, Workers: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "a.avif"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-q 33", "-s 8"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("the encoder was not given %q: %s", want, got)
		}
	}
	// Out-of-range values fall back rather than being passed through: avifenc
	// would reject them and every image would fail.
	root2 := t.TempDir()
	pngFixture(t, filepath.Join(root2, "b.png"), 40, 30)
	if _, err := ConvertDirectoryAVIF(root2, AVIFOptions{Quality: 999, Speed: -3, Quiet: true, Workers: 1}); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(root2, "b.avif"))
	if !strings.Contains(string(got), "-q 45") || !strings.Contains(string(got), "-s 6") {
		t.Errorf("out-of-range settings must fall back to the defaults: %s", got)
	}
}

// TestWebPIsNeverASource: avifenc cannot read it — the real encoder says
// "unrecognized file format" — which is why this pass runs before the WebP one.
// A stand-in encoder accepts anything, so this asserts the SELECTION rather
// than the encode, and that is the assertion that would have caught the bug.
func TestWebPIsNeverASource(t *testing.T) {
	fakeEncoder(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only.webp"), []byte("RIFF....WEBP"), 0o600); err != nil {
		t.Fatal(err)
	}
	made, err := ConvertDirectoryAVIF(root, AVIFOptions{Quiet: true, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if made != 0 {
		t.Errorf("made = %d — a .webp must never be used as a source", made)
	}
}

// TestGeneratedVariantsAreNotSourcesThemselves, or every rebuild would make
// derivatives of derivatives.
func TestGeneratedVariantsAreNotSources(t *testing.T) {
	fakeEncoder(t)
	root := t.TempDir()
	pngFixture(t, filepath.Join(root, "hero.png"), 100, 80)
	pngFixture(t, filepath.Join(root, "hero-480.png"), 100, 80)

	sources, err := avifSources(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sources {
		if strings.Contains(filepath.Base(s), "-480") {
			t.Errorf("a generated variant was treated as a source: %s", s)
		}
	}
	if len(sources) != 1 {
		t.Errorf("sources = %v, want the original alone", sources)
	}
}

// TestExistingFilesAreLeftAloneUnlessForced, so a rebuild does not re-encode a
// library that has not changed — at four times WebP's cost, that matters.
func TestExistingFilesAreLeftAloneUnlessForced(t *testing.T) {
	fakeEncoder(t)
	root := t.TempDir()
	pngFixture(t, filepath.Join(root, "a.png"), 40, 30)

	if _, err := ConvertDirectoryAVIF(root, AVIFOptions{Quiet: true, Workers: 1}); err != nil {
		t.Fatal(err)
	}
	marker := []byte("KEEP ME")
	if err := os.WriteFile(filepath.Join(root, "a.avif"), marker, 0o600); err != nil {
		t.Fatal(err)
	}

	made, err := ConvertDirectoryAVIF(root, AVIFOptions{Quiet: true, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if made != 0 {
		t.Errorf("made = %d, want 0 — nothing changed", made)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.avif")); string(got) != string(marker) {
		t.Error("an existing file was rewritten without --reconvert-images")
	}

	if _, err := ConvertDirectoryAVIF(root, AVIFOptions{Force: true, Quiet: true, Workers: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.avif")); string(got) == string(marker) {
		t.Error("Force must re-encode")
	}
}

// TestAFailingEncoderDoesNotStopTheBuild: one bad image must not cost a site
// its other four hundred.
func TestAFailingEncoderDoesNotStopTheBuild(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "avifenc"), []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	pngFixture(t, filepath.Join(root, "a.png"), 40, 30)
	made, err := ConvertDirectoryAVIF(root, AVIFOptions{Quiet: true, Workers: 1})
	if err != nil {
		t.Fatalf("a failing encoder is not a build failure: %v", err)
	}
	if made != 0 {
		t.Errorf("made = %d, want 0", made)
	}
}

// TestResizeToPNGProducesTheRequestedWidth, since the responsive variants are
// only as good as this step.
func TestResizeToPNGProducesTheRequestedWidth(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.png")
	pngFixture(t, src, 200, 100)
	dst := filepath.Join(root, "out.png")

	if err := resizeToPNG(src, dst, 50); err != nil {
		t.Fatal(err)
	}
	w, ok := imageWidth(dst)
	if !ok {
		t.Fatal("the result must be a readable image")
	}
	if w != 50 {
		t.Errorf("width = %d, want 50", w)
	}
	// A source that is not an image is an error, not a silent empty file.
	bad := filepath.Join(root, "bad.png")
	if err := os.WriteFile(bad, []byte("not a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resizeToPNG(bad, filepath.Join(root, "x.png"), 50); err == nil {
		t.Error("an unreadable source must be reported")
	}
}

// TestAVIFSummaryNamesWhatHappened.
func TestAVIFSummary(t *testing.T) {
	got := AVIFSummary(7)
	for _, want := range []string{"7", "AVIF", "picture"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary must mention %q: %s", want, got)
		}
	}
}

// testPNG builds a small real PNG. Written by hand rather than checked in: a
// test that states the bytes it is working with beats one that trusts a
// fixture nobody has opened.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAVariantFailureDoesNotLoseTheFullSizeOne: an encoder that manages the
// original and chokes on one width must still leave the site with an AVIF.
func TestAVariantFailureDoesNotLoseTheFullSize(t *testing.T) {
	dir := t.TempDir()
	// Succeeds unless the output name carries a width suffix.
	script := "#!/bin/sh\nout=\"\"\nfor a in \"$@\"; do out=\"$a\"; done\n" +
		"case \"$out\" in *-*.avif) exit 4 ;; esac\nprintf 'ok' > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(dir, "avifenc"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	pngFixture(t, filepath.Join(root, "hero.png"), 200, 100)
	made, err := ConvertDirectoryAVIF(root, AVIFOptions{Sizes: []int{50}, Quiet: true, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if made != 1 {
		t.Errorf("made = %d, want the full-size one", made)
	}
	if _, err := os.Stat(filepath.Join(root, "hero.avif")); err != nil {
		t.Errorf("the full-size AVIF must survive a variant failure: %v", err)
	}
}

// TestAnUnreadableSourceIsSkipped rather than failing the pass: one corrupt
// file in a media library must not cost the other four hundred.
func TestAnUnreadableSourceIsSkipped(t *testing.T) {
	fakeEncoder(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.png"), []byte("not a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	pngFixture(t, filepath.Join(root, "good.png"), 60, 40)

	made, err := ConvertDirectoryAVIF(root, AVIFOptions{Sizes: []int{30}, Quiet: true, Workers: 1})
	if err != nil {
		t.Fatalf("one bad file is not a build failure: %v", err)
	}
	// The good image got its full size and its variant; the broken one got the
	// full size only, because the width step needs to decode it.
	if made < 2 {
		t.Errorf("made = %d, want the good image and its variant at least", made)
	}
	if _, err := os.Stat(filepath.Join(root, "good-30.avif")); err != nil {
		t.Errorf("the readable image must still get its variant: %v", err)
	}
}

// TestWalkingAnUnreadableTreeIsNotAFailure.
func TestConvertOnAMissingDirectory(t *testing.T) {
	fakeEncoder(t)
	made, err := ConvertDirectoryAVIF(filepath.Join(t.TempDir(), "nope"), AVIFOptions{Quiet: true, Workers: 1})
	if err != nil {
		t.Errorf("a missing directory = %v", err)
	}
	if made != 0 {
		t.Errorf("made = %d", made)
	}
}
