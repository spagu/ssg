package config

import (
	"strings"
	"testing"
)

// GO-183 (security review of designer-ui): the boundary between
// designer-ui:settings and code that runs in CI or in every visitor's browser
// is the class of a key. These keys were "plain" and are not.

// TestSchemaFileReadingKeysExecute: a key naming a file the build reads and
// renders into the public site must be "executes". ssg resolves these paths
// without confinement (outputs_custom and render_hooks accept absolute paths,
// a shortcode template joins "../" freely), so a plain class let a holder of
// designer-ui:settings publish e.g. /proc/self/environ or .git/config (which
// holds the checkout token) as a page of the site.
func TestSchemaFileReadingKeysExecute(t *testing.T) {
	s := BuildSchema()
	for _, path := range []string{"shortcodes[].template", "render_hooks", "outputs_custom[].template"} {
		n := s.Node(path)
		if n == nil {
			t.Fatalf("%s is not in the schema", path)
		}
		if n.Class != ClassExecutes {
			t.Errorf("%s class = %q, want %q: it names a file the build reads into the public output", path, n.Class, ClassExecutes)
		}
	}
}

// TestSchemaThirdPartyScriptKeysExecute: analytics ids load a vendor's script
// (a tag-manager container is arbitrary JavaScript chosen by whoever owns the
// id) on every page, at the origin that also serves the admin panels and the
// auth worker, so they need the same trust as a theme edit.
func TestSchemaThirdPartyScriptKeysExecute(t *testing.T) {
	s := BuildSchema()
	for _, path := range []string{"analytics", "analytics_ids"} {
		if n := s.Node(path); n == nil || n.Class != ClassExecutes {
			t.Errorf("%s must be %q (it decides which third-party script every visitor runs)", path, ClassExecutes)
		}
	}
}

// TestSchemaVisitorRedirectingKeysExecute (SR-10, owner decision 2026-11-15):
// keys that decide where a visitor's request or a link ends up — _redirects
// rules (which the host may apply before the auth worker's login routes),
// link rewriting, the site's own domain (canonical URLs, feeds, sitemap) and
// the base path the shared navigation sends administrators to — can send
// people to a host the writer chose, so they need designer-ui:advanced.
// None of them is writable through `ssg mcp` designer_config_set.
func TestSchemaVisitorRedirectingKeysExecute(t *testing.T) {
	s := BuildSchema()
	for _, path := range []string{"redirects", "link_rewrites", "domain", "modules[].url"} {
		n := s.Node(path)
		if n == nil {
			t.Fatalf("%s is not in the schema", path)
		}
		if n.Class != ClassExecutes {
			t.Errorf("%s class = %q, want %q: it can send visitors elsewhere", path, n.Class, ClassExecutes)
		}
		if n.MCPWritable {
			t.Errorf("%s is writable through ssg mcp; only plain keys may be", path)
		}
	}
}

// TestOverlayTemplateIsOneName: the theme is <templates_dir>/<template>, and
// ssg copies its css/, js/ and images/ into the public output, so `template`
// written through the overlay is one folder name — never a path that leaves
// templates_dir (templates_dir itself is "executes").
func TestOverlayTemplateIsOneName(t *testing.T) {
	for _, bad := range []string{"../../.git", "..", ".", "a/b", `a\b`, "/etc", "x/../../y"} {
		err := ValidateOverlay(map[string]any{"template": bad})
		if err == nil || !strings.Contains(err.Error(), "template") {
			t.Errorf("template %q accepted: %v", bad, err)
		}
	}
	for _, good := range []string{"ssgtheme", "my-theme.v2", ""} {
		if err := ValidateOverlay(map[string]any{"template": good}); err != nil {
			t.Errorf("template %q refused: %v", good, err)
		}
	}
}
