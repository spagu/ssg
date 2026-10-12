package config

import (
	"strings"
	"testing"
)

// The resume: block reads from every format Load accepts.
func TestResumeLoads(t *testing.T) {
	yaml := writeConfig(t, ".ssg.yaml", "template: simple\ndomain: example.com\n"+
		"resume:\n  file: cv.json\n  url: /about/cv/\n  title: CV\n  contact: true\n")
	var cfg *Config
	var err error
	out := captureStderr(t, func() { cfg, err = Load(yaml) })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(out, "unknown configuration key") {
		t.Errorf("resume was reported unknown:\n%s", out)
	}
	want := ResumeConfig{File: "cv.json", URL: "/about/cv/", Title: "CV", Contact: true}
	if cfg.Resume != want {
		t.Errorf("yaml: %+v", cfg.Resume)
	}
	toml := writeConfig(t, ".ssg.toml", "template = \"simple\"\ndomain = \"example.com\"\n[resume]\nfile = \"cv.json\"\n")
	if cfg, err = Load(toml); err != nil || cfg.Resume.File != "cv.json" || cfg.Resume.Contact {
		t.Errorf("toml: %+v, %v", cfg, err)
	}
	json := writeConfig(t, ".ssg.json", `{"template": "simple", "domain": "example.com", "resume": {"file": "cv.json", "contact": false}}`)
	if cfg, err = Load(json); err != nil || cfg.Resume.File != "cv.json" {
		t.Errorf("json: %+v, %v", cfg, err)
	}
}
