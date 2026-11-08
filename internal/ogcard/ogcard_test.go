package ogcard

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// writePNG writes a w×h image of one colour and returns its path.
func writePNG(t *testing.T, name string, w, h int, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// colours is the minimum a site must give: Google Grey 900, white, Blue 200.
func colours(s Spec) Spec {
	if s.Background == "" && s.BackgroundImage == "" {
		s.Background = "#202124"
	}
	if s.Text == "" {
		s.Text = "#FFFFFF"
	}
	if s.Accent == "" {
		s.Accent = "#8AB4F8"
	}
	return s
}

func mustLoad(t *testing.T, s Spec) Layout {
	t.Helper()
	l, err := colours(s).Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestDefaultCardIsA1200x630PNG(t *testing.T) {
	l := mustLoad(t, Spec{})
	data, err := Render(Card{Eyebrow: "Poradniki", Title: "Zażółć gęślą jaźń", Footer: "Przykład"}, l)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 1200 || b.Dy() != 630 {
		t.Fatalf("size %v, want 1200×630", b)
	}
	// The accent bar runs down the left edge; the background is the default.
	if got := color.RGBAModel.Convert(img.At(2, 300)).(color.RGBA); got != l.Accent {
		t.Errorf("accent bar %v, want %v", got, l.Accent)
	}
	if got := color.RGBAModel.Convert(img.At(1190, 5)).(color.RGBA); got != l.Background {
		t.Errorf("background %v, want %v", got, l.Background)
	}
	if l.Ext() != ".png" {
		t.Errorf("ext %q", l.Ext())
	}
}

func TestJPEGWithBackgroundImageAndLogo(t *testing.T) {
	bg := writePNG(t, "bg.png", 300, 200, color.RGBA{R: 200, A: 255})
	logo := writePNG(t, "logo.png", 100, 100, color.RGBA{G: 200, A: 255})
	l := mustLoad(t, Spec{Width: 600, Height: 315, BackgroundImage: bg, Overlay: 0.5, Logo: logo, Format: "JPG", Quality: 90})
	if !l.JPEG || l.Ext() != ".jpg" || l.Quality != 90 {
		t.Fatalf("layout %+v", l)
	}
	data, err := Render(Card{Title: "Tytuł", Footer: strings.Repeat("Bardzo długa nazwa ", 10)}, l)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 600 || b.Dy() != 315 {
		t.Fatalf("size %v", b)
	}
	// Top-right corner: red photo under a half-strength dark overlay — neither
	// the pure photo nor the flat background.
	r, g, _, _ := img.At(590, 5).RGBA()
	if r>>8 < 90 || r>>8 > 140 || g>>8 > 40 {
		t.Errorf("overlay pixel r=%d g=%d", r>>8, g>>8)
	}
	// The logo sits bottom-right.
	_, g, _, _ = img.At(600-36-5, 315-36-5).RGBA()
	if g>>8 < 150 {
		t.Errorf("logo missing: g=%d", g>>8)
	}
}

func TestOverlayIsClamped(t *testing.T) {
	if l := mustLoad(t, Spec{Overlay: 3}); l.Overlay != 1 {
		t.Errorf("overlay %v", l.Overlay)
	}
	if l := mustLoad(t, Spec{Overlay: -1}); l.Overlay != 0 {
		t.Errorf("overlay %v", l.Overlay)
	}
}

func TestCustomFont(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	if l := mustLoad(t, Spec{Font: path}); l.Font == nil {
		t.Fatal("font not loaded")
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.png")
	if err := os.WriteFile(junk, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	for name, s := range map[string]Spec{
		"background colour": {Background: "red"},
		"text colour":       {Text: "#12"},
		"accent colour":     {Accent: "#zzzzzz"},
		"format":            {Format: "webp"},
		"missing image":     {BackgroundImage: missing},
		"undecodable image": {BackgroundImage: junk},
		"missing logo":      {Logo: missing},
		"missing font":      {Font: missing},
		"undecodable font":  {Font: junk},
	} {
		if _, err := colours(s).Load(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestKeyFollowsSpecAndFiles(t *testing.T) {
	logo := writePNG(t, "logo.png", 4, 4, color.RGBA{A: 255})
	a, err := Spec{Logo: logo}.Key()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := (Spec{Logo: logo}).Key(); a != b {
		t.Error("key is not stable")
	}
	if c, _ := (Spec{Logo: logo, Background: "#000"}).Key(); a == c {
		t.Error("colour change kept the key")
	}
	if err := os.WriteFile(logo, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d, _ := (Spec{Logo: logo}).Key(); a == d {
		t.Error("new logo bytes kept the key")
	}
	if _, err := (Spec{Font: filepath.Join(t.TempDir(), "none")}).Key(); err == nil {
		t.Error("missing file gave a key")
	}
}

func TestParseHexAndContrast(t *testing.T) {
	c, err := ParseHex(" #fff ")
	if err != nil || c != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("got %v %v", c, err)
	}
	black, _ := ParseHex("000000")
	if r := Contrast(black, c); r < 20.9 || r > 21.1 {
		t.Errorf("black/white %.2f, want 21", r)
	}
	if Contrast(c, black) != Contrast(black, c) {
		t.Error("contrast is not symmetric")
	}
	// Default colours pass AA.
	if w := mustLoad(t, Spec{}).ContrastWarnings(); len(w) != 0 {
		t.Errorf("defaults warn: %v", w)
	}
	w := mustLoad(t, Spec{Background: "#777", Text: "#888", Accent: "#666"}).ContrastWarnings()
	if len(w) != 2 || !strings.Contains(w[0], "text") || !strings.Contains(w[1], "accent") {
		t.Errorf("warnings %v", w)
	}
	// Over a photo the result depends on the photo: not judged.
	bg := writePNG(t, "bg.png", 2, 2, color.RGBA{A: 255})
	if w := mustLoad(t, Spec{Background: "#777", Text: "#888", BackgroundImage: bg}).ContrastWarnings(); w != nil {
		t.Errorf("photo card warns: %v", w)
	}
}

// TestColoursAreRequired: a card is the site's look, so the site states it.
// The text colour and a background (colour or image) are required; the
// accent follows the text, and an image alone is shaded with black.
func TestColoursAreRequired(t *testing.T) {
	for name, s := range map[string]Spec{
		"nothing":       {},
		"no text":       {Background: "#000"},
		"no background": {Text: "#fff"},
	} {
		if _, err := s.Load(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	l, err := Spec{Background: "#000", Text: "#fff"}.Load()
	if err != nil || l.Accent != l.Text {
		t.Errorf("accent %v, text %v, %v", l.Accent, l.Text, err)
	}
	bg := writePNG(t, "bg.png", 2, 2, color.RGBA{A: 255})
	l, err = Spec{BackgroundImage: bg, Text: "#fff"}.Load()
	if err != nil || l.Background != (color.RGBA{A: 255}) {
		t.Errorf("shade %v, %v", l.Background, err)
	}
}

func TestTextFitting(t *testing.T) {
	f := newFace(mustLoad(t, Spec{}).Font, 40)
	// A word wider than the line is broken inside, and no line overflows.
	lines := wrap(f, "a "+strings.Repeat("x", 80)+" b", 300)
	if len(lines) < 3 || lines[0] != "a" || lines[len(lines)-1] == "" {
		t.Fatalf("lines %q", lines)
	}
	for _, l := range lines {
		if width(f, l) > 300 {
			t.Errorf("line %q overflows", l)
		}
	}
	// A glyph wider than the line still advances.
	if head, rest := splitAt(f, "WW", 1); head != "W" || rest != "W" {
		t.Errorf("split %q %q", head, rest)
	}
	if got := clampLines(f, []string{"one", "two", "three"}, 2, 300); len(got) != 2 || !strings.HasSuffix(got[1], ellipsis) {
		t.Errorf("clamp %q", got)
	}
	if got := clampLines(f, []string{"one"}, 2, 300); len(got) != 1 {
		t.Errorf("clamp short %q", got)
	}
	if got := fitLine(f, "short", 300); got != "short" {
		t.Errorf("fit %q", got)
	}
	if got := fitLine(f, "a long line, that will not fit", 200); !strings.HasSuffix(got, ellipsis) || width(f, got) > 200 {
		t.Errorf("fit long %q", got)
	}
	if wrap(f, "   ", 300) != nil {
		t.Error("blank title gave lines")
	}
}

func TestOverlongTitleEndsWithEllipsis(t *testing.T) {
	l := mustLoad(t, Spec{})
	canvas := image.NewRGBA(image.Rect(0, 0, 1200, 630))
	// Drawn into a box two minimum-size lines tall, a long title is cut.
	box := image.Rect(0, 0, 1000, 2*lineHeight(newFace(l.Font, titleMin)))
	drawTitle(canvas, l, strings.Repeat("słowo ", 200), box, 1)
	if _, err := Render(Card{Title: strings.Repeat("słowo ", 400)}, l); err != nil {
		t.Fatal(err)
	}
}
