package config

// web_push: in its spellings and three file formats (#368).

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebPushConfig(t *testing.T) {
	for name, c := range map[string]struct {
		file, body string
		want       WebPushConfig
	}{
		"yaml bool":        {".ssg.yaml", "web_push: true\n", WebPushConfig{Enabled: true}},
		"yaml mapping":     {".ssg.yaml", "web_push: {endpoint: /push}\n", WebPushConfig{Enabled: true, Endpoint: "/push"}},
		"yaml enabled off": {".ssg.yaml", "web_push: {enabled: false, endpoint: /push}\n", WebPushConfig{Endpoint: "/push"}},
		"json bool":        {"ssg.json", `{"web_push": true}`, WebPushConfig{Enabled: true}},
		"json object":      {"ssg.json", `{"web_push": {"endpoint": "/push"}}`, WebPushConfig{Enabled: true, Endpoint: "/push"}},
		"toml bool":        {"ssg.toml", "web_push = true\n", WebPushConfig{Enabled: true}},
		"toml table":       {"ssg.toml", "[web_push]\nendpoint = \"/push\"\n", WebPushConfig{Enabled: true, Endpoint: "/push"}},
		"unset":            {".ssg.yaml", "domain: example.com\n", WebPushConfig{}},
	} {
		path := filepath.Join(t.TempDir(), c.file)
		if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cfg.WebPush != c.want {
			t.Errorf("%s: got %+v, want %+v", name, cfg.WebPush, c.want)
		}
	}
}

func TestWebPushConfigRejectsOtherShapes(t *testing.T) {
	var w WebPushConfig
	if err := w.UnmarshalTOML("yes"); err == nil {
		t.Error("a string is not a toggle")
	}
	if err := w.UnmarshalJSON([]byte(`"yes"`)); err == nil {
		t.Error("a JSON string is not a toggle")
	}
	if err := w.UnmarshalTOML(map[string]interface{}{"endpoint": func() {}}); err == nil {
		t.Error("a table JSON cannot encode must fail")
	}
	path := filepath.Join(t.TempDir(), ".ssg.yaml")
	_ = os.WriteFile(path, []byte("web_push: [1]\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("a list is not a mapping")
	}
}
