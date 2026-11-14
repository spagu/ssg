package admintheme

// The panels' colour tokens as shipped in workers/core/public/core/tokens.css
// (which mirrors newsletter-ui's) — a test holds these maps to that file —
// and which of them a brand changes.

// Themed lists, in the order theme.css writes them, the tokens a brand sets.
// Everything else stays as shipped: the neutral backgrounds and text, which a
// brand has no reason to change and which every other pair is built against,
// and the state colours (danger, critical, warning, ok, info), which never
// come from a brand so that red always means an error.
var Themed = []string{
	"primary", "on-primary", "primary-soft", "on-primary-soft", "link", "focus",
	"c1", "c2", "c3", "c4", "c5",
	"nav-bg", "on-nav", "nav-muted", "nav-hover", "nav-control", "nav-current",
}

// IsThemed reports whether name is a token a brand sets.
func IsThemed(name string) bool {
	for _, t := range Themed {
		if t == name {
			return true
		}
	}
	return false
}

// Shipped holds every colour token of tokens.css, per scheme.
var Shipped = map[string]map[string]string{
	"light": {
		"bg": "#efebe9", "surface": "#ffffff", "surface-sunk": "#efebe9", "text": "#3e2723", "text-muted": "#5d4037",
		"border": "#d7ccc8", "control": "#8d6e63", "primary": "#00695c", "on-primary": "#ffffff", "primary-soft": "#e0f2f1",
		"on-primary-soft": "#004d40", "link": "#00695c", "focus": "#bf360c", "danger": "#c62828", "on-danger": "#ffffff", "critical": "#c62828",
		"warning": "#e65100", "ok": "#2e7d32", "info": "#00695c", "grid": "#d7ccc8", "axis": "#8d6e63",
		"c1": "#00897b", "c2": "#e64a19", "c3": "#5e35b1", "c4": "#c2185b", "c5": "#1565c0",
		"nav-bg": "#004d40", "on-nav": "#ffffff", "nav-muted": "#b2dfdb", "nav-hover": "#00695c", "nav-control": "#80cbc4", "nav-current": "#ffab91",
	},
	"dark": {
		"bg": "#121212", "surface": "#1e1e1e", "surface-sunk": "#121212", "text": "#efebe9", "text-muted": "#d7ccc8",
		"border": "#4e342e", "control": "#a1887f", "primary": "#80cbc4", "on-primary": "#004d40", "primary-soft": "#004d40",
		"on-primary-soft": "#b2dfdb", "link": "#80cbc4", "focus": "#ffab91", "danger": "#ef9a9a", "on-danger": "#3e2723", "critical": "#ef9a9a",
		"warning": "#ffb74d", "ok": "#81c784", "info": "#80cbc4", "grid": "#3e2723", "axis": "#a1887f",
		"c1": "#26a69a", "c2": "#f4511e", "c3": "#7e57c2", "c4": "#d81b60", "c5": "#1e88e5",
		"nav-bg": "#004d40", "on-nav": "#ffffff", "nav-muted": "#b2dfdb", "nav-hover": "#00695c", "nav-control": "#80cbc4", "nav-current": "#ffab91",
	},
}

// Schemes in the order everything is listed.
var Schemes = []string{"light", "dark"}

// pair is a foreground and the background it is used on (or two colours
// that sit side by side), and what it must reach.
type pair struct {
	fg, bg string
	min    float64 // contrast ratio; 0 for a distance pair
	apart  float64 // OKLab distance; 0 for a contrast pair
}

const (
	textMin = 4.5 // WCAG 2.2 1.4.3, text
	uiMin   = 3.0 // WCAG 2.2 1.4.11 and 2.4.13, marks, boundaries, focus
)

// pairs is every pair the panels use that involves a themed token — the
// ones test/home-contrast in workers/core checks for the shipped tokens, and
// the ones the bar's stylesheet adds (muted text and edges on a hovered
// item). Derivation meets each by construction; Verify proves it, and checks
// a configuration's own token overrides against the same list.
var pairs = []pair{
	{fg: "primary", bg: "surface", min: textMin}, {fg: "primary", bg: "bg", min: textMin}, {fg: "primary", bg: "surface-sunk", min: textMin},
	{fg: "on-primary", bg: "primary", min: textMin},
	// Links are text on the page, on cards and on inset areas.
	{fg: "link", bg: "surface", min: textMin}, {fg: "link", bg: "bg", min: textMin}, {fg: "link", bg: "surface-sunk", min: textMin},
	{fg: "on-primary-soft", bg: "primary-soft", min: textMin}, {fg: "surface", bg: "on-primary-soft", min: textMin},
	{fg: "on-nav", bg: "nav-bg", min: textMin}, {fg: "nav-muted", bg: "nav-bg", min: textMin},
	{fg: "on-nav", bg: "nav-hover", min: textMin}, {fg: "nav-muted", bg: "nav-hover", min: textMin},
	{fg: "focus", bg: "surface", min: uiMin}, {fg: "focus", bg: "bg", min: uiMin}, {fg: "focus", bg: "surface-sunk", min: uiMin},
	{fg: "c1", bg: "surface", min: uiMin}, {fg: "c2", bg: "surface", min: uiMin}, {fg: "c3", bg: "surface", min: uiMin},
	{fg: "c4", bg: "surface", min: uiMin}, {fg: "c5", bg: "surface", min: uiMin},
	{fg: "nav-current", bg: "nav-bg", min: uiMin}, {fg: "nav-current", bg: "nav-hover", min: uiMin},
	{fg: "nav-control", bg: "nav-bg", min: uiMin}, {fg: "nav-control", bg: "nav-hover", min: uiMin},
	// Side by side: the focus ring around a primary button, and the series of
	// one chart.
	{fg: "focus", bg: "primary", apart: Apart},
	{fg: "c1", bg: "c2", apart: SeriesApart}, {fg: "c1", bg: "c3", apart: SeriesApart}, {fg: "c1", bg: "c4", apart: SeriesApart}, {fg: "c1", bg: "c5", apart: SeriesApart},
	{fg: "c2", bg: "c3", apart: SeriesApart}, {fg: "c2", bg: "c4", apart: SeriesApart}, {fg: "c2", bg: "c5", apart: SeriesApart},
	{fg: "c3", bg: "c4", apart: SeriesApart}, {fg: "c3", bg: "c5", apart: SeriesApart}, {fg: "c4", bg: "c5", apart: SeriesApart},
}
