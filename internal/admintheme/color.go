// Package admintheme computes the admin panels' look from `admin_theme:` in
// .ssg.yaml (GO-152): two brand colours become every colour token the panels
// use, in a light and a dark scheme, with WCAG 2.2 contrast guaranteed by
// construction; fonts and a logo are validated; the result is written as
// /core/theme.css and as the module registry's `theme` field.
//
// The worker serves what this package computes and does no colour maths.
package admintheme

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// RGB is an sRGB colour with 8 bits per channel — what a hex value holds, and
// the only form a colour is ever checked or emitted in, so a check is about
// the exact colour the browser paints.
type RGB struct{ R, G, B uint8 }

// ParseHex reads #rgb or #rrggbb (either case). Anything else is refused,
// including #rgba / #rrggbbaa: a translucent colour has no contrast ratio of
// its own — it depends on whatever lies beneath it.
func ParseHex(s string) (RGB, error) {
	if !strings.HasPrefix(s, "#") {
		return RGB{}, fmt.Errorf("%q is not a hex colour like #1b3a4b", s)
	}
	h := s[1:]
	switch len(h) {
	case 3:
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	case 6:
	case 4, 8:
		return RGB{}, fmt.Errorf("%q has an alpha channel; give an opaque colour (#rgb or #rrggbb) — a translucent colour has no contrast ratio", s)
	default:
		return RGB{}, fmt.Errorf("%q is not a hex colour like #1b3a4b", s)
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return RGB{}, fmt.Errorf("%q is not a hex colour like #1b3a4b", s)
	}
	return RGB{b[0], b[1], b[2]}, nil
}

// MustHex is ParseHex for the package's own constants.
func MustHex(s string) RGB {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// Hex renders the colour as lowercase #rrggbb.
func (c RGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// linear converts one 8-bit sRGB channel to linear light.
func linear(v uint8) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// Luminance is WCAG's relative luminance.
func (c RGB) Luminance() float64 {
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

// Contrast is the WCAG 2.2 contrast ratio of two colours, 1 to 21.
func Contrast(a, b RGB) float64 {
	x, y := a.Luminance(), b.Luminance()
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// floor2 rounds a ratio down to two decimals, so 4.499 is never shown as a
// passing 4.50.
func floor2(r float64) float64 { return math.Floor(r*100) / 100 }
