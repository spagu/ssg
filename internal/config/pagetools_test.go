package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// loadPageTools loads a config file with the given name and body.
func loadPageTools(t *testing.T, name, body string) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return cfg
}

// TestCookModeShorthand: `cook_mode: true` and the full mapping both work,
// in every config format.
func TestCookModeShorthand(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"a.yaml", "cook_mode: true\n"},
		{"b.json", `{"cook_mode": true}`},
		{"c.toml", "cook_mode = true\n"},
	} {
		if cfg := loadPageTools(t, tt.name, tt.body); !cfg.CookMode.Enabled {
			t.Errorf("%s: shorthand not enabled", tt.name)
		}
	}
	for _, tt := range []struct{ name, body string }{
		{"a.yaml", "cook_mode:\n  enabled: true\n  label: Gotuj\n  auto: false\n  labels:\n    next: Dalej\n"},
		{"b.json", `{"cook_mode": {"enabled": true, "label": "Gotuj", "auto": false, "labels": {"next": "Dalej"}}}`},
		{"c.toml", "[cook_mode]\nenabled = true\nlabel = \"Gotuj\"\nauto = false\n[cook_mode.labels]\nnext = \"Dalej\"\n"},
	} {
		c := loadPageTools(t, tt.name, tt.body).CookMode
		if !c.Enabled || c.Label != "Gotuj" || c.Auto == nil || *c.Auto || c.Labels["next"] != "Dalej" {
			t.Errorf("%s: mapping = %+v", tt.name, c)
		}
	}
	p := loadPageTools(t, "p.yaml", "print:\n  enabled: true\n  label: Drukuj\n  sections: [posts, pages]\n  labels:\n    recipe: Przepis\n").Print
	if !p.Enabled || p.Label != "Drukuj" || len(p.Sections) != 2 || p.Labels["recipe"] != "Przepis" {
		t.Errorf("print = %+v", p)
	}
}

// TestCookModeDecodeErrors: a value of the wrong shape is reported.
func TestCookModeDecodeErrors(t *testing.T) {
	var c CookModeConfig
	if err := c.UnmarshalTOML("yes"); err == nil {
		t.Error("TOML: a string is neither form")
	}
	if err := c.UnmarshalTOML(map[string]interface{}{"enabled": func() {}}); err == nil {
		t.Error("TOML: an unencodable table must fail")
	}
	if err := c.UnmarshalJSON([]byte(`"yes"`)); err == nil {
		t.Error("JSON: a string is neither form")
	}
	if err := yaml.Unmarshal([]byte("cook_mode: maybe\n"), &struct {
		C CookModeConfig `yaml:"cook_mode"`
	}{}); err == nil {
		t.Error("YAML: a non-boolean scalar must fail")
	}
}
