package config

// og_cards: in its spellings and three file formats.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadOGCards(t *testing.T, name, body string) OGCardsConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return cfg.OGCards
}

func TestOGCardsConfig(t *testing.T) {
	full := OGCardsConfig{Enabled: true, Dir: "cards", Sections: []string{"posts"}, Width: 1200, Height: 630,
		Background: "#111111", TextColor: "#ffffff", AccentColor: "#8ab4f8", BackgroundImage: "bg.jpg",
		Overlay: 0, Logo: "logo.png", Font: "font.ttf", Format: "jpeg", Quality: 80}
	on := OGCardsConfig{Enabled: true, Overlay: DefaultOGCardOverlay}
	for name, c := range map[string]struct {
		file, body string
		want       OGCardsConfig
	}{
		"yaml bool": {".ssg.yaml", "og_cards: true\n", on},
		"yaml mapping": {".ssg.yaml", "og_cards:\n  dir: cards\n  sections: [posts]\n  width: 1200\n  height: 630\n" +
			"  background: \"#111111\"\n  text_color: \"#ffffff\"\n  accent_color: \"#8ab4f8\"\n  background_image: bg.jpg\n" +
			"  overlay: 0\n  logo: logo.png\n  font: font.ttf\n  format: jpeg\n  quality: 80\n", full},
		"yaml default overlay": {".ssg.yaml", "og_cards: {dir: og}\n", OGCardsConfig{Enabled: true, Dir: "og", Overlay: DefaultOGCardOverlay}},
		"yaml enabled off":     {".ssg.yaml", "og_cards: {enabled: false}\n", OGCardsConfig{Overlay: DefaultOGCardOverlay}},
		"json bool":            {"ssg.json", `{"og_cards": true}`, on},
		"json object": {"ssg.json", `{"og_cards": {"dir": "cards", "sections": ["posts"], "width": 1200, "height": 630,
			"background": "#111111", "text_color": "#ffffff", "accent_color": "#8ab4f8", "background_image": "bg.jpg",
			"overlay": 0, "logo": "logo.png", "font": "font.ttf", "format": "jpeg", "quality": 80}}`, full},
		"toml bool": {"ssg.toml", "og_cards = true\n", on},
		"toml table": {"ssg.toml", "[og_cards]\ndir = \"cards\"\nsections = [\"posts\"]\nwidth = 1200\nheight = 630\n" +
			"background = \"#111111\"\ntext_color = \"#ffffff\"\naccent_color = \"#8ab4f8\"\nbackground_image = \"bg.jpg\"\n" +
			"overlay = 0.0\nlogo = \"logo.png\"\nfont = \"font.ttf\"\nformat = \"jpeg\"\nquality = 80\n", full},
		"unset": {".ssg.yaml", "domain: example.com\n", OGCardsConfig{}},
	} {
		if got := loadOGCards(t, c.file, c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", name, got, c.want)
		}
	}
}
