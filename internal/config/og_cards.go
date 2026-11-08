package config

// `og_cards:` — a social preview image for every page that has none of its
// own, drawn at build time (1.9.0). See internal/ogcard.

import "gopkg.in/yaml.v3"

// DefaultOGCardOverlay is how strongly the background colour covers a
// background image when the config does not say: enough for white text to
// stay legible over most photos.
const DefaultOGCardOverlay = 0.6

// OGCardsConfig is the card layout and where the cards go. Off unless set;
// text_color and background (or background_image) are required, which
// ogcard.Spec.Load checks, so `og_cards: true` alone stops the build with the
// key to add. A mapping turns it on unless it says `enabled: false`.
type OGCardsConfig struct {
	Enabled  bool
	Dir      string   // output directory, default "og"
	Sections []string // posts, pages; default both
	Width    int
	Height   int
	// Background, TextColor and AccentColor are "#rrggbb"; TextColor and
	// Background or BackgroundImage are required, AccentColor follows TextColor.
	Background  string
	TextColor   string
	AccentColor string
	// BackgroundImage, Logo and Font are paths from the project root.
	BackgroundImage string
	Overlay         float64 // 0..1, default DefaultOGCardOverlay
	Logo            string
	Font            string
	Format          string // png (default) or jpeg
	Quality         int    // JPEG quality, default 85
}

// ogCardsFields is the mapping form. Enabled and Overlay are pointers so an
// absent key can mean "on" and "the default", and `overlay: 0` still means none.
type ogCardsFields struct {
	Enabled         *bool    `yaml:"enabled" toml:"enabled" json:"enabled"`
	Dir             string   `yaml:"dir" toml:"dir" json:"dir"`
	Sections        []string `yaml:"sections" toml:"sections" json:"sections"`
	Width           int      `yaml:"width" toml:"width" json:"width"`
	Height          int      `yaml:"height" toml:"height" json:"height"`
	Background      string   `yaml:"background" toml:"background" json:"background"`
	TextColor       string   `yaml:"text_color" toml:"text_color" json:"text_color"`
	AccentColor     string   `yaml:"accent_color" toml:"accent_color" json:"accent_color"`
	BackgroundImage string   `yaml:"background_image" toml:"background_image" json:"background_image"`
	Overlay         *float64 `yaml:"overlay" toml:"overlay" json:"overlay"`
	Logo            string   `yaml:"logo" toml:"logo" json:"logo"`
	Font            string   `yaml:"font" toml:"font" json:"font"`
	Format          string   `yaml:"format" toml:"format" json:"format"`
	Quality         int      `yaml:"quality" toml:"quality" json:"quality"`
}

func (o *OGCardsConfig) set(f ogCardsFields) {
	overlay := DefaultOGCardOverlay
	if f.Overlay != nil {
		overlay = *f.Overlay
	}
	*o = OGCardsConfig{
		Enabled: toggleOn(f.Enabled), Dir: f.Dir, Sections: f.Sections,
		Width: f.Width, Height: f.Height,
		Background: f.Background, TextColor: f.TextColor, AccentColor: f.AccentColor,
		BackgroundImage: f.BackgroundImage, Overlay: overlay, Logo: f.Logo, Font: f.Font,
		Format: f.Format, Quality: f.Quality,
	}
}

// UnmarshalYAML accepts `og_cards: true` as well as the mapping.
func (o *OGCardsConfig) UnmarshalYAML(value *yaml.Node) error {
	o.Overlay = DefaultOGCardOverlay
	return decodeToggleYAML(value, &o.Enabled, o.set)
}

// UnmarshalJSON accepts `"og_cards": true` as well as the object.
func (o *OGCardsConfig) UnmarshalJSON(data []byte) error {
	o.Overlay = DefaultOGCardOverlay
	return decodeToggleJSON(data, &o.Enabled, o.set)
}

// UnmarshalTOML accepts `og_cards = true` as well as the table.
func (o *OGCardsConfig) UnmarshalTOML(data interface{}) error {
	o.Overlay = DefaultOGCardOverlay
	return decodeToggleTOML("og_cards", data, &o.Enabled, o.set)
}
