package generator

// A shortcode could not read the site's data, call a theme partial, or work on a
// collection — so the one thing a shortcode is for, placing a data-driven block
// where the author wants it, could not be done (#254).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortcodeThemeGen builds a generator with a real theme set — a partial and a
// shortcode template — the way loadTemplates leaves it.
func shortcodeThemeGen(t *testing.T, files map[string]string) *Generator {
	t.Helper()
	g := newTestGen(t, "")
	dir := t.TempDir()
	theme := filepath.Join(dir, "theme")
	if err := os.MkdirAll(filepath.Join(theme, "partials"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		path := filepath.Join(theme, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g.config.TemplatesDir = dir
	g.config.Template = "theme"
	if err := g.loadTemplates(); err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	return g
}

// TestShortcodeReadsSiteData: data/ files and external sources are site-wide,
// exactly like .Vars, and are admitted on the same reasoning.
func TestShortcodeReadsSiteData(t *testing.T) {
	g := shortcodeThemeGen(t, map[string]string{
		"index.html":   `x`,
		"reviews.html": `{{ range .SiteData.reviews }}{{ .name }};{{ end }}|{{ .ExternalData.places.total }}`,
	})
	g.data = map[string]interface{}{
		"reviews": []interface{}{
			map[string]interface{}{"name": "Ada"},
			map[string]interface{}{"name": "Grace"},
		},
	}
	g.externalData = map[string]interface{}{
		"places": map[string]interface{}{"total": 42},
	}

	out := g.renderShortcode(Shortcode{Name: "reviews", Template: "reviews.html"})
	if want := "Ada;Grace;|42"; out != want {
		t.Errorf("shortcode rendered %q, want %q", out, want)
	}
}

// TestShortcodeDataStaysTheEntrysOwnMap: `.Data` has meant the shortcodes:
// entry's `data:` map since shortcodes existed. Repointing it at the site's data
// files would break every theme using it, so the site data arrived under a name
// of its own.
func TestShortcodeDataStaysTheEntrysOwnMap(t *testing.T) {
	g := shortcodeThemeGen(t, map[string]string{
		"index.html": `x`,
		"sc.html":    `{{ .Data.cta }}`,
	})
	g.data = map[string]interface{}{"cta": "site-wide"}

	out := g.renderShortcode(Shortcode{
		Name: "sc", Template: "sc.html",
		Data: map[string]string{"cta": "entry-scoped"},
	})
	if out != "entry-scoped" {
		t.Errorf(".Data changed meaning: got %q", out)
	}
}

// TestShortcodeCallsAThemePartial: a block a page template and a shortcode both
// need had to be written twice.
func TestShortcodeCallsAThemePartial(t *testing.T) {
	g := shortcodeThemeGen(t, map[string]string{
		"index.html":         `x`,
		"partials/card.html": `{{ define "card" }}[card {{ . }}]{{ end }}`,
		"reviewblock.html":   `{{ template "card" .Attrs.who }}`,
	})

	out := g.renderShortcode(Shortcode{
		Name: "reviewblock", Template: "reviewblock.html",
		Attrs: map[string]string{"who": "Ada"},
	})
	if want := "[card Ada]"; out != want {
		t.Errorf("partial not reachable from a shortcode: got %q, want %q", out, want)
	}
}

// TestShortcodeCannotDamageTheThemeSet: a shortcode is parsed into a COPY of the
// theme namespace, so one that redefines a name cannot change what a page
// template renders.
func TestShortcodeCannotDamageTheThemeSet(t *testing.T) {
	g := shortcodeThemeGen(t, map[string]string{
		"index.html":         `x`,
		"partials/card.html": `{{ define "card" }}theme card{{ end }}`,
		"hijack.html":        `{{ define "card" }}hijacked{{ end }}{{ template "card" }}`,
	})

	if out := g.renderShortcode(Shortcode{Name: "hijack", Template: "hijack.html"}); out != "hijacked" {
		t.Errorf("the shortcode's own define should win inside it, got %q", out)
	}
	var sb strings.Builder
	if err := g.tmpl.ExecuteTemplate(&sb, "card", nil); err != nil {
		t.Fatalf("theme set: %v", err)
	}
	if sb.String() != "theme card" {
		t.Errorf("the shortcode overwrote the theme's partial: %q", sb.String())
	}
}

// TestShortcodeFiltersACollection: with data in scope, the reason for keeping
// the collection helpers out expired — and #253's conversion is what lets an
// attribute reach them.
func TestShortcodeFiltersACollection(t *testing.T) {
	g := shortcodeThemeGen(t, map[string]string{
		"index.html": `x`,
		"reviews.html": `{{ $r := filter "rating" "ge" (float .Attrs.min) .SiteData.reviews }}` +
			`{{ range first (int .Attrs.limit) (sort "name" "asc" $r) }}{{ .name }};{{ end }}`,
	})
	g.data = map[string]interface{}{
		"reviews": []interface{}{
			map[string]interface{}{"name": "Ada", "rating": 5},
			map[string]interface{}{"name": "Bob", "rating": 3},
			map[string]interface{}{"name": "Cleo", "rating": 5},
			map[string]interface{}{"name": "Dee", "rating": 4},
		},
	}

	out := g.renderShortcode(Shortcode{
		Name: "reviews", Template: "reviews.html",
		Attrs: map[string]string{"min": "4", "limit": "2"},
	})
	if want := "Ada;Cleo;"; out != want {
		t.Errorf("[reviews min=\"4\" limit=\"2\"] rendered %q, want %q", out, want)
	}
}

// TestShortcodeWithoutAThemeSetStillRenders: a bare generator — a test, an
// alt-engine theme — parses shortcodes standalone exactly as before.
func TestShortcodeWithoutAThemeSetStillRenders(t *testing.T) {
	g := newTestGen(t, "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sc.html"), []byte(`hi {{ .Name }} {{ int "2" }}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.config.TemplatesDir = dir
	g.config.Template = "."

	if out := g.renderShortcode(Shortcode{Name: "sc", Template: "sc.html"}); out != "hi sc 2" {
		t.Errorf("standalone shortcode rendered %q", out)
	}
}

// TestShortcodeBaseOfWithoutATheme: an alt-engine theme has no html/template
// set to copy, and shortcodes fall back to parsing standalone.
func TestShortcodeBaseOfWithoutATheme(t *testing.T) {
	if shortcodeBaseOf(nil, nil) != nil {
		t.Error("there is no namespace to copy when there is no theme set")
	}
	g := newTestGen(t, "")
	g.shortcodeBase = nil
	if g.shortcodeSet() != nil {
		t.Error("shortcodeSet must be nil without a base")
	}
}
