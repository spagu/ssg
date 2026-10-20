package config

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// TestBreadcrumbsConfigShapes covers #335: `breadcrumbs:` takes a boolean or
// a mapping in YAML, JSON and TOML.
func TestBreadcrumbsConfigShapes(t *testing.T) {
	var y struct {
		B BreadcrumbsConfig `yaml:"breadcrumbs"`
	}
	if err := yaml.Unmarshal([]byte("breadcrumbs: false"), &y); err != nil || y.B.Enabled == nil || *y.B.Enabled {
		t.Fatalf("yaml false: %+v %v", y.B, err)
	}
	src := "breadcrumbs:\n  home: Strona główna\n  sections:\n    posts: {name: Baza wiedzy, url: /baza-wiedzy/}\n"
	y.B = BreadcrumbsConfig{}
	if err := yaml.Unmarshal([]byte(src), &y); err != nil || y.B.Home != "Strona główna" || y.B.Sections["posts"].URL != "/baza-wiedzy/" {
		t.Fatalf("yaml map: %+v %v", y.B, err)
	}
	if err := yaml.Unmarshal([]byte("breadcrumbs: maybe"), &y); err == nil {
		t.Error("yaml: a non-boolean scalar must be an error")
	}

	var j struct {
		B BreadcrumbsConfig `json:"breadcrumbs"`
	}
	if err := json.Unmarshal([]byte(`{"breadcrumbs": true}`), &j); err != nil || j.B.Enabled == nil || !*j.B.Enabled {
		t.Fatalf("json true: %+v %v", j.B, err)
	}
	if err := json.Unmarshal([]byte(`{"breadcrumbs": {"home": "Start"}}`), &j); err != nil || j.B.Home != "Start" {
		t.Fatalf("json map: %+v %v", j.B, err)
	}

	var tm struct {
		B BreadcrumbsConfig `toml:"breadcrumbs"`
	}
	if _, err := toml.Decode("breadcrumbs = false", &tm); err != nil || tm.B.Enabled == nil || *tm.B.Enabled {
		t.Fatalf("toml false: %+v %v", tm.B, err)
	}
	if _, err := toml.Decode("[breadcrumbs]\nhome = \"Start\"\n[breadcrumbs.sections.posts]\nname = \"Wpisy\"\n", &tm); err != nil || tm.B.Sections["posts"].Name != "Wpisy" {
		t.Fatalf("toml table: %+v %v", tm.B, err)
	}
	if err := tm.B.UnmarshalTOML("nope"); err == nil {
		t.Error("toml: a string must be an error")
	}
}
