package admintheme

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestParseHex(t *testing.T) {
	for in, want := range map[string]string{"#fff": "#ffffff", "#ABC": "#aabbcc", "#1B3a4b": "#1b3a4b", "#000000": "#000000"} {
		c, err := ParseHex(in)
		if err != nil || c.Hex() != want {
			t.Errorf("%s → %s %v", in, c.Hex(), err)
		}
	}
	for in, msg := range map[string]string{
		"1b3a4b": "not a hex colour", "#12": "not a hex colour", "#ggg": "not a hex colour", "#1234567": "not a hex colour",
		"#ffff": "alpha channel", "#1b3a4bff": "alpha channel", "red": "not a hex colour", "": "not a hex colour",
		"#12345g": "not a hex colour", "#fff;}": "not a hex colour", "#+12345": "not a hex colour",
	} {
		if _, err := ParseHex(in); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%q: %v", in, err)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("MustHex panics on a bad constant")
		}
	}()
	MustHex("nope")
}

func TestContrast(t *testing.T) {
	black, w := MustHex("#000"), MustHex("#fff")
	if r := Contrast(black, w); math.Abs(r-21) > 1e-9 || Contrast(w, black) != r || Contrast(w, w) != 1 {
		t.Errorf("contrast %v", r)
	}
	// A known pair: Teal 800 on white.
	if r := floor2(Contrast(MustHex("#00695c"), w)); r != 6.61 {
		t.Errorf("teal 800 on white %v", r)
	}
	if floor2(4.4999) != 4.49 {
		t.Error("floor2 rounds down")
	}
}

// TestOKLCHRoundTrip: every colour survives sRGB → OKLCH → sRGB, and the
// gamut mapping keeps lightness and hue while giving up chroma.
func TestOKLCHRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- a fixed test sample
	colours := []RGB{{0, 0, 0}, {255, 255, 255}, {255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {128, 128, 128}, {255, 255, 0}}
	for range 3000 {
		colours = append(colours, RGB{uint8(r.IntN(256)), uint8(r.IntN(256)), uint8(r.IntN(256))})
	}
	for _, c := range colours {
		back := ToLCH(c).ToRGB()
		if d := max(diff(c.R, back.R), diff(c.G, back.G), diff(c.B, back.B)); d > 1 {
			t.Fatalf("%s → %+v → %s", c.Hex(), ToLCH(c), back.Hex())
		}
	}
	// Out of gamut: a very colourful light blue does not exist in sRGB.
	p := LCH{0.9, 0.4, 260}
	got := ToLCH(p.ToRGB())
	if math.Abs(got.L-0.9) > 0.01 || math.Abs(got.H-260) > 3 || got.C >= 0.4 {
		t.Errorf("gamut mapping: %+v", got)
	}
	if (LCH{1.5, 0, 0}).ToRGB() != white || (LCH{-1, 0, 0}).ToRGB() != (RGB{}) {
		t.Error("lightness is clamped")
	}
	if ToLCH(MustHex("#808080")).C != 0 || ToLCH(MustHex("#0000ff")).H < 200 {
		t.Error("grey has no chroma; blue's hue is around 264°")
	}
	if deltaE(white, white) != 0 || deltaE(white, RGB{}) < 0.99 {
		t.Error("deltaE")
	}
}

func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestFitEdges: nothing can satisfy an impossible need, and the colour is
// returned as it was.
func TestFitEdges(t *testing.T) {
	grey := MustHex("#777777")
	if got, ok := fit(grey, []need{{token: "x", other: grey, min: 22}}); ok || got != grey {
		t.Errorf("impossible: %s %v", got.Hex(), ok)
	}
	// Barely failing: the result is within the step and passes.
	got, ok := fit(MustHex("#767676"), []need{{token: "surface", other: white, min: 4.6}})
	if !ok || Contrast(got, white) < 4.6 || ToLCH(got).L > ToLCH(MustHex("#767676")).L {
		t.Errorf("near: %s", got.Hex())
	}
}
