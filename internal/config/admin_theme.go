package config

// AdminThemeConfig is `admin_theme:` (GO-152): the site's own look for the
// admin panels — a name, a logo, a main colour and an accent, font stacks —
// set once and served by the core worker. Every key is optional; without the
// section the panels look as shipped. The build validates every value and
// derives the full palette (internal/admintheme); this type only carries
// what the file says.
type AdminThemeConfig struct {
	// Name replaces "Site admin" in the shared navigation (plain text).
	Name string `yaml:"name" toml:"name" json:"name"`
	// Logo and LogoDark are image files on this site's host (.svg, .png,
	// .webp), shown by <img> only. The bar is dark in both schemes, so it
	// shows LogoDark when set.
	Logo     string `yaml:"logo" toml:"logo" json:"logo"`
	LogoDark string `yaml:"logo_dark" toml:"logo_dark" json:"logo_dark"`
	// Main and Accent are the brand colours, #rgb or #rrggbb.
	Main   string `yaml:"main" toml:"main" json:"main"`
	Accent string `yaml:"accent" toml:"accent" json:"accent"`
	// Font is the headings' font stack, FontBody everything else's.
	Font     string `yaml:"font" toml:"font" json:"font"`
	FontBody string `yaml:"font_body" toml:"font_body" json:"font_body"`
	// FontFiles are @font-face files on this site's host.
	FontFiles []AdminFontFile `yaml:"font_files" toml:"font_files" json:"font_files"`
	// Tokens overrides single colour tokens after derivation, per scheme:
	// {light: {primary: "#…"}, dark: {…}}. Each overridden pair is checked
	// again under check_admin_theme.
	Tokens map[string]map[string]string `yaml:"tokens" toml:"tokens" json:"tokens"`
}

// AdminFontFile is one `font_files:` entry.
type AdminFontFile struct {
	Family string `yaml:"family" toml:"family" json:"family"`
	Src    string `yaml:"src" toml:"src" json:"src"`
	Weight string `yaml:"weight" toml:"weight" json:"weight"`
	Style  string `yaml:"style" toml:"style" json:"style"`
}
