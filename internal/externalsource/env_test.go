package externalsource

import (
	"errors"
	"strings"
	"testing"
)

// Coverage for the environment-variable expansion added in GO-055 (issue #35):
// inline "$VAR"/"${VAR}" in url/headers/query, the "$$" escape, the distinct
// UnsetEnvError, and the "optional source with an unset variable is skipped"
// rule that lets one config serve a whole team.

func TestExpandEnvInline(t *testing.T) {
	t.Setenv("API_BASE", "https://api.example.com")
	t.Setenv("API_VER", "v2")
	t.Setenv("EMPTY_VAR", "")

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr string // expected unset variable name, "" when expansion succeeds
	}{
		{"plain value", "https://api.example.com/x", "https://api.example.com/x", ""},
		{"whole value", "$API_BASE", "https://api.example.com", ""},
		{"prefix", "$API_BASE/api/products", "https://api.example.com/api/products", ""},
		{"braced", "${API_BASE}/api", "https://api.example.com/api", ""},
		{"two refs", "$API_BASE/$API_VER/products", "https://api.example.com/v2/products", ""},
		{"braced adjacent", "${API_BASE}${API_VER}", "https://api.example.comv2", ""},
		{"dollar escape", "price is $$5", "price is $5", ""},
		{"non-identifier is literal", "price is $5 today", "price is $5 today", ""},
		{"trailing dollar", "a$", "a$", ""},
		{"unset", "$NOT_SET_ANYWHERE/x", "", "NOT_SET_ANYWHERE"},
		{"empty counts as unset", "$EMPTY_VAR/x", "", "EMPTY_VAR"},
		{"first unset wins", "$NOT_SET_ANYWHERE/$ALSO_NOT_SET", "", "NOT_SET_ANYWHERE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandEnvInline("src", "url", tc.value)
			if tc.wantErr != "" {
				var unset *UnsetEnvError
				if !errors.As(err, &unset) {
					t.Fatalf("expandEnvInline(%q) error = %v, want *UnsetEnvError", tc.value, err)
				}
				if unset.Name != tc.wantErr {
					t.Errorf("unset variable = %q, want %q", unset.Name, tc.wantErr)
				}
				if !strings.Contains(unset.Error(), "$"+tc.wantErr) {
					t.Errorf("error %q does not name the variable", unset.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("expandEnvInline(%q) error = %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("expandEnvInline(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestResolveHTTPExpandsEnv is the issue's headline case: one config that points
// at production or at a local Worker depending on the environment.
func TestResolveHTTPExpandsEnv(t *testing.T) {
	t.Setenv("MY_API_BASE", "http://127.0.0.1:8787")
	t.Setenv("API_TOKEN_VALUE", "s3cret")

	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"accommodations": {
			Type:      "http",
			Format:    "json",
			URL:       "$MY_API_BASE/api/accommodations",
			Headers:   map[string]string{"Authorization": "Bearer $API_TOKEN_VALUE"},
			Query:     map[string]string{"base": "${MY_API_BASE}"},
			AllowHTTP: boolPtr(true),
		},
	}}
	sources, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	src := sources[0]
	if src.URL != "http://127.0.0.1:8787/api/accommodations" {
		t.Errorf("url = %q, want the expanded local URL", src.URL)
	}
	if src.Headers["Authorization"] != "Bearer s3cret" {
		t.Errorf("header = %q, want inline expansion", src.Headers["Authorization"])
	}
	if src.Query["base"] != "http://127.0.0.1:8787" {
		t.Errorf("query = %q, want inline expansion", src.Query["base"])
	}
}

// TestResolveAllSkipsOptionalUnsetEnv verifies the documented rule: an unset
// variable fails a required source and only warns for an optional one.
func TestResolveAllSkipsOptionalUnsetEnv(t *testing.T) {
	optional := Config{Enabled: true, Sources: map[string]SourceConfig{
		"local": {Type: "http", Format: "json", URL: "$SSG_TEST_UNSET_BASE/api", Required: boolPtr(false)},
		"file":  {Type: "file", Format: "yaml", Path: "nav.yaml"},
	}}
	sources, warnings, err := resolveAll(optional)
	if err != nil {
		t.Fatalf("resolveAll: %v, want the optional source skipped", err)
	}
	if len(sources) != 1 || sources[0].Name != "file" {
		t.Fatalf("sources = %+v, want only the file source", sources)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "SSG_TEST_UNSET_BASE") {
		t.Fatalf("warnings = %v, want one naming the variable", warnings)
	}

	// The same source, required (the default), aborts the build instead.
	required := Config{Enabled: true, Sources: map[string]SourceConfig{
		"local": {Type: "http", Format: "json", URL: "$SSG_TEST_UNSET_BASE/api"},
	}}
	if _, _, err := resolveAll(required); err == nil {
		t.Fatal("resolveAll with a required source = nil error, want failure")
	}

	// defaults.required: false covers every source that does not say otherwise.
	viaDefaults := Config{Enabled: true, Defaults: Defaults{Required: boolPtr(false)},
		Sources: map[string]SourceConfig{
			"local": {Type: "http", Format: "json", URL: "$SSG_TEST_UNSET_BASE/api"},
		}}
	sources, warnings, err = resolveAll(viaDefaults)
	if err != nil || len(sources) != 0 || len(warnings) != 1 {
		t.Fatalf("resolveAll via defaults = (%v, %v, %v), want a single warning", sources, warnings, err)
	}
}

// TestAllowSwitchesLayerFromDefaults covers allow_http/allow_private set once
// under defaults, which previously had no effect (issue #35).
func TestAllowSwitchesLayerFromDefaults(t *testing.T) {
	cfg := Config{Enabled: true,
		Defaults: Defaults{AllowHTTP: boolPtr(true), AllowPrivate: boolPtr(true)},
		Sources: map[string]SourceConfig{
			"a": {Type: "http", Format: "json", URL: "http://127.0.0.1:8787/a.json"},
			// A source may still opt back out of what defaults allow.
			"b": {Type: "http", Format: "json", URL: "https://api.example.com/b.json", AllowHTTP: boolPtr(false)},
		}}
	sources, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !sources[0].AllowHTTP || !sources[0].AllowPrivate {
		t.Errorf("source a = %+v, want both switches inherited from defaults", sources[0])
	}
	if sources[1].AllowHTTP {
		t.Error("source b overrode allow_http to false, want the override honoured")
	}
	if !sources[1].AllowPrivate {
		t.Error("source b should still inherit allow_private from defaults")
	}
}
