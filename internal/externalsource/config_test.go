package externalsource

import (
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestResolveDefaultsAndOrdering(t *testing.T) {
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"zeta":  {Type: "file", Path: "z.yaml"},
		"alpha": {Type: "file", Path: "a.json"},
	}}
	sources, err := Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Name != "alpha" || sources[1].Name != "zeta" {
		t.Fatalf("ordering = %+v", sources)
	}
	a := sources[0]
	if a.Format != "json" || !a.Required || a.MaxSize != defaultMaxSize {
		t.Fatalf("defaults = %+v", a)
	}
}

func TestResolveFormatInference(t *testing.T) {
	cases := map[string]string{"a.yaml": "yaml", "b.yml": "yaml", "c.json": "json",
		"d.toml": "toml", "e.csv": "csv", "f.xml": "xml"}
	for path, want := range cases {
		cfg := Config{Sources: map[string]SourceConfig{"s": {Type: "file", Path: path}}}
		sources, err := Resolve(cfg)
		if err != nil || sources[0].Format != want {
			t.Errorf("%s: format=%q err=%v", path, sources[0].Format, err)
		}
	}
	// Explicit format wins over the extension.
	cfg := Config{Sources: map[string]SourceConfig{"s": {Type: "file", Path: "data.txt", Format: "JSON"}}}
	sources, err := Resolve(cfg)
	if err != nil || sources[0].Format != "json" {
		t.Fatalf("explicit format: %+v %v", sources, err)
	}
}

func TestResolveErrors(t *testing.T) {
	cases := map[string]SourceConfig{
		"bad name":         {Type: "file", Path: "a.yaml"},
		"http needs url":   {Type: "http"},
		"sql not yet":      {Type: "sql"},
		"cms not yet":      {Type: "cms"},
		"unknown type":     {Type: "carrier-pigeon", Path: "a.yaml"},
		"missing path":     {Type: "file"},
		"unknown format":   {Type: "file", Path: "a.parquet"},
		"explicit unknown": {Type: "file", Path: "a.yaml", Format: "parquet"},
	}
	for label, sc := range cases {
		name := "src"
		if label == "bad name" {
			name = "BadName"
		}
		cfg := Config{Sources: map[string]SourceConfig{name: sc}}
		if _, err := Resolve(cfg); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
	// Later-phase types name the phase in the error.
	cfg := Config{Sources: map[string]SourceConfig{"db": {Type: "sql"}}}
	_, err := Resolve(cfg)
	if err == nil || !strings.Contains(err.Error(), "phase 3") {
		t.Fatalf("sql error = %v", err)
	}
}

func TestResolveRequiredOverrides(t *testing.T) {
	cfg := Config{
		Defaults: Defaults{Required: boolPtr(false)},
		Sources: map[string]SourceConfig{
			"opt": {Type: "file", Path: "a.yaml"},
			"req": {Type: "file", Path: "b.yaml", Required: boolPtr(true)},
		},
	}
	sources, err := Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sources[0].Required || !sources[1].Required {
		t.Fatalf("required = %+v", sources)
	}
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{"": defaultMaxSize, "5MB": 5 << 20, "512KB": 512 << 10,
		"1GB": 1 << 30, "1024": 1024, " 2 mb ": 2 << 20}
	for in, want := range cases {
		got, err := parseSize(in, defaultMaxSize)
		if err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v (want %d)", in, got, err, want)
		}
	}
	for _, bad := range []string{"abc", "-5MB", "0"} {
		if _, err := parseSize(bad, defaultMaxSize); err == nil {
			t.Errorf("parseSize(%q): expected error", bad)
		}
	}
	// Defaults surface through Resolve.
	cfg := Config{Defaults: Defaults{MaxSize: "nope"}, Sources: map[string]SourceConfig{"s": {Type: "file", Path: "a.yaml"}}}
	if _, err := Resolve(cfg); err == nil {
		t.Fatal("bad max_response_size must error")
	}
}
