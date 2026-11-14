package modules

// Theme is the registry's `theme` field (GO-152): the site's look for the
// admin panels, computed by the build from `admin_theme:` in .ssg.yaml
// (internal/admintheme). Null in the registry when the site sets none.
//
// The core worker serves the colour tokens as /core/theme.css (the build
// writes that file too; the worker does no colour maths) and the rest as data:
// the brand for the shared navigation, and the palette, its contrast checks
// and the adjustments for the read-only "Appearance" view. A field that the
// configuration leaves out is null, never an empty string.
type Theme struct {
	// Name replaces "Site admin" in the shared navigation.
	Name *string `json:"name"`
	// Logo and LogoDark are paths of image files on this host.
	Logo     *string `json:"logo"`
	LogoDark *string `json:"logoDark"`
	// FontDisplay (headings) and FontUI (everything else) are the CSS font
	// stacks as written to theme.css, fallbacks included.
	FontDisplay *string    `json:"fontDisplay"`
	FontUI      *string    `json:"fontUi"`
	FontFiles   []FontFile `json:"fontFiles"`
	// Main and Accent are the two brand colours as configured; Derived lists
	// which of them the build derived from the other ("main", "accent").
	Main    *string  `json:"main"`
	Accent  *string  `json:"accent"`
	Derived []string `json:"derived"`
	// Light and Dark map token names (without the leading "--") to the
	// colours theme.css sets in each scheme. Empty when no colour is set.
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
	// Adjustments says where a brand colour could not serve a role as given
	// and what was used instead.
	Adjustments []Adjustment `json:"adjustments"`
	// Checks is every contrast pair the panels use that involves a themed
	// token, with its ratio.
	Checks []Check `json:"checks"`
}

// FontFile is one @font-face the build writes into theme.css.
type FontFile struct {
	Family string `json:"family"`
	Src    string `json:"src"`
	Format string `json:"format"`
	Weight string `json:"weight"`
	Style  string `json:"style"`
}

// Adjustment records a brand colour moved in lightness to serve a role.
type Adjustment struct {
	Scheme    string `json:"scheme"`
	Token     string `json:"token"`
	Requested string `json:"requested"`
	Used      string `json:"used"`
	// Reason is "contrast" (too close in lightness to Against) or "distinct"
	// (too similar to the colour of Against, which sits next to it).
	Reason  string `json:"reason"`
	Against string `json:"against"`
}

// Check is one foreground/background pair and its WCAG contrast ratio,
// rounded down to two decimals, and the minimum it must reach: 4.5 for text,
// 3 for a mark or a boundary.
type Check struct {
	Scheme  string  `json:"scheme"`
	Fg      string  `json:"fg"`
	Bg      string  `json:"bg"`
	Ratio   float64 `json:"ratio"`
	Minimum float64 `json:"minimum"`
}
