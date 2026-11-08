// Package ogcard draws the social preview card of a page — the 1200×630
// image Facebook, LinkedIn, X, Slack and messengers show beside a shared link
// — at build time, in pure Go: no browser, no external tool, no service.
//
// The platforms read PNG and JPEG, not SVG, so the card is a raster image.
// The look is one layout per site (colours, font, optional background image
// and logo); each page contributes its words: title, a small line above it
// (category or section) and a footer (the site's name).
package ogcard

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // decode JPEG backgrounds and logos
	_ "image/png"  // decode PNG backgrounds and logos
	"math"
	"os"
	"strconv"
	"strings"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"

	"github.com/spagu/ssg/internal/cache"
)

// Defaults: the size every platform crops to without loss. The colours have
// none: a card is the site's own look, so the site has to say what it is.
const (
	DefaultWidth   = 1200
	DefaultHeight  = 630
	DefaultQuality = 85
	// shadeBehindImage is the overlay colour when only a background image is
	// given.
	shadeBehindImage = "#000000"
	// minContrast is WCAG 2.2 AA for normal-size text.
	minContrast = 4.5
)

// Spec is the layout as configured: colours as "#rrggbb", assets as file
// paths. Text and one of Background or BackgroundImage are required; Accent
// falls back to Text, the other fields to the defaults above.
type Spec struct {
	Width, Height   int
	Background      string
	Text            string
	Accent          string
	BackgroundImage string  // photo or texture, cropped to fill the card
	Overlay         float64 // 0..1: Background laid over the image so text stays legible
	Logo            string  // drawn bottom-right
	Font            string  // TTF/OTF; empty = Go Bold (Latin, Greek, Cyrillic)
	Format          string  // "png" (default) or "jpeg"
	Quality         int     // JPEG quality, default 85
}

// Layout is a Spec loaded: colours parsed, images decoded, font ready.
type Layout struct {
	Width, Height            int
	Background, Text, Accent color.RGBA
	BackgroundImage          image.Image
	Overlay                  float64
	Logo                     image.Image
	Font                     *opentype.Font
	JPEG                     bool
	Quality                  int
}

// Ext is the file extension the layout encodes to.
func (l Layout) Ext() string {
	if l.JPEG {
		return ".jpg"
	}
	return ".png"
}

// Load turns the spec into a layout, reading every file it names. A missing
// required value, a wrong path or a wrong colour is an error: a site that
// asked for its brand font should not quietly ship cards in another.
func (s Spec) Load() (Layout, error) {
	if strings.TrimSpace(s.Text) == "" {
		return Layout{}, fmt.Errorf("og_cards: set text_color, the colour of the title (e.g. \"#ffffff\")")
	}
	if strings.TrimSpace(s.Background) == "" && s.BackgroundImage == "" {
		return Layout{}, fmt.Errorf("og_cards: set background (a colour, e.g. \"#202124\") or background_image (a PNG/JPEG)")
	}
	l := Layout{Width: orInt(s.Width, DefaultWidth), Height: orInt(s.Height, DefaultHeight),
		Overlay: math.Max(0, math.Min(1, s.Overlay)), Quality: orInt(s.Quality, DefaultQuality)}
	var err error
	for _, c := range []struct {
		dst      *color.RGBA
		val, def string
	}{{&l.Background, s.Background, shadeBehindImage}, {&l.Text, s.Text, ""}, {&l.Accent, s.Accent, s.Text}} {
		if *c.dst, err = ParseHex(orStr(c.val, c.def)); err != nil {
			return Layout{}, err
		}
	}
	switch strings.ToLower(strings.TrimSpace(s.Format)) {
	case "", "png":
	case "jpeg", "jpg":
		l.JPEG = true
	default:
		return Layout{}, fmt.Errorf("og card format %q: use png or jpeg", s.Format)
	}
	if l.BackgroundImage, err = loadImage(s.BackgroundImage); err != nil {
		return Layout{}, err
	}
	if l.Logo, err = loadImage(s.Logo); err != nil {
		return Layout{}, err
	}
	l.Font, err = loadFont(s.Font)
	return l, err
}

// Key fingerprints everything that shapes a card besides its words: the
// spec and the bytes of the files it names, so replacing the logo or the
// font re-renders every card.
func (s Spec) Key() (string, error) {
	k := cache.NewKeyer("ogcard-v1", 16)
	k.WriteDelim(fmt.Sprintf("%+v", s))
	for _, path := range []string{s.BackgroundImage, s.Logo, s.Font} {
		if path == "" {
			continue
		}
		if err := k.WriteFileContents(path); err != nil {
			return "", fmt.Errorf("og card: %w", err)
		}
		k.WriteDelim("")
	}
	return k.Sum(), nil
}

// ContrastWarnings names the colour pairs below WCAG 2.2 AA. Over a
// background image the result depends on the photo, so only the flat card is
// judged.
func (l Layout) ContrastWarnings() []string {
	if l.BackgroundImage != nil {
		return nil
	}
	var out []string
	for _, c := range []struct {
		name string
		fg   color.RGBA
	}{{"text", l.Text}, {"accent", l.Accent}} {
		if r := Contrast(c.fg, l.Background); r < minContrast {
			out = append(out, fmt.Sprintf("og card %s colour has contrast %.1f:1 against the background (WCAG AA needs %.1f:1)", c.name, r, minContrast))
		}
	}
	return out
}

// ParseHex reads "#rgb" or "#rrggbb".
func ParseHex(s string) (color.RGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if len(h) != 6 || err != nil {
		return color.RGBA{}, fmt.Errorf("og card colour %q: expected #rrggbb", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil // #nosec G115 -- masked to a byte
}

// Contrast is the WCAG 2.2 contrast ratio of two opaque colours.
func Contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.RGBA) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

func loadImage(path string) (image.Image, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- a path from the site's own config
	if err != nil {
		return nil, fmt.Errorf("og card: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("og card image %s: %w", path, err)
	}
	return img, nil
}

func loadFont(path string) (*opentype.Font, error) {
	data := gobold.TTF
	if path != "" {
		var err error
		if data, err = os.ReadFile(path); err != nil { // #nosec G304 -- a path from the site's own config
			return nil, fmt.Errorf("og card: %w", err)
		}
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("og card font %s: %w", path, err)
	}
	return f, nil
}

func orInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func orStr(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}
