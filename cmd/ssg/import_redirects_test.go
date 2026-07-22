package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextPathToRedirect(t *testing.T) {
	cases := []struct {
		src, dst         string
		wantFrom, wantTo string
		wantErr          bool
	}{
		{"/old", "/new", "/old", "/new", false},
		{"/blog/:slug*", "/articles/:slug*", "/blog/*", "/articles/:splat", false},
		{"/num/:id(\\d+)", "/n/:id", "", "", true},
	}
	for _, c := range cases {
		from, to, err := nextPathToRedirect(c.src, c.dst)
		if (err != nil) != c.wantErr {
			t.Fatalf("nextPathToRedirect(%q) err=%v wantErr=%v", c.src, err, c.wantErr)
		}
		if err == nil && (from != c.wantFrom || to != c.wantTo) {
			t.Fatalf("nextPathToRedirect(%q,%q) = (%q,%q), want (%q,%q)", c.src, c.dst, from, to, c.wantFrom, c.wantTo)
		}
	}
}

func TestNextStatus(t *testing.T) {
	perm := true
	nonPerm := false
	if s := nextStatus(nextRedirect{Permanent: &perm}); s != 301 {
		t.Fatalf("permanent should map to 301, got %d", s)
	}
	if s := nextStatus(nextRedirect{Permanent: &nonPerm}); s != 302 {
		t.Fatalf("non-permanent should map to 302, got %d", s)
	}
	if s := nextStatus(nextRedirect{StatusCode: 307}); s != 307 {
		t.Fatalf("explicit statusCode should win, got %d", s)
	}
}

func TestParseNextRedirects_LiteralsAndSkips(t *testing.T) {
	// A backtick destination the flat parser can still see (no nested ${…}
	// braces) exercises the template-literal warning path directly. Nested
	// has:/${…} entries defeat the flat matcher and are caught one level up by
	// the source-count reconciliation (see TestImportRedirectsFromConfig_…).
	body := `return [
		{ source: '/a', destination: '/b', permanent: true },
		{ source: '/c', destination: ` + "`/tmpl`" + `, permanent: true },
	]`
	entries, warnings := parseNextRedirects(body)
	if len(entries) != 1 || entries[0].Source != "/a" {
		t.Fatalf("expected only the literal entry, got %+v", entries)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "template-literal") {
		t.Fatalf("expected a template-literal warning, got: %v", warnings)
	}
}

func TestImportRedirectsFromJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redirects.json")
	body := `[
		{"source":"/old","destination":"/new","permanent":true},
		{"source":"/t","destination":"/h","permanent":false},
		{"source":"/g","destination":"/l","permanent":true,"has":[{"type":"cookie"}]}
	]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, warnings, err := importRedirectsFromJSON(path)
	if err != nil {
		t.Fatalf("importRedirectsFromJSON: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules (conditional skipped), got %d", len(rules))
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "conditional redirect") {
		t.Fatalf("expected a conditional-skip warning, got: %v", warnings)
	}
}

func TestImportRedirectsFromConfig_Reconciliation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "next.config.ts")
	body := `module.exports = { async redirects() { return [
		{ source: '/old', destination: '/new', permanent: true },
		{ source: '/gated', destination: '/login', permanent: true, has: [{ type: 'cookie', key: 'a' }] },
	] } }`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, warnings, err := importRedirectsFromConfig(path)
	if err != nil {
		t.Fatalf("importRedirectsFromConfig: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 parsed rule, got %d", len(rules))
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "could not be parsed automatically") {
		t.Fatalf("expected a reconciliation warning for the dropped has: entry, got: %v", warnings)
	}
}

func TestParseNextRedirects_StatusCodeAndNoDestination(t *testing.T) {
	body := `[
		{ source: '/a', destination: '/b', statusCode: 307 },
		{ source: '/orphan' },
	]`
	entries, _ := parseNextRedirects(body)
	if len(entries) != 1 || entries[0].StatusCode != 307 {
		t.Fatalf("expected one entry with statusCode 307, got %+v", entries)
	}
}

func TestConvertNextRedirects_RegexParamAndConditional(t *testing.T) {
	perm := true
	entries := []nextRedirect{
		{Source: "/num/:id(\\d+)", Destination: "/n/:id", Permanent: &perm},
		{Source: "/gated", Destination: "/login", Permanent: &perm, Has: []any{map[string]any{"type": "cookie"}}},
		{Source: "/ok", Destination: "/done", Permanent: &perm},
	}
	rules, warnings, err := convertNextRedirects(entries)
	if err != nil {
		t.Fatalf("convertNextRedirects: %v", err)
	}
	if len(rules) != 1 || rules[0].From != "/ok" {
		t.Fatalf("only the clean rule should survive, got %+v", rules)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "regex-constrained") || !strings.Contains(joined, "conditional redirect") {
		t.Fatalf("expected both skip warnings, got: %v", warnings)
	}
}

func TestRenderRedirectsYAML(t *testing.T) {
	out := renderRedirectsYAML([]importedRule{{From: "/a", To: "/b", Status: 301}})
	if !strings.Contains(out, "redirects:") || !strings.Contains(out, `from: "/a"`) {
		t.Fatalf("unexpected YAML:\n%s", out)
	}
	empty := renderRedirectsYAML(nil)
	if !strings.Contains(empty, "redirects:\n  []") {
		t.Fatalf("empty should render an empty list:\n%s", empty)
	}
}

func TestDispatchSubcommand(t *testing.T) {
	if _, handled := dispatchSubcommand([]string{"build", "site"}); handled {
		t.Fatal("non-subcommand args should not be handled")
	}
	if _, handled := dispatchSubcommand([]string{"import"}); handled {
		t.Fatal("a lone verb should not be handled")
	}
	if code, handled := dispatchSubcommand([]string{"import", "redirects"}); !handled || code != 2 {
		t.Fatalf("import redirects with no input should be handled with exit 2, got (%d,%v)", code, handled)
	}
}

func TestRunImportRedirects_JSONPathAndErrors(t *testing.T) {
	if code := runImportRedirects(nil); code != 2 {
		t.Fatalf("no input should exit 2, got %d", code)
	}
	if code := runImportRedirects([]string{"--from-json", "/no/such/file.json"}); code != 1 {
		t.Fatalf("missing file should exit 1, got %d", code)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "r.json")
	if err := os.WriteFile(path, []byte(`[{"source":"/a","destination":"/b","permanent":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runImportRedirects([]string{"--from-json", path}); code != 0 {
		t.Fatalf("valid JSON should exit 0, got %d", code)
	}
	// A config-path positional argument also works.
	cfg := filepath.Join(dir, "next.config.ts")
	if err := os.WriteFile(cfg, []byte(`redirects(){return [{ source: '/x', destination: '/y', permanent: true }]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runImportRedirects([]string{cfg}); code != 0 {
		t.Fatalf("valid config should exit 0, got %d", code)
	}
}

func TestImportRedirectsFromJSON_BadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not an array}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := importRedirectsFromJSON(path); err == nil {
		t.Fatal("expected a JSON parse error")
	}
}
