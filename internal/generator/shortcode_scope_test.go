package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #37: a shortcode template referencing $.Vars used to drop the whole
// shortcode from the page, leaving only a warning and an exit code of 0. Site
// variables are now in scope, and shortcode_errors decides what a failure
// leaves behind — with the historical "drop" as the default, so an existing
// site's output does not change.

// shortcodeGen builds a generator whose theme holds one shortcode template.
func shortcodeGen(t *testing.T, tmplBody, mode string, vars map[string]interface{}) *Generator {
	t.Helper()
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templateDir, 0o750); err != nil {
		t.Fatalf("creating template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "demo.html"), []byte(tmplBody), 0o600); err != nil {
		t.Fatalf("writing template: %v", err)
	}
	return &Generator{
		config: Config{
			TemplatesDir:      filepath.Join(tmpDir, "templates"),
			Template:          "test",
			Variables:         vars,
			ShortcodeErrors:   mode,
			ShortcodeBrackets: true,
		},
		shortcodeMap: map[string]Shortcode{
			"demo": {Name: "demo", Template: "demo.html", Data: map[string]string{"colour": "green"}},
		},
	}
}

// TestShortcodeTemplateSeesVars is the issue's headline case: $.Vars.key inside
// a shortcode template resolves instead of blowing the shortcode away.
func TestShortcodeTemplateSeesVars(t *testing.T) {
	vars := map[string]interface{}{"stripe_public_key": "pk_test_123"}
	gen := shortcodeGen(t, `<div data-key="{{$.Vars.stripe_public_key}}" data-colour="{{.Data.colour}}">hello</div>`, "", vars)

	got := gen.processShortcodesWith("{{demo}}", gen.renderShortcode)
	want := `<div data-key="pk_test_123" data-colour="green">hello</div>`
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
	if len(gen.shortcodeFailures) != 0 {
		t.Errorf("failures = %v, want none", gen.shortcodeFailures)
	}
	// The dot form must work too — both are bound to the same struct.
	gen2 := shortcodeGen(t, `<b>{{.Vars.stripe_public_key}}</b>`, "", vars)
	if got := gen2.processShortcodesWith("{{demo}}", gen2.renderShortcode); got != "<b>pk_test_123</b>" {
		t.Errorf(".Vars form rendered %q", got)
	}
	// An undefined variable stays empty rather than failing the shortcode:
	// Go templates resolve a missing map key to the zero value.
	gen3 := shortcodeGen(t, `<b>{{$.Vars.nope}}</b>`, "", vars)
	if got := gen3.processShortcodesWith("{{demo}}", gen3.renderShortcode); got != "<b></b>" {
		t.Errorf("missing var rendered %q, want an empty value", got)
	}
}

// TestShortcodeErrorModes pins the three failure modes, including the default
// that must stay byte-identical to the pre-#37 behaviour.
func TestShortcodeErrorModes(t *testing.T) {
	const bad = `{{$.NoSuchField.Nested}}`

	tests := []struct {
		mode    string
		content string
		want    string
	}{
		// Default and explicit "drop": the shortcode disappears, as before.
		{"", "Hello {{demo}} world", "Hello  world"},
		{"drop", "Hello {{demo}} world", "Hello  world"},
		{"unrecognised-mode", "Hello {{demo}} world", "Hello  world"},
		// keep/strict leave the source visible in the page.
		{"keep", "Hello {{demo}} world", "Hello {{demo}} world"},
		{"strict", "Hello {{demo}} world", "Hello {{demo}} world"},
		// Bracket forms keep their own raw source, attributes and all.
		{"keep", `a [demo key="v"] b`, `a [demo key="v"] b`},
		{"keep", "a [demo]inner[/demo] b", "a [demo]inner[/demo] b"},
		{"drop", `a [demo key="v"] b`, "a  b"},
	}

	for _, tc := range tests {
		gen := shortcodeGen(t, bad, tc.mode, nil)
		got := gen.processShortcodesWith(tc.content, gen.renderShortcode)
		if got != tc.want {
			t.Errorf("mode %q: rendered %q, want %q", tc.mode, got, tc.want)
		}
		if len(gen.shortcodeFailures) == 0 {
			t.Errorf("mode %q: no failure recorded", tc.mode)
		}
	}
}

// TestShortcodeErrorCheck covers the build-level verdict for each mode.
func TestShortcodeErrorCheck(t *testing.T) {
	const bad = `{{$.NoSuchField.Nested}}`

	for _, mode := range []string{"", "drop", "keep"} {
		gen := shortcodeGen(t, bad, mode, nil)
		gen.processShortcodesWith("{{demo}}", gen.renderShortcode)
		if err := gen.shortcodeErrorCheck(); err != nil {
			t.Errorf("mode %q: shortcodeErrorCheck = %v, want nil", mode, err)
		}
	}

	gen := shortcodeGen(t, bad, "strict", nil)
	gen.processShortcodesWith("{{demo}}", gen.renderShortcode)
	err := gen.shortcodeErrorCheck()
	if err == nil {
		t.Fatal("strict mode: shortcodeErrorCheck = nil, want a build error")
	}
	if !strings.Contains(err.Error(), "demo") {
		t.Errorf("strict error %q does not name the shortcode", err)
	}

	// Strict with nothing broken must not fail the build.
	ok := shortcodeGen(t, `<b>fine</b>`, "strict", nil)
	ok.processShortcodesWith("{{demo}}", ok.renderShortcode)
	if err := ok.shortcodeErrorCheck(); err != nil {
		t.Errorf("strict mode with no failures = %v, want nil", err)
	}
}

// TestShortcodeMissingTemplateModes covers the two non-execute failure paths
// (no template configured, template file absent) through the same modes.
func TestShortcodeMissingTemplateModes(t *testing.T) {
	base := shortcodeGen(t, `<b>unused</b>`, "keep", nil)

	// No template configured at all.
	base.shortcodeMap = map[string]Shortcode{"demo": {Name: "demo"}}
	if got := base.processShortcodesWith("x {{demo}} y", base.renderShortcode); got != "x {{demo}} y" {
		t.Errorf("no-template shortcode in keep mode = %q, want the raw source", got)
	}

	// Template configured but missing on disk.
	base.shortcodeMap = map[string]Shortcode{"demo": {Name: "demo", Template: "absent.html"}}
	base.shortcodeTmpls = nil
	if got := base.processShortcodesWith("x {{demo}} y", base.renderShortcode); got != "x {{demo}} y" {
		t.Errorf("missing-file shortcode in keep mode = %q, want the raw source", got)
	}
	if len(base.shortcodeFailures) != 2 {
		t.Errorf("failures = %d, want 2", len(base.shortcodeFailures))
	}
}
