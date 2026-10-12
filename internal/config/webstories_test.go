package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestWebStoriesConfig: `web_stories: true` and the full mapping both work,
// in every config format.
func TestWebStoriesConfig(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"a.yaml", "web_stories: true\n"},
		{"b.json", `{"web_stories": true}`},
		{"c.toml", "web_stories = true\n"},
	} {
		if cfg := loadPageTools(t, tt.name, tt.body); !cfg.WebStories.Enabled {
			t.Errorf("%s: shorthand not enabled", tt.name)
		}
	}
	for _, tt := range []struct{ name, body string }{
		{"a.yaml", "web_stories:\n  enabled: true\n  url: /stories/\n  publisher_logo: img/logo.png\n  pages: 5\n  min_images: 3\n  cta: Get the recipe\n  sections: [posts/recipes]\n"},
		{"b.json", `{"web_stories": {"enabled": true, "url": "/stories/", "publisher_logo": "img/logo.png", "pages": 5, "min_images": 3, "cta": "Get the recipe", "sections": ["posts/recipes"]}}`},
		{"c.toml", "[web_stories]\nenabled = true\nurl = \"/stories/\"\npublisher_logo = \"img/logo.png\"\npages = 5\nmin_images = 3\ncta = \"Get the recipe\"\nsections = [\"posts/recipes\"]\n"},
	} {
		w := loadPageTools(t, tt.name, tt.body).WebStories
		if !w.Enabled || w.URL != "/stories/" || w.PublisherLogo != "img/logo.png" || w.Pages != 5 || w.MinImages != 3 ||
			w.CTA != "Get the recipe" || len(w.Sections) != 1 {
			t.Errorf("%s: mapping = %+v", tt.name, w)
		}
	}
}

// TestWebStoriesDecodeErrors: a value of the wrong shape is reported.
func TestWebStoriesDecodeErrors(t *testing.T) {
	var w WebStoriesConfig
	if err := w.UnmarshalTOML("yes"); err == nil {
		t.Error("TOML: a string is neither form")
	}
	if err := w.UnmarshalTOML(map[string]interface{}{"enabled": func() {}}); err == nil {
		t.Error("TOML: an unencodable table must fail")
	}
	if err := w.UnmarshalJSON([]byte(`"yes"`)); err == nil {
		t.Error("JSON: a string is neither form")
	}
	if err := yaml.Unmarshal([]byte("web_stories: maybe\n"), &struct {
		W WebStoriesConfig `yaml:"web_stories"`
	}{}); err == nil {
		t.Error("YAML: a non-boolean scalar must fail")
	}
}
