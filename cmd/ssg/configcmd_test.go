package main

// `ssg config view|set|unset` (GO-101).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleConfig is a config with the shape that matters here: comments above
// keys, a comment at the end of a line, blank lines between sections, and a
// nested mapping.
const sampleConfig = `# The site.
source: mysite
domain: example.com
template: simple

# How it looks.
highlight: true
highlight_style: github   # a Chroma style name

taxonomies:
  audience:
    multiple: false
`

// inProject writes a config into a temporary working directory and returns its
// path, so each test edits its own file.
func inProject(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestConfigSetChangesOneLine: the acceptance criterion of the whole feature.
func TestConfigSetChangesOneLine(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig)
	if code := runConfig([]string{"set", "highlight_style", "monokai"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	before, after := strings.Split(sampleConfig, "\n"), strings.Split(readFile(t, path), "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed: %d → %d", len(before), len(after))
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("%d lines changed, want 1", changed)
	}
}

// TestConfigSetTypesTheValue: what lands in the file is a bool, a number or a
// list, not the string the shell handed over — unless --string says so.
func TestConfigSetTypesTheValue(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig)
	if code := runConfig([]string{"set", "highlight", "false"}); code != 0 {
		t.Fatalf("bool: exit %d", code)
	}
	if code := runConfig([]string{"set", "toc_depth", "4"}); code != 0 {
		t.Fatalf("int: exit %d", code)
	}
	if code := runConfig([]string{"set", "outputs", "[html,markdown,json]"}); code != 0 {
		t.Fatalf("list: exit %d", code)
	}
	if code := runConfig([]string{"set", "--string", "title", "2026"}); code != 0 {
		t.Fatalf("--string: exit %d", code)
	}
	got := readFile(t, path)
	for _, want := range []string{"highlight: false", "toc_depth: 4", "- json", `title: "2026"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestConfigSetJSON writes a structure the flat forms cannot express.
func TestConfigSetJSON(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig)
	if code := runConfig([]string{"set", "--json", "marketing", `{"og_site_name":"Example"}`}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(readFile(t, path), "og_site_name: Example") {
		t.Errorf("json value not written:\n%s", readFile(t, path))
	}
	if code := runConfig([]string{"set", "--json", "marketing", "{not json"}); code != 1 {
		t.Errorf("malformed json: exit %d, want 1", code)
	}
}

// TestConfigSetNestedAndQuoted reaches where a top-level editor could not.
func TestConfigSetNestedAndQuoted(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig+"\nheaders:\n  \"/css/*\":\n    Cache-Control: \"public\"\n")
	if code := runConfig([]string{"set", "taxonomies.audience.multiple", "true"}); code != 0 {
		t.Fatalf("nested: exit %d", code)
	}
	if code := runConfig([]string{"set", `headers."/css/*".Cache-Control`, "public, max-age=60"}); code != 0 {
		t.Fatalf("quoted: exit %d", code)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "multiple: true") || !strings.Contains(got, "Cache-Control: public, max-age=60") {
		t.Errorf("edits not applied:\n%s", got)
	}
}

// TestConfigSetRefusesAnEditThatBreaksTheConfig: validated in a scratch file
// first, so the real one is never written and never needs rolling back.
func TestConfigSetRefusesAnEditThatBreaksTheConfig(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig)
	if code := runConfig([]string{"set", "languages", "5"}); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if got := readFile(t, path); got != sampleConfig {
		t.Errorf("the file was modified:\n%s", got)
	}
	if _, err := os.Stat(siblingCheckPath(path)); err == nil {
		t.Error("the scratch file was left behind")
	}
}

// TestConfigUnset removes a key and reports one that was never set.
func TestConfigUnset(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig)
	if code := runConfig([]string{"unset", "highlight_style"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(readFile(t, path), "highlight_style") {
		t.Error("key not removed")
	}
	if runConfig([]string{"unset", "highlight_style"}) != 1 {
		t.Error("unsetting what is not set must fail")
	}
	if runConfig([]string{"unset"}) != 2 {
		t.Error("a missing path is a usage error")
	}
}

// TestConfigView prints the file, one path, and the effective value.
func TestConfigView(t *testing.T) {
	inProject(t, ".ssg.yaml", sampleConfig)
	for _, args := range [][]string{
		{"view"},
		{"view", "highlight_style"},
		{"view", "taxonomies"},
		{"view", "--effective"},
		{"view", "--effective", "highlight_style"},
	} {
		if code := runConfig(args); code != 0 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if runConfig([]string{"view", "nope"}) != 1 {
		t.Error("an unset path must fail")
	}
	if runConfig([]string{"view", "--effective", "nope"}) != 1 {
		t.Error("an unknown key must fail even in the effective view")
	}
	if runConfig([]string{"view", "--nonsense"}) != 2 {
		t.Error("an unknown option is a usage error")
	}
}

// TestConfigViewEffectiveShowsDefaults: the effective view answers "what will
// the generator actually use", which is not what the file says.
func TestConfigViewEffectiveShowsDefaults(t *testing.T) {
	path := inProject(t, ".ssg.yaml", "source: mysite\ndomain: example.com\n")
	out, err := captureStdout(func() error {
		if runConfig([]string{"view", "--effective", "check_markup"}) != 0 {
			t.Error("a defaulted key must resolve")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "warn" {
		t.Errorf("check_markup = %q, want the default \"warn\"", strings.TrimSpace(out))
	}
	// And the raw view does not invent it.
	if runConfig([]string{"view", "check_markup"}) != 1 {
		t.Error("the raw view must not report a key the file does not have")
	}
	_ = path
}

// TestConfigRefusesNonYAML: version one edits YAML, and says so rather than
// rewriting a TOML file without its comments.
func TestConfigRefusesNonYAML(t *testing.T) {
	path := inProject(t, ".ssg.toml", "source = \"mysite\"\ndomain = \"example.com\"\n")
	if runConfig([]string{"set", "--config=" + path, "title", "X"}) != 1 {
		t.Error("a TOML config must be refused by set")
	}
	if runConfig([]string{"unset", "--config=" + path, "title"}) != 1 {
		t.Error("a TOML config must be refused by unset")
	}
	// Reading it is fine — nothing is at risk.
	if runConfig([]string{"view", "--config=" + path}) != 0 {
		t.Error("viewing a TOML config is harmless and should work")
	}
}

// TestConfigPathDiscovery: --config in either form, and the project default.
func TestConfigPathDiscovery(t *testing.T) {
	path := inProject(t, ".ssg.yaml", sampleConfig)
	got, rest := configPathFrom([]string{"--config", path, "a", "b"})
	if got != path || len(rest) != 2 {
		t.Errorf("--config FILE: %q %v", got, rest)
	}
	got, rest = configPathFrom([]string{"--config=" + path, "x"})
	if got != path || len(rest) != 1 {
		t.Errorf("--config=FILE: %q %v", got, rest)
	}
	if got, _ = configPathFrom(nil); got == "" {
		t.Error("the project's own config should be found")
	}
	// Nothing to find, and nothing to guess at.
	t.Chdir(t.TempDir())
	if runConfig([]string{"view"}) != 1 {
		t.Error("no config anywhere must be an error, not a panic")
	}
}

// TestConfigUsageErrors: the argument shapes that are simply wrong.
func TestConfigUsageErrors(t *testing.T) {
	inProject(t, ".ssg.yaml", sampleConfig)
	if runConfig([]string{"set", "onlypath"}) != 2 {
		t.Error("set needs a path and a value")
	}
	if runConfig([]string{"set", "a", "b", "c"}) != 2 {
		t.Error("set takes exactly two arguments")
	}
	if runConfig([]string{"set", "a..b", "x"}) != 1 {
		t.Error("a malformed path is refused")
	}
}

// TestConfigSubcommandDispatch keeps the verb+noun rule: a source directory
// named "config" still builds.
func TestConfigSubcommandDispatch(t *testing.T) {
	for _, noun := range []string{"view", "set", "unset"} {
		if !isConfigSubcommand(noun) {
			t.Errorf("`config %s` must dispatch", noun)
		}
	}
	for _, noun := range []string{"build", "", "--help"} {
		if isConfigSubcommand(noun) {
			t.Errorf("`config %s` must not dispatch", noun)
		}
	}
	if _, handled := dispatchSubcommand([]string{"config", "build"}); handled {
		t.Error("an unknown noun after config must fall through to the build")
	}
	if _, handled := dispatchSubcommand([]string{"config", "view"}); !handled {
		t.Error("`ssg config view` must be handled")
	}
}

// TestSiblingCheckPath keeps the extension, because that is how the loader
// picks its parser.
func TestSiblingCheckPath(t *testing.T) {
	if got := siblingCheckPath("/a/.ssg.yaml"); got != "/a/.ssg.ssgcheck.yaml" {
		t.Errorf("got %q", got)
	}
	if got := siblingCheckPath("cfg.yml"); got != "cfg.ssgcheck.yml" {
		t.Errorf("got %q", got)
	}
}

// TestConfigUnreadableFile: a config that cannot be read is reported rather
// than treated as empty.
func TestConfigUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	missing := filepath.Join(dir, "gone.yaml")
	if runConfig([]string{"view", "--config=" + missing}) != 1 {
		t.Error("a missing file must be an error")
	}
	if runConfig([]string{"set", "--config=" + missing, "title", "x"}) != 1 {
		t.Error("a missing file must be an error for set too")
	}
	if runConfig([]string{"view", "--effective", "--config=" + missing}) != 1 {
		t.Error("a missing file must be an error for the effective view")
	}
}

// TestConfigSetReportsAnUnwritableProject: when the scratch file cannot be
// written the edit stops there, with the original untouched.
func TestConfigSetReportsAnUnwritableProject(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".ssg.yaml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if code := runConfig([]string{"set", "--config=" + path, "template", "ssgtheme"}); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if readFile(t, path) != sampleConfig {
		t.Error("the config was modified despite the failure")
	}
}

// TestConfigViewEffectiveRedactsSecrets: the effective view resolves $VAR
// references, so printing it whole would put a live credential on the terminal.
func TestConfigViewEffectiveRedactsSecrets(t *testing.T) {
	t.Setenv("SSG_TEST_TOKEN", "s3cr3t-value")
	inProject(t, ".ssg.yaml", `source: mysite
domain: example.com
jwt_secret: hunter2
server_users:
  - admin:hunter2
mcp:
  git:
    token: $SSG_TEST_TOKEN
`)
	out, err := captureStdout(func() error {
		if runConfig([]string{"view", "--effective"}) != 0 {
			t.Error("the effective view should print")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"hunter2", "s3cr3t-value"} {
		if strings.Contains(out, leaked) {
			t.Errorf("%q reached the terminal:\n%s", leaked, out)
		}
	}
	if !strings.Contains(out, "redacted") {
		t.Errorf("nothing was marked as redacted:\n%s", out)
	}
	// The raw view still shows the file as written — it is the author's own
	// file, and they can read it with any tool.
	raw, err := captureStdout(func() error {
		runConfig([]string{"view"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "hunter2") {
		t.Error("the raw view must show the file as it is on disk")
	}
}

// TestRedactSecretsLeavesOrdinarySettings: redaction is broad, not blind.
func TestRedactSecretsLeavesOrdinarySettings(t *testing.T) {
	got := string(redactSecrets([]byte("title: Example\njwt_secret: hunter2\ntoc_depth: 3\n")))
	if strings.Contains(got, "hunter2") {
		t.Errorf("secret survived:\n%s", got)
	}
	if !strings.Contains(got, "title: Example") || !strings.Contains(got, "toc_depth: 3") {
		t.Errorf("ordinary settings were taken too:\n%s", got)
	}
}
