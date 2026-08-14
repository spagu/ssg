package migrate

// Credentials and navigation (#132), and the theme's own post types (#130).
// WordPress refuses /wp/v2/menus to an anonymous caller, so a public export
// comes back without navigation while reporting success — the migration has to
// carry credentials and, when there are none, say why the site has no menu.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func argsFor(t *testing.T, opts Options) string {
	t.Helper()
	opts.Dest = "content/x"
	args, _, err := wpexporterArgs("https://e.com", opts)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(args, " ")
}

func TestAuthArgsForwarded(t *testing.T) {
	got := argsFor(t, Options{AuthUser: "editor", AuthPass: "s3cret"})
	if !strings.Contains(got, "--auth-user editor") || !strings.Contains(got, "--auth-pass s3cret") {
		t.Fatalf("basic auth not forwarded: %q", got)
	}
	// A token stands in for the pair, the way an Authorization header would.
	got = argsFor(t, Options{AuthToken: "tok", AuthUser: "editor", AuthPass: "s3cret"})
	if !strings.Contains(got, "--auth-token tok") || strings.Contains(got, "--auth-user") {
		t.Fatalf("token must win over the pair: %q", got)
	}
	// Nothing configured, nothing passed.
	if got = argsFor(t, Options{}); strings.Contains(got, "--auth") {
		t.Fatalf("no credentials must add no flags: %q", got)
	}
}

func TestCustomTypeArgs(t *testing.T) {
	got := argsFor(t, Options{CustomTypes: []string{"cpt_services", "cpt_team"}})
	if !strings.Contains(got, "--custom-types cpt_services,cpt_team") {
		t.Fatalf("custom types not selected: %q", got)
	}
	if got = argsFor(t, Options{NoCustomTypes: true}); !strings.Contains(got, "--no-custom-types") {
		t.Fatalf("exclusion not forwarded: %q", got)
	}
	// Excluding wins over a selection: the operator said no.
	got = argsFor(t, Options{NoCustomTypes: true, CustomTypes: []string{"cpt_services"}})
	if strings.Contains(got, "--custom-types cpt") {
		t.Fatalf("exclusion must not be undone by a selection: %q", got)
	}
	if got = argsFor(t, Options{}); strings.Contains(got, "custom-types") {
		t.Fatalf("the default must not touch custom types: %q", got)
	}
}

// writeMenus puts a metadata.json holding n menus into dest.
func writeMenus(t *testing.T, dest string, n int) {
	t.Helper()
	menus := make([]map[string]any, n)
	for i := range menus {
		menus[i] = map[string]any{"id": i + 1, "name": "Main", "slug": "main"}
	}
	raw, err := json.Marshal(map[string]any{"menus": menus})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "metadata.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCountMenus(t *testing.T) {
	dest := t.TempDir()
	if got := countMenus(dest); got != 0 { // no metadata.json at all
		t.Fatalf("missing metadata = %d", got)
	}
	if err := os.WriteFile(filepath.Join(dest, "metadata.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countMenus(dest); got != 0 {
		t.Fatalf("unreadable metadata = %d, want 0 (never a failure)", got)
	}
	writeMenus(t, dest, 3)
	if got := countMenus(dest); got != 3 {
		t.Fatalf("countMenus = %d, want 3", got)
	}
}

// TestMenusWarning: the absence of navigation is explained, and the advice
// matches what the run actually did.
func TestMenusWarning(t *testing.T) {
	if w := menusWarning(0, Options{}); !strings.Contains(w, "--auth-user") ||
		!strings.Contains(w, "edit_theme_options") {
		t.Fatalf("an anonymous run must name the cause and the fix: %q", w)
	}
	w := menusWarning(0, Options{AuthUser: "editor"})
	if !strings.Contains(w, "even with credentials") {
		t.Fatalf("an authenticated run needs different advice: %q", w)
	}
	if w := menusWarning(2, Options{}); w != "" {
		t.Fatalf("menus present → no warning, got %q", w)
	}
	// Asked for no menus: their absence is the answer, not a problem.
	if w := menusWarning(0, Options{Content: []string{"pages", " NO-MENUS "}}); w != "" {
		t.Fatalf("an explicit exclusion must not warn: %q", w)
	}
}

// TestFetchReportsMenus: the count and the warning reach the report the
// operator reads, which is the whole point — a site that comes up unnavigable
// with a clean summary is the failure this closes.
func TestFetchReportsMenus(t *testing.T) {
	p, _ := Lookup("wordpress")
	dest := t.TempDir()

	rep, err := p.Fetch("https://e.com", Options{
		Dest:     dest,
		LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
		Run:      func(string, []string, bool) error { return nil }, // exports nothing
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Menus != 0 || len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "authentication") {
		t.Fatalf("an anonymous run must report why there is no navigation: %+v", rep)
	}

	rep, err = p.Fetch("https://e.com", Options{
		Dest:     dest,
		AuthUser: "editor",
		AuthPass: "s3cret",
		LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
		Run: func(_ string, args []string, _ bool) error {
			if !strings.Contains(strings.Join(args, " "), "--auth-user editor") {
				t.Errorf("credentials never reached the engine: %v", args)
			}
			writeMenus(t, dest, 2)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Menus != 2 || len(rep.Warnings) != 0 {
		t.Fatalf("menus present must be counted and unwarned: %+v", rep)
	}
}
