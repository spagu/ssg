package admintheme

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
)

// extremes are the colours the ticket names, and the corners of sRGB.
var extremes = []string{
	"#ffffff", "#000000", "#808080", "#777777", "#f5f5f5", "#0a0a0a",
	"#ff0000", "#00ff00", "#0000ff", "#ffff00", "#00ffff", "#ff00ff",
	"#fffde7", "#fff9c4", "#ffffe0", "#d50000", "#1b3a4b", "#e07a5f", "#e07a60",
	"#00695c", "#bf360c", "#004d40", "#ffab91", "#123456", "#fedcba",
}

// sample is a deterministic set of colour pairs: every pair of extremes,
// near-identical pairs, and seeded random ones.
func sample(n int) [][2]*RGB {
	var out [][2]*RGB
	ptr := func(c RGB) *RGB { return &c }
	for _, a := range extremes {
		out = append(out, [2]*RGB{ptr(MustHex(a)), nil}, [2]*RGB{nil, ptr(MustHex(a))})
		for _, b := range extremes {
			out = append(out, [2]*RGB{ptr(MustHex(a)), ptr(MustHex(b))})
		}
	}
	r := rand.New(rand.NewPCG(152, 2026)) // #nosec G404 -- a fixed test sample, not a secret
	rnd := func() RGB { return RGB{uint8(r.IntN(256)), uint8(r.IntN(256)), uint8(r.IntN(256))} }
	for range n {
		a := rnd()
		near := RGB{a.R ^ uint8(r.IntN(4)), a.G ^ uint8(r.IntN(4)), a.B}
		out = append(out, [2]*RGB{ptr(a), ptr(rnd())}, [2]*RGB{ptr(a), ptr(near)})
	}
	return out
}

func name(c *RGB) string {
	if c == nil {
		return "-"
	}
	return c.Hex()
}

// TestDeriveGuarantees: for every pair of the sample, every requirement of
// every pair the panels use holds in both schemes.
func TestDeriveGuarantees(t *testing.T) {
	n := 1500
	if testing.Short() {
		n = 150
	}
	for _, in := range sample(n) {
		p := Derive(in[0], in[1])
		if problems := Verify(p.Tokens); len(problems) > 0 {
			t.Errorf("main %s accent %s: %v", name(in[0]), name(in[1]), problems)
		}
		for _, s := range Schemes {
			for _, k := range Themed {
				if _, ok := p.Tokens[s][k]; !ok {
					t.Errorf("%s: %s missing", s, k)
				}
			}
		}
	}
}

func TestDeriveIsDeterministic(t *testing.T) {
	for _, in := range sample(40) {
		if a, b := Derive(in[0], in[1]), Derive(in[0], in[1]); !reflect.DeepEqual(a, b) {
			t.Fatalf("%s %s: two runs differ", name(in[0]), name(in[1]))
		}
	}
}

// TestDeriveKeepsUsableColours: a brand colour that can serve its role is
// used exactly; only the roles that need it move, and each move is recorded.
func TestDeriveKeepsUsableColours(t *testing.T) {
	m, a := MustHex("#1b3a4b"), MustHex("#b3401f")
	p := Derive(&m, &a)
	if got := p.Tokens["light"]["primary"]; got != m {
		t.Errorf("light primary moved: %s", got.Hex())
	}
	for _, k := range []string{"nav-bg", "c1"} {
		if p.Tokens["light"][k] != m {
			t.Errorf("%s moved: %s", k, p.Tokens["light"][k].Hex())
		}
	}
	if p.Tokens["light"]["focus"] != a || p.Tokens["light"]["c2"] != a {
		t.Errorf("light accent roles moved: %v", p.Adjustments)
	}
	moved := map[string]bool{}
	for _, adj := range p.Adjustments {
		moved[adj.Scheme+"/"+adj.Token] = true
		if adj.Requested == adj.Used {
			t.Errorf("recorded a non-move %+v", adj)
		}
	}
	for _, k := range []string{"dark/primary", "dark/c1", "both/nav-current"} {
		if !moved[k] {
			t.Errorf("%s not recorded: %v", k, p.Adjustments)
		}
	}
	if moved["light/primary"] || moved["both/nav-bg"] {
		t.Errorf("a usable colour was recorded as moved: %v", p.Adjustments)
	}
}

// TestDeriveTicketExample: the coral of the ticket is too light for focus on
// white in the light scheme? It is not (3.0:1 needs only 3) — but it is too
// light as link text, which is main's role; as main it moves and says so.
func TestDeriveRecordsReasons(t *testing.T) {
	coral := MustHex("#e07a5f")
	p := Derive(&coral, nil)
	if p.Derived[0] != "accent" || p.Main != coral {
		t.Fatalf("derived %v", p.Derived)
	}
	var primary *string
	for _, adj := range p.Adjustments {
		if adj.Scheme == "light" && adj.Token == "primary" {
			s := fmt.Sprintf("%s→%s %s %s", adj.Requested, adj.Used, adj.Reason, adj.Against)
			primary = &s
		}
	}
	if primary == nil || Contrast(p.Tokens["light"]["primary"], white) < 4.5 {
		t.Errorf("light primary: %v %v", primary, p.Adjustments)
	}
	same := MustHex("#336699")
	q := Derive(&same, &same)
	found := false
	for _, adj := range q.Adjustments {
		found = found || adj.Reason == "distinct"
	}
	if !found {
		t.Errorf("identical main and accent are pulled apart and say so: %v", q.Adjustments)
	}
	if Derive(nil, nil) != nil {
		t.Error("no colour, no palette")
	}
	grey := MustHex("#808080")
	if g := Derive(&grey, nil); ToLCH(g.Accent).C < 0.05 {
		t.Errorf("a grey main gets a coloured accent: %s", g.Accent.Hex())
	}
	if g := Derive(nil, &coral); g.Derived[0] != "main" || ToLCH(g.Main).L > 0.43 {
		t.Errorf("main from accent: %s", g.Main.Hex())
	}
}

// TestDeriveSeriesApart: every two chart series, in both schemes, are at
// least SeriesApart from each other — explicitly, beside Verify — and each
// keeps 3:1 on the cards it is drawn on.
func TestDeriveSeriesApart(t *testing.T) {
	series := []string{"c1", "c2", "c3", "c4", "c5"}
	n := 600
	if testing.Short() {
		n = 60
	}
	for _, in := range sample(n) {
		p := Derive(in[0], in[1])
		for _, s := range Schemes {
			for i, a := range series {
				if Contrast(p.Tokens[s][a], p.Tokens[s]["surface"]) < uiMin {
					t.Errorf("%s %s %s: %s on surface", name(in[0]), name(in[1]), s, a)
				}
				for _, b := range series[i+1:] {
					if d := deltaE(p.Tokens[s][a], p.Tokens[s][b]); d < SeriesApart {
						t.Errorf("%s %s %s: %s %s and %s %s are %.3f apart", name(in[0]), name(in[1]), s, a, p.Tokens[s][a].Hex(), b, p.Tokens[s][b].Hex(), d)
					}
				}
			}
		}
	}
}

// TestDeriveLinks: links take the accent — exactly when it reads as text on
// every surface a link sits on, else the nearest shade that does, recorded —
// in both schemes, across the whole sample.
func TestDeriveLinks(t *testing.T) {
	n := 600
	if testing.Short() {
		n = 60
	}
	for _, in := range sample(n) {
		p := Derive(in[0], in[1])
		for _, s := range Schemes {
			link := p.Tokens[s]["link"]
			for _, bg := range []string{"surface", "bg", "surface-sunk"} {
				if r := Contrast(link, p.Tokens[s][bg]); r < textMin {
					t.Errorf("%s %s %s: link %s on %s is %.2f:1", name(in[0]), name(in[1]), s, link.Hex(), bg, r)
				}
			}
			moved := false
			for _, adj := range p.Adjustments {
				moved = moved || (adj.Scheme == s && adj.Token == "link")
			}
			if (link != p.Accent) != moved {
				t.Errorf("%s %s %s: link %s, accent %s, recorded %v", name(in[0]), name(in[1]), s, link.Hex(), p.Accent.Hex(), moved)
			}
		}
	}
	m, a := MustHex("#1b3a4b"), MustHex("#b3401f")
	if p := Derive(&m, &a); p.Tokens["light"]["link"] != a || p.Tokens["light"]["primary"] != m {
		t.Errorf("light: link %s (want the accent), primary %s (want main)", p.Tokens["light"]["link"].Hex(), p.Tokens["light"]["primary"].Hex())
	}
}
