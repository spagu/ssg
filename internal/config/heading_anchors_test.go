package config

// heading_anchors: in its three spellings and three file formats (#364).

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadAnchors(t *testing.T, name, body string) HeadingAnchorsConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return cfg.HeadingAnchors
}

func TestHeadingAnchorsConfig(t *testing.T) {
	full := HeadingAnchorsConfig{Enabled: true, Levels: []int{2, 3}, Symbol: "¶", Class: "a", Label: "L {text}"}
	for name, c := range map[string]struct {
		file, body string
		want       HeadingAnchorsConfig
	}{
		"yaml bool":        {".ssg.yaml", "heading_anchors: true\n", HeadingAnchorsConfig{Enabled: true}},
		"yaml mapping":     {".ssg.yaml", "heading_anchors: {levels: [2, 3], symbol: \"¶\", class: a, label: \"L {text}\"}\n", full},
		"yaml enabled off": {".ssg.yaml", "heading_anchors: {enabled: false, symbol: x}\n", HeadingAnchorsConfig{Symbol: "x"}},
		"json bool":        {"ssg.json", `{"heading_anchors": true}`, HeadingAnchorsConfig{Enabled: true}},
		"json object":      {"ssg.json", `{"heading_anchors": {"levels": [2, 3], "symbol": "¶", "class": "a", "label": "L {text}"}}`, full},
		"toml bool":        {"ssg.toml", "heading_anchors = true\n", HeadingAnchorsConfig{Enabled: true}},
		"toml table":       {"ssg.toml", "[heading_anchors]\nlevels = [2, 3]\nsymbol = \"¶\"\nclass = \"a\"\nlabel = \"L {text}\"\n", full},
		"unset":            {".ssg.yaml", "domain: example.com\n", HeadingAnchorsConfig{}},
	} {
		if got := loadAnchors(t, c.file, c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", name, got, c.want)
		}
	}
}

func TestHeadingAnchorsConfigRejectsOtherShapes(t *testing.T) {
	var h HeadingAnchorsConfig
	if err := h.UnmarshalTOML("yes"); err == nil {
		t.Error("a string is not a toggle")
	}
	if err := h.UnmarshalJSON([]byte(`"yes"`)); err == nil {
		t.Error("a JSON string is not a toggle")
	}
	path := filepath.Join(t.TempDir(), ".ssg.yaml")
	_ = os.WriteFile(path, []byte("heading_anchors: [1]\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("a list is not a mapping")
	}
}
