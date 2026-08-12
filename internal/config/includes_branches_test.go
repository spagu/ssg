package config

// Branch-level tests for the include machinery and the YAML language
// normalizer (coverage raise, 1.8.28): malformed input, the nesting limit and
// the merge helpers' fallback branches. Remote includes go to httptest servers
// only — no real network I/O.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Inputs that are not the expanded object form must pass through untouched, so
// a plain `languages: [en, pl]` config keeps working exactly as written.
func TestNormalizeYAMLLanguagesPassthrough(t *testing.T) {
	cases := map[string]string{
		"empty document":   "",
		"comment only":     "# nothing here\n",
		"plain code list":  "languages: [en, pl]\n",
		"empty list":       "languages: []\n",
		"no languages key": "domain: x\n",
	}
	for name, in := range cases {
		out, expanded, err := normalizeYAMLLanguages([]byte(in))
		if err != nil || expanded != nil || string(out) != in {
			t.Errorf("%s: out=%q expanded=%v err=%v, want untouched passthrough", name, out, expanded, err)
		}
	}
}

// Broken YAML and an expanded list whose items cannot decode into
// LanguageConfig both surface an error instead of a silently empty config.
func TestNormalizeYAMLLanguagesErrors(t *testing.T) {
	if _, _, err := normalizeYAMLLanguages([]byte("\ta: b")); err == nil {
		t.Error("invalid YAML must error")
	}
	bad := "languages:\n  - code:\n      not: a-string\n"
	if _, _, err := normalizeYAMLLanguages([]byte(bad)); err == nil {
		t.Error("an undecodable expanded language entry must error")
	}
}

// A config that parses as YAML but cannot decode into the Config struct (wrong
// type for a known key) is a load error, not a silent zero value.
func TestLoadYAMLWrongType(t *testing.T) {
	dir := t.TempDir()
	p := writeYAML(t, dir, ".ssg.yaml", "minify_all: [1, 2]\n")
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "parsing YAML config") {
		t.Fatalf("YAML type mismatch not reported: %v", err)
	}
}

// The same contract for JSON: valid JSON with a wrong-typed known key errors.
func TestLoadJSONWrongType(t *testing.T) {
	dir := t.TempDir()
	p := writeYAML(t, dir, ".ssg.json", `{"minify_all": "yes"}`)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "parsing JSON config") {
		t.Fatalf("JSON type mismatch not reported: %v", err)
	}
}

// GO-077: a functions-mode worker with no output_dir configured serves from
// the stock "output" directory.
func TestApplyWorkerWatchDefaultsOutputFallback(t *testing.T) {
	cfg := &Config{Worker: WorkerConfig{Dir: "workers/api"}}
	ApplyWorkerWatchDefaults(cfg)
	if cfg.WatchRunnerDir != "output" {
		t.Fatalf("WatchRunnerDir = %q, want the default output dir", cfg.WatchRunnerDir)
	}
}

// A chain of includes deeper than maxIncludeDepth is cut off with a clear
// error instead of recursing forever.
func TestIncludeDepthLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i <= maxIncludeDepth+1; i++ {
		body := "search_index: true\n"
		if i <= maxIncludeDepth {
			body = fmt.Sprintf("include:\n  - chain-%d.yaml\n", i+1)
		}
		writeYAML(t, dir, fmt.Sprintf("chain-%d.yaml", i), body)
	}
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - chain-0.yaml\ntemplate: t\ndomain: x\n")
	if _, err := Load(main); err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("depth limit not enforced: %v", err)
	}
}

// A remote include whose auth carries a literal secret is rejected before any
// fetch happens: credentials must reference an environment variable.
func TestIncludeAuthLiteralSecretRejected(t *testing.T) {
	dir := t.TempDir()
	main := writeYAML(t, dir, ".ssg.yaml",
		"include:\n  - url: https://example.invalid/base.yaml\n    auth:\n      type: bearer\n      token: literal-secret\ntemplate: t\n")
	if _, err := Load(main); err == nil || !strings.Contains(err.Error(), "environment variable") {
		t.Fatalf("literal secret not rejected: %v", err)
	}
}

// An included file with broken YAML is reported under the include's own name,
// so the user fixes the right file.
func TestIncludeBadYAML(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "broken.yaml", "a: [\n")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - broken.yaml\ntemplate: t\ndomain: x\n")
	if _, err := Load(main); err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("broken include not attributed to its file: %v", err)
	}
}

// An empty included file is a valid, empty document — not an error — and the
// including file's own content is preserved.
func TestIncludeEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "empty.yaml", "")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - empty.yaml\ntemplate: t\ndomain: x\n")
	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("empty include must not fail: %v", err)
	}
	if cfg.Template != "t" {
		t.Errorf("main content lost around an empty include: %q", cfg.Template)
	}
}

// include: must be a list; a scalar value and wrong-typed entries are config
// errors with actionable messages.
func TestIncludeMalformedShapes(t *testing.T) {
	cases := map[string]struct {
		body    string
		wantErr string
	}{
		"scalar include":     {"include: base.yaml\ntemplate: t\n", "must be a list"},
		"numeric entry":      {"include:\n  - 42\ntemplate: t\n", "neither a path/URL string nor a map"},
		"map without target": {"include:\n  - on_error: warn\ntemplate: t\n", "needs a `path` or a `url`"},
	}
	for name, tc := range cases {
		dir := t.TempDir()
		main := writeYAML(t, dir, ".ssg.yaml", tc.body)
		if _, err := Load(main); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.wantErr)
		}
	}
}

// A bare-string URL include (no map form) is fetched and merged like any
// remote include.
func TestIncludeBareURLString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("search_index: true\n"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - "+srv.URL+"/base.yaml\ntemplate: t\ndomain: x\n")
	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("bare URL include: %v", err)
	}
	if !cfg.SearchIndex {
		t.Error("remote content from a bare URL include was not merged")
	}
}

// Per-include fetch tuning: retries, retry_delay and timeout land in the
// entry's fetch options instead of being silently dropped.
func TestParseIncludeEntriesFetchTuning(t *testing.T) {
	raw := []interface{}{map[string]interface{}{
		"path":        "x.yaml",
		"retries":     2,
		"retry_delay": "1s",
		"timeout":     "2s",
	}}
	entries, err := parseIncludeEntries(raw)
	if err != nil || len(entries) != 1 {
		t.Fatalf("parseIncludeEntries: %v %v", entries, err)
	}
	got := entries[0].opts
	if got.Retries != 2 || got.RetryDelay != time.Second || got.Timeout != 2*time.Second {
		t.Errorf("opts = %+v, want retries=2 delay=1s timeout=2s", got)
	}
}

// asInt accepts every numeric shape a YAML/JSON decoder produces for an
// integer, and rejects anything else.
func TestAsInt(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
		ok   bool
	}{
		{int(3), 3, true},
		{int64(4), 4, true},
		{float64(5), 5, true},
		{"6", 0, false},
	}
	for _, c := range cases {
		if got, ok := asInt(c.in); got != c.want || ok != c.ok {
			t.Errorf("asInt(%v) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// deepMerge tolerates a nil destination, treating it as an empty document.
func TestDeepMergeNilDst(t *testing.T) {
	out := deepMerge(nil, map[string]interface{}{"a": 1})
	if out["a"] != 1 {
		t.Fatalf("deepMerge(nil, …) = %v", out)
	}
}

// Nested maps merge recursively through an include: the base's sibling keys
// survive and the including file wins per key — not wholesale replacement.
func TestIncludesNestedMapMerge(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "base.yaml", "params:\n  a: one\n  b: two\n")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - base.yaml\nparams:\n  b: three\n")
	data, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := resolveIncludes(main, data)
	if err != nil {
		t.Fatalf("resolveIncludes: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(merged, &doc); err != nil {
		t.Fatal(err)
	}
	params, ok := doc["params"].(map[string]interface{})
	if !ok || params["a"] != "one" || params["b"] != "three" {
		t.Errorf("nested merge = %v, want a kept from base and b overridden", doc["params"])
	}
}

// The by-name list merge only applies when every element is a map carrying a
// string name; anything else falls back to wholesale replacement.
func TestAllNamedMaps(t *testing.T) {
	cases := []struct {
		name string
		in   []interface{}
		want bool
	}{
		{"empty list", []interface{}{}, false},
		{"non-map element", []interface{}{"s"}, false},
		{"map without name", []interface{}{map[string]interface{}{"x": 1}}, false},
		{"named maps", []interface{}{map[string]interface{}{"name": "a"}}, true},
	}
	for _, c := range cases {
		if got := allNamedMaps(c.in); got != c.want {
			t.Errorf("%s: allNamedMaps = %v, want %v", c.name, got, c.want)
		}
	}
}
