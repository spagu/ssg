package admintheme

import (
	"math"

	"github.com/spagu/ssg/internal/modules"
)

// Palette is what Derive computes: every colour token of both schemes (the
// shipped neutral and state tokens included, so pairs can be checked), and
// the record of each brand colour that had to move to serve its role.
type Palette struct {
	Main, Accent RGB
	// Derived names the input the build made from the other: "main" or
	// "accent". Empty when both were given.
	Derived     []string
	Tokens      map[string]map[string]RGB
	Adjustments []modules.Adjustment
}

// The two roles (decided by the owner): main is the brand's surface — the
// navigation bar, primary buttons, the current menu item, the first chart
// series; accent is what calls attention — links (--link), the focus ring,
// the current-page marker in the bar, the second chart series.
//
// Derive returns nil when neither colour is given: the panels keep their
// shipped look. With one, the other is derived from it.
func Derive(main, accent *RGB) *Palette {
	if main == nil && accent == nil {
		return nil
	}
	p := &Palette{Tokens: map[string]map[string]RGB{}}
	switch {
	case main == nil:
		p.Accent, p.Main, p.Derived = *accent, mutedMain(*accent), []string{"main"}
	case accent == nil:
		p.Main, p.Accent, p.Derived = *main, contrastingAccent(*main), []string{"accent"}
	default:
		p.Main, p.Accent = *main, *accent
	}
	d := &deriver{}
	nav := d.nav(p.Main, p.Accent)
	for _, s := range Schemes {
		p.Tokens[s] = d.scheme(s, p.Main, p.Accent, nav)
	}
	p.Adjustments = d.adj
	return p
}

// contrastingAccent: the opposite hue at a middle lightness; a grey brand
// colour has no hue to oppose, so it gets the shipped accent's (deep orange).
func contrastingAccent(m RGB) RGB {
	p := ToLCH(m)
	h, c := math.Mod(p.H+180, 360), math.Max(p.C, 0.12)
	if p.C < 0.02 {
		h = 40
	}
	return LCH{math.Min(0.7, math.Max(0.5, p.L)), c, h}.ToRGB()
}

// mutedMain: the accent's hue, darker and with half its colourfulness.
func mutedMain(a RGB) RGB {
	p := ToLCH(a)
	return LCH{math.Min(p.L, 0.42), p.C / 2, p.H}.ToRGB()
}

var white = RGB{255, 255, 255}

type deriver struct{ adj []modules.Adjustment }

// role fits a brand colour to a role and records the move, if any. Derived
// variants (hover, soft, muted) are fitted with fit directly: they are not
// the brand's colours, so there is nothing to explain.
func (d *deriver) role(scheme, token string, want RGB, needs []need) RGB {
	used, _ := fit(want, needs)
	if used != want {
		n, _ := failing(want, needs)
		reason := "contrast"
		if n.min == 0 {
			reason = "distinct"
		}
		d.adj = append(d.adj, modules.Adjustment{Scheme: scheme, Token: token, Requested: want.Hex(), Used: used.Hex(), Reason: reason, Against: n.token})
	}
	return used
}

func fitted(c RGB, needs ...need) RGB {
	out, _ := fit(c, needs)
	return out
}

// nav derives the bar, which is its own dark surface in both schemes: white
// text needs 7:1 on it so the lighter secondary text and the hover state still
// reach 4.5:1 on it too.
func (d *deriver) nav(m, a RGB) map[string]RGB {
	bar := d.role("both", "nav-bg", m, []need{{token: "on-nav", other: white, min: 7}})
	hover := fitted(shift(bar, 0.08), need{token: "on-nav", other: white, min: textMin})
	at := func(min float64) []need {
		return []need{{token: "nav-bg", other: bar, min: min}, {token: "nav-hover", other: hover, min: min}}
	}
	return map[string]RGB{
		"nav-bg": bar, "on-nav": white, "nav-hover": hover,
		"nav-muted":   fitted(tone(m, 0.9, 0.05), at(textMin)...),
		"nav-control": fitted(tone(m, 0.8, 0.08), at(uiMin)...),
		"nav-current": d.role("both", "nav-current", a, at(uiMin)),
	}
}

// scheme derives one scheme's tokens over its shipped neutrals.
func (d *deriver) scheme(name string, m, a RGB, nav map[string]RGB) map[string]RGB {
	t := map[string]RGB{}
	for k, v := range Shipped[name] {
		t[k] = MustHex(v)
	}
	for k, v := range nav {
		t[k] = v
	}
	text := func(names ...string) []need {
		out := make([]need, 0, len(names))
		for _, n := range names {
			out = append(out, need{token: n, other: t[n], min: textMin})
		}
		return out
	}
	t["primary"] = d.role(name, "primary", m, text("surface", "bg"))
	if name == "light" {
		t["on-primary"] = white // primary reaches 4.5:1 on white, so white reaches it on primary
		t["primary-soft"] = tone(m, 0.95, 0.035)
		t["on-primary-soft"] = fitted(shift(t["primary"], -0.08), text("primary-soft", "surface")...)
	} else {
		t["on-primary"] = fitted(tone(m, 0.25, 0.06), text("primary")...)
		t["primary-soft"] = tone(m, 0.33, 0.06)
		t["on-primary-soft"] = fitted(tone(m, 0.9, 0.05), text("primary-soft", "surface")...)
	}
	// Links are text: 4.5:1 on every surface a link sits on.
	t["link"] = d.role(name, "link", a, text("surface", "bg", "surface-sunk"))
	surface := need{token: "surface", other: t["surface"], min: uiMin}
	t["focus"] = d.role(name, "focus", a, []need{surface, {token: "bg", other: t["bg"], min: uiMin}, {token: "primary", other: t["primary"], apart: Apart}})
	t["c1"] = d.role(name, "c1", m, []need{surface})
	t["c2"] = d.role(name, "c2", a, []need{surface, {token: "c1", other: t["c1"], apart: SeriesApart}})
	d.series(name, t, m)
	return t
}

// series picks c3–c5 from candidates: the shipped colours, then colours a
// sixth of the hue circle apart from main. A first pass takes candidates that
// already keep 3:1 on the surface and SeriesApart from every series chosen,
// as they are; only if that leaves a slot open does a second pass move a
// candidate to the nearest lightness that does — so series stay as far
// apart as the candidates allow, not merely at the threshold.
func (d *deriver) series(name string, t map[string]RGB, m RGB) {
	ref := ToLCH(MustHex(Shipped[name]["c3"]))
	p := ToLCH(m)
	candidates := []RGB{MustHex(Shipped[name]["c3"]), MustHex(Shipped[name]["c4"]), MustHex(Shipped[name]["c5"])}
	for _, turn := range []float64{120, 240, 60, 180, 300} {
		candidates = append(candidates, LCH{ref.L, math.Max(p.C, 0.12), math.Mod(p.H+turn, 360)}.ToRGB())
	}
	chosen := []string{"c1", "c2"}
	needs := func() []need {
		out := []need{{token: "surface", other: t["surface"], min: uiMin}}
		for _, k := range chosen {
			out = append(out, need{token: k, other: t[k], apart: SeriesApart})
		}
		return out
	}
	take := func(c RGB) {
		key := []string{"c3", "c4", "c5"}[len(chosen)-2]
		t[key] = c
		chosen = append(chosen, key)
	}
	used := make([]bool, len(candidates))
	for i, c := range candidates {
		if len(chosen) < 5 && meets(c, needs()) {
			used[i] = true
			take(c)
		}
	}
	for i, c := range candidates {
		if len(chosen) == 5 {
			return
		}
		if got, ok := fit(c, needs()); !used[i] && ok {
			take(got)
		}
	}
}
