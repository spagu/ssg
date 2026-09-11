package config

// The marketing: block #264 added reached the build only when set on the struct
// directly; loaded from YAML, its keys were unknown and dropped, because
// models.Marketing carried json tags alone. Every format a struct is read from
// gets a test that reads it through that format.

import (
	"strings"
	"testing"
)

func TestMarketingLoadsFromYAML(t *testing.T) {
	path := writeConfig(t, ".ssg.yaml", "template: simple\ndomain: example.com\n"+
		"marketing:\n  og_image: /img/card.png\n  og_site_name: Example\n  twitter_site: \"@example\"\n"+
		"  verification:\n    google-site-verification: abc\n")

	var cfg *Config
	var err error
	out := captureStderr(t, func() { cfg, err = Load(path) })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(out, "unknown configuration key") {
		t.Errorf("a documented marketing key was reported unknown:\n%s", out)
	}
	m := cfg.Marketing
	if m.OGImage != "/img/card.png" || m.OGSiteName != "Example" || m.TwitterSite != "@example" {
		t.Errorf("marketing did not load from YAML: %+v", m)
	}
	if m.Verification["google-site-verification"] != "abc" {
		t.Errorf("nested map did not load: %+v", m.Verification)
	}
}

// The same block through TOML and JSON, since Load accepts all three.
func TestMarketingLoadsFromTOMLAndJSON(t *testing.T) {
	toml := writeConfig(t, ".ssg.toml", "template = \"simple\"\ndomain = \"example.com\"\n[marketing]\nog_image = \"/img/card.png\"\nog_site_name = \"Example\"\n")
	cfg, err := Load(toml)
	if err != nil {
		t.Fatalf("Load toml: %v", err)
	}
	if cfg.Marketing.OGImage != "/img/card.png" || cfg.Marketing.OGSiteName != "Example" {
		t.Errorf("toml marketing = %+v", cfg.Marketing)
	}

	js := writeConfig(t, ".ssg.json", `{"template":"simple","domain":"example.com","marketing":{"og_image":"/img/card.png","og_site_name":"Example"}}`)
	cfg, err = Load(js)
	if err != nil {
		t.Fatalf("Load json: %v", err)
	}
	if cfg.Marketing.OGImage != "/img/card.png" || cfg.Marketing.OGSiteName != "Example" {
		t.Errorf("json marketing = %+v", cfg.Marketing)
	}
}
