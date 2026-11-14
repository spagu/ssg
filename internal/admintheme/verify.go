package admintheme

import (
	"fmt"

	"github.com/spagu/ssg/internal/modules"
)

// Problem is a pair that misses its requirement.
type Problem struct {
	Scheme string
	Fg, Bg string
	Detail string
}

// Verify checks every pair of both schemes. A palette from Derive has no
// problem (the property tests prove it); a palette with the configuration's
// own token overrides may.
func Verify(tokens map[string]map[string]RGB) []Problem {
	var out []Problem
	for _, s := range Schemes {
		t := tokens[s]
		for _, p := range pairs {
			fg, bg := t[p.fg], t[p.bg]
			switch {
			case p.min > 0 && Contrast(fg, bg) < p.min:
				out = append(out, Problem{s, p.fg, p.bg, fmt.Sprintf("%s %s on %s %s is %.2f:1, needs %.1f:1", p.fg, fg.Hex(), p.bg, bg.Hex(), floor2(Contrast(fg, bg)), p.min)})
			case p.apart > 0 && deltaE(fg, bg) < p.apart:
				out = append(out, Problem{s, p.fg, p.bg, fmt.Sprintf("%s %s and %s %s sit side by side but are too alike to tell apart (OKLab distance %.3f, needs %.2f)", p.fg, fg.Hex(), p.bg, bg.Hex(), deltaE(fg, bg), p.apart)})
			}
		}
	}
	return out
}

// Checks lists every contrast pair with its ratio, for the Appearance view.
func Checks(tokens map[string]map[string]RGB) []modules.Check {
	var out []modules.Check
	for _, s := range Schemes {
		for _, p := range pairs {
			if p.min > 0 {
				out = append(out, modules.Check{Scheme: s, Fg: p.fg, Bg: p.bg, Ratio: floor2(Contrast(tokens[s][p.fg], tokens[s][p.bg])), Minimum: p.min})
			}
		}
	}
	return out
}

// themedHex is the themed part of one scheme as hex strings.
func themedHex(t map[string]RGB) map[string]string {
	out := make(map[string]string, len(Themed))
	for _, k := range Themed {
		out[k] = t[k].Hex()
	}
	return out
}
