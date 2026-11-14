package admintheme

// OKLab / OKLCH (Björn Ottosson, 2020): a perceptual colour space in which
// changing L changes only how light a colour looks, so a brand colour can be
// made lighter or darker without its hue drifting. Implemented here rather
// than imported: it is forty lines and the build takes no dependency for it.

import "math"

// LCH is a colour in OKLCH: lightness 0..1, chroma ≥ 0, hue in degrees.
type LCH struct{ L, C, H float64 }

// lab is a colour in OKLab.
type lab struct{ L, A, B float64 }

func cbrt(v float64) float64 { return math.Cbrt(v) }

// toLab converts an 8-bit sRGB colour to OKLab.
func toLab(c RGB) lab {
	r, g, b := linear(c.R), linear(c.G), linear(c.B)
	l := cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return lab{
		L: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		A: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		B: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// ToLCH converts an 8-bit sRGB colour to OKLCH. A grey has chroma 0 and hue 0.
func ToLCH(c RGB) LCH {
	p := toLab(c)
	ch := math.Hypot(p.A, p.B)
	if ch < 1e-4 {
		return LCH{L: p.L}
	}
	h := math.Atan2(p.B, p.A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return LCH{L: p.L, C: ch, H: h}
}

// linearRGB converts OKLCH to linear-light sRGB, possibly out of [0, 1].
func linearRGB(p LCH) (r, g, b float64) {
	rad := p.H * math.Pi / 180
	a, bb := p.C*math.Cos(rad), p.C*math.Sin(rad)
	l := p.L + 0.3963377774*a + 0.2158037573*bb
	m := p.L - 0.1055613458*a - 0.0638541728*bb
	s := p.L - 0.0894841775*a - 1.2914855480*bb
	l, m, s = l*l*l, m*m*m, s*s*s
	return 4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s
}

// inGamut: the linear values fit sRGB (with a rounding allowance).
func inGamut(r, g, b float64) bool {
	const e = 1e-6
	return r >= -e && r <= 1+e && g >= -e && g <= 1+e && b >= -e && b <= 1+e
}

// encode turns one linear channel into 8 bits.
func encode(v float64) uint8 {
	v = math.Min(1, math.Max(0, v))
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(math.Round(v * 255))
}

// ToRGB converts OKLCH to 8-bit sRGB. A colour outside sRGB keeps its
// lightness and hue and loses chroma — only as much as it takes to fit
// (bisection) — which is what keeps a brand's hue when it is made lighter or
// darker.
func (p LCH) ToRGB() RGB {
	p.L = math.Min(1, math.Max(0, p.L))
	r, g, b := linearRGB(p)
	if !inGamut(r, g, b) {
		lo, hi := 0.0, p.C
		for range 24 {
			mid := (lo + hi) / 2
			if r2, g2, b2 := linearRGB(LCH{p.L, mid, p.H}); inGamut(r2, g2, b2) {
				lo = mid
			} else {
				hi = mid
			}
		}
		r, g, b = linearRGB(LCH{p.L, lo, p.H})
	}
	return RGB{encode(r), encode(g), encode(b)}
}

// deltaE is the OKLab distance of two colours: about 0.02 is the smallest
// difference people notice; this package asks 0.1 of colours that sit side by
// side and must be told apart.
func deltaE(x, y RGB) float64 {
	a, b := toLab(x), toLab(y)
	return math.Sqrt((a.L-b.L)*(a.L-b.L) + (a.A-b.A)*(a.A-b.A) + (a.B-b.B)*(a.B-b.B))
}
