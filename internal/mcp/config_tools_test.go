package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configServer builds a designer server over a temp config file.
func configServer(t *testing.T, yaml string, validate func(string) error) (*Server, string) {
	t.Helper()
	s, root := newTestServer(t, nil)
	path := filepath.Join(root, ".ssg.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	s.opts.ConfigPath = path
	s.opts.ValidateConfig = validate
	s.tools = s.buildTools()
	s.byName = map[string]tool{}
	for _, tl := range s.tools {
		s.byName[tl.name] = tl
	}
	return s, path
}

const baseConfig = `# Site configuration
template: basic
domain: example.com

# Diagrams
mermaid: true
mermaid_theme: neutral   # inline note
jwt_secret: $JWT
`

// TestConfigReadListsOnlyWritableKeys: the read tool shows the allow-listed keys
// with their values and never leaks other settings (notably secrets).
func TestConfigReadListsOnlyWritableKeys(t *testing.T) {
	s, _ := configServer(t, baseConfig, nil)
	out := text(call(t, s, "designer_config_read", nil))
	for _, want := range []string{"mermaid_theme", "neutral", "template", "basic", "highlight_style"} {
		if !strings.Contains(out, want) {
			t.Errorf("read output missing %q:\n%s", want, out)
		}
	}
	for _, leak := range []string{"jwt_secret", "$JWT", "domain"} {
		if strings.Contains(out, leak) {
			t.Errorf("read output leaked %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "not set") {
		t.Error("unset keys should be marked as such")
	}
}

// TestConfigSetPreservesFile: setting a key rewrites only that value and keeps
// the file's comments and other keys intact.
func TestConfigSetPreservesFile(t *testing.T) {
	s, path := configServer(t, baseConfig, nil)
	if r := call(t, s, "designer_config_set", map[string]any{"key": "mermaid_theme", "value": "forest"}); r.IsError {
		t.Fatalf("set: %s", text(r))
	}
	got, _ := os.ReadFile(path)
	out := string(got)
	if !strings.Contains(out, "mermaid_theme: forest") {
		t.Errorf("value not written:\n%s", out)
	}
	for _, keep := range []string{"# Site configuration", "# Diagrams", "template: basic", "jwt_secret: $JWT"} {
		if !strings.Contains(out, keep) {
			t.Errorf("edit lost %q:\n%s", keep, out)
		}
	}
}

// TestConfigSetTypes: bools and ints are written unquoted, and a bad value for a
// typed key is refused before anything is written.
func TestConfigSetTypes(t *testing.T) {
	s, path := configServer(t, baseConfig, nil)
	if r := call(t, s, "designer_config_set", map[string]any{"key": "minify_all", "value": "true"}); r.IsError {
		t.Fatalf("bool set: %s", text(r))
	}
	if r := call(t, s, "designer_config_set", map[string]any{"key": "paginate", "value": "12"}); r.IsError {
		t.Fatalf("int set: %s", text(r))
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "minify_all: true") || !strings.Contains(string(out), "paginate: 12") {
		t.Errorf("typed values not written:\n%s", out)
	}
	before := string(out)
	if r := call(t, s, "designer_config_set", map[string]any{"key": "paginate", "value": "lots"}); !r.IsError {
		t.Error("a non-numeric int value must be refused")
	}
	after, _ := os.ReadFile(path)
	if string(after) != before {
		t.Error("a refused value must not touch the file")
	}
}

// TestConfigSetRefusesNonPresentationKeys: secrets, deployment, server and
// content/URL keys are all rejected, and the file is untouched.
func TestConfigSetRefusesNonPresentationKeys(t *testing.T) {
	s, path := configServer(t, baseConfig, nil)
	before, _ := os.ReadFile(path)
	for _, key := range []string{"jwt_secret", "deploy_target", "domain", "content_dir", "permalinks", "hooks", "sass_binary", "ai", "mcp", "notifications"} {
		r := call(t, s, "designer_config_set", map[string]any{"key": key, "value": "x"})
		if !r.IsError || !strings.Contains(text(r), "designer_config_read") {
			t.Errorf("%q must be refused with guidance, got: %s", key, text(r))
		}
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Error("refused keys must leave the file untouched")
	}
}

// TestConfigSetRollsBackInvalid: a change that makes the config unloadable is
// reverted, so the designer cannot strand the project.
func TestConfigSetRollsBackInvalid(t *testing.T) {
	s, path := configServer(t, baseConfig, func(string) error { return fmt.Errorf("unknown theme") })
	before, _ := os.ReadFile(path)
	r := call(t, s, "designer_config_set", map[string]any{"key": "template", "value": "ghost"})
	if !r.IsError || !strings.Contains(text(r), "rolled back") {
		t.Errorf("invalid config must roll back: %s", text(r))
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("rollback did not restore the file:\n%s", after)
	}
}

// TestConfigSetAddsMissingKey: a key absent from the file is appended.
func TestConfigSetAddsMissingKey(t *testing.T) {
	s, path := configServer(t, "template: basic\n", nil)
	if r := call(t, s, "designer_config_set", map[string]any{"key": "toc", "value": "true"}); r.IsError {
		t.Fatalf("set: %s", text(r))
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "toc: true") {
		t.Errorf("missing key not appended:\n%s", out)
	}
}

// TestConfigToolsHiddenWithoutPath: no config file ⇒ the tools do not exist and
// the instructions stay silent about them.
func TestConfigToolsHiddenWithoutPath(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if r := call(t, s, "designer_config_set", map[string]any{"key": "toc", "value": "true"}); !r.IsError ||
		!strings.Contains(text(r), "unknown tool") {
		t.Errorf("config tools must be hidden: %s", text(r))
	}
	if strings.Contains(s.instructions(), "designer_config_read") {
		t.Error("instructions must not mention config tools when there is no config file")
	}
}

// TestConfigReadErrors: a missing config file reports a readable error.
func TestConfigReadErrors(t *testing.T) {
	s, path := configServer(t, baseConfig, nil)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if r := call(t, s, "designer_config_read", nil); !r.IsError {
		t.Error("missing config must error")
	}
	if r := call(t, s, "designer_config_set", map[string]any{"key": "toc", "value": "true"}); !r.IsError {
		t.Error("set against a missing config must error")
	}
	if r := call(t, s, "designer_config_set", map[string]any{"key": "toc"}); !r.IsError {
		t.Error("missing value must error")
	}
}

// TestTheSiteNameAndDescriptionAreSettable — the reported dead end. Asking the
// assistant to change the site title had no MCP-reachable answer except
// hardcoding the string into every template, which duplicates it across four
// files (#212).
func TestTheSiteNameAndDescriptionAreSettable(t *testing.T) {
	s, path := configServer(t, baseConfig, func(string) error { return nil })

	for key, want := range map[string]string{
		"title":       "Tradik",
		"description": "A site about things.",
	} {
		res := call(t, s, "designer_config_set", map[string]any{"key": key, "value": want})
		if res.IsError {
			t.Fatalf("setting %s failed: %s", key, text(res))
		}
		if got := readFileString(t, path); !strings.Contains(got, key+": "+want) &&
			!strings.Contains(got, key+": \""+want+"\"") {
			t.Errorf("%s not written to the config:\n%s", key, got)
		}
	}

	// And they are discoverable, or an assistant cannot know to use them.
	listing := text(call(t, s, "designer_config_read", map[string]any{}))
	for _, key := range []string{"title", "description"} {
		if !strings.Contains(listing, key) {
			t.Errorf("designer_config_read does not offer %q:\n%s", key, listing)
		}
	}
}

// TestTheRefusedSetIsUnchanged: the boundary is not "is it about rendering" but
// whether a key carries a secret, moves the deployment, changes what the server
// does, or changes what a URL is. Widening it for the site name must not have
// widened it for anything else (#212).
func TestTheRefusedSetIsUnchanged(t *testing.T) {
	s, _ := configServer(t, baseConfig, func(string) error { return nil })

	refused := []string{
		"jwt_secret", "deploy", "server_auth", "endpoints", "hooks",
		"sass_binary", "post_url_format", "content_dir", "output_dir",
		// Suggested alongside title/description, and absent on purpose:
		// default_language drives i18n, URL prefixes and hreflang.
		"default_language", "language", "author",
	}
	for _, key := range refused {
		res := call(t, s, "designer_config_set", map[string]any{"key": key, "value": "x"})
		if !res.IsError {
			t.Errorf("%q must not be settable through MCP", key)
		}
	}
}

// readFileString reads a file the test just wrote through.
func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
