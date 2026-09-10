package config

// `outputs:` accepts two shapes and always has (GO-092): a flat list that
// applies to every content type, and a mapping per type. The flat form is what
// existing configs write, so it cannot change meaning; the mapping is the new
// one. A second key would have left two ways to say the same thing forever.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOutputsAcceptsAFlatListAndAMapping(t *testing.T) {
	var flat struct {
		Outputs OutputsSpec `yaml:"outputs"`
	}
	if err := yaml.Unmarshal([]byte("outputs: [html, json]\n"), &flat); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(flat.Outputs.All, []string{"html", "json"}) || flat.Outputs.PerType != nil {
		t.Errorf("flat = %+v", flat.Outputs)
	}

	var perType struct {
		Outputs OutputsSpec `yaml:"outputs"`
	}
	if err := yaml.Unmarshal([]byte("outputs:\n  page: [html, json]\n  post: [html, markdown]\n"), &perType); err != nil {
		t.Fatal(err)
	}
	if perType.Outputs.All != nil {
		t.Errorf("a mapping is not a flat list: %+v", perType.Outputs)
	}
	if !reflect.DeepEqual(perType.Outputs.PerType["post"], []string{"html", "markdown"}) {
		t.Errorf("perType = %+v", perType.Outputs.PerType)
	}
}

// TestOutputsWrittenAsSomethingElseIsRefused: a scalar there is a typo, and
// guessing what it meant would publish a format nobody asked for.
func TestOutputsWrittenAsSomethingElseIsRefused(t *testing.T) {
	var cfg struct {
		Outputs OutputsSpec `yaml:"outputs"`
	}
	err := yaml.Unmarshal([]byte("outputs: html\n"), &cfg)
	if err == nil {
		t.Fatal("a bare scalar should be refused")
	}
	if got := err.Error(); !strings.Contains(got, "expected a list of formats") {
		t.Errorf("error = %q", got)
	}
}

// TestOutputsAbsentIsNotAnError: the key is optional, and an empty node must
// decode to nothing rather than fail the config.
func TestOutputsAbsentIsNotAnError(t *testing.T) {
	var spec OutputsSpec
	if err := spec.UnmarshalYAML(&yaml.Node{}); err != nil {
		t.Fatalf("an absent key = %v", err)
	}
	if spec.All != nil || spec.PerType != nil {
		t.Errorf("spec = %+v", spec)
	}
}

// TestOutputsFromJSONTakesBothShapesToo, which is the path a JSON or
// TOML-decoded config arrives by.
func TestOutputsFromJSONTakesBothShapesToo(t *testing.T) {
	var flat OutputsSpec
	if err := json.Unmarshal([]byte(`["html","json"]`), &flat); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(flat.All, []string{"html", "json"}) {
		t.Errorf("flat = %+v", flat)
	}

	var perType OutputsSpec
	if err := json.Unmarshal([]byte(`{"page":["html"]}`), &perType); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(perType.PerType["page"], []string{"html"}) {
		t.Errorf("perType = %+v", perType)
	}

	var neither OutputsSpec
	if err := json.Unmarshal([]byte(`"html"`), &neither); err == nil {
		t.Error("a bare string should be refused")
	}
}

// TestOutputsReachTheFieldsTheGeneratorReads: the decoded shape is useless
// until Load copies it across, and a config built in code sets the fields
// directly, so both have to end up in the same place.
func TestOutputsReachTheFieldsTheGeneratorReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flat.yaml")
	if err := os.WriteFile(path, []byte("domain: example.com\noutputs: [html, json]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Outputs, []string{"html", "json"}) {
		t.Errorf("outputs = %v", cfg.Outputs)
	}

	path = filepath.Join(dir, "map.yaml")
	if err := os.WriteFile(path, []byte("domain: example.com\noutputs:\n  post: [html, markdown]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.OutputsPerType["post"], []string{"html", "markdown"}) {
		t.Errorf("per type = %v", cfg.OutputsPerType)
	}
	if cfg.Outputs != nil {
		t.Errorf("a mapping must not also fill the flat list: %v", cfg.Outputs)
	}
}

// TestMCPSearchIsOffUntilBothHalvesAreGiven: a URL without a collection is a
// half-configured backend, and treating it as configured would send every find
// query at a server that cannot answer it.
func TestMCPSearchIsOffUntilBothHalvesAreGiven(t *testing.T) {
	if (MCPSearch{MddbURL: "http://mddb.example.com"}).Enabled() {
		t.Error("a URL alone is not a search backend")
	}
	if (MCPSearch{MddbCollection: "site"}).Enabled() {
		t.Error("a collection alone is not a search backend")
	}
	if !(MCPSearch{MddbURL: "http://mddb.example.com", MddbCollection: "site"}).Enabled() {
		t.Error("both halves should enable it")
	}
}

// TestMCPSearchValidatesWritesUnlessToldNotTo: validation is the default, and
// the setting exists to turn it off for a large batch — so an unset pointer
// must mean on, not off.
func TestMCPSearchValidatesWritesUnlessToldNotTo(t *testing.T) {
	if !(MCPSearch{}).ValidateEnabled() {
		t.Error("validation is on by default")
	}
	on, off := true, false
	if !(MCPSearch{MddbValidate: &on}).ValidateEnabled() {
		t.Error("explicitly on should be on")
	}
	if (MCPSearch{MddbValidate: &off}).ValidateEnabled() {
		t.Error("explicitly off should be off")
	}
}
