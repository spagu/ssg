package admintheme

import "math"

// need is one requirement a colour must meet against another, already
// decided colour: a minimum contrast ratio, or (apart) a minimum OKLab
// distance for two colours that sit side by side and must be told apart.
type need struct {
	token string // the other colour's token name, for messages
	other RGB
	min   float64
	apart float64
}

// Apart is the OKLab distance asked of the main and accent roles where they
// sit side by side (the focus ring around a primary button). SeriesApart is
// asked of every two series of a chart, in both schemes: a little more, as
// lines cross and markers are small — every pair of the shipped series
// clears it (the closest, Deep Purple and Blue, is 0.127 apart).
const (
	Apart       = 0.1
	SeriesApart = 0.12
)

// failing returns the first need c does not meet: contrast before distance,
// in the order given.
func failing(c RGB, needs []need) (need, bool) {
	for _, n := range needs {
		if n.min > 0 && Contrast(c, n.other) < n.min {
			return n, true
		}
	}
	for _, n := range needs {
		if n.apart > 0 && deltaE(c, n.other) < n.apart {
			return n, true
		}
	}
	return need{}, false
}

func meets(c RGB, needs []need) bool {
	_, bad := failing(c, needs)
	return !bad
}

// fit returns c itself when it meets every need — a brand colour that can
// serve the role is never nudged — and otherwise the colour of the same hue
// and chroma (less only where sRGB cannot hold it) at the nearest OKLCH
// lightness that does. ok is false when no lightness does.
//
// The search walks lightness in steps of 0.01 over the whole range and keeps
// the passing step nearest to the original; then it bisects between that step
// and its neighbour towards the original, keeping the passing end, so the
// result is within 0.0001 of the nearest passing lightness. Every candidate
// is judged as the 8-bit colour it rounds to — the colour actually painted.
func fit(c RGB, needs []need) (RGB, bool) {
	if meets(c, needs) {
		return c, true
	}
	p := ToLCH(c)
	at := func(l float64) RGB { return LCH{l, p.C, p.H}.ToRGB() }
	best, found := 0.0, false
	for i := 0; i <= 100; i++ {
		l := float64(i) / 100
		if meets(at(l), needs) && (!found || math.Abs(l-p.L) < math.Abs(best-p.L)) {
			best, found = l, true
		}
	}
	if !found {
		return c, false
	}
	good, bad := best, best-math.Copysign(0.01, best-p.L)
	if math.Abs(best-p.L) <= 0.01 {
		bad = p.L
	}
	for range 14 {
		mid := (good + bad) / 2
		if meets(at(mid), needs) {
			good = mid
		} else {
			bad = mid
		}
	}
	return at(good), true
}

// tone is a colour of c's hue at lightness l, its chroma capped at maxC:
// the tinted surfaces and quiet variants derived from a brand colour.
func tone(c RGB, l, maxC float64) RGB {
	p := ToLCH(c)
	return LCH{l, math.Min(p.C, maxC), p.H}.ToRGB()
}

// shift is c made lighter (dl > 0) or darker by dl in OKLCH lightness.
func shift(c RGB, dl float64) RGB {
	p := ToLCH(c)
	return LCH{p.L + dl, p.C, p.H}.ToRGB()
}
