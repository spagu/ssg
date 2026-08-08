package generator

// Tests for #95 (fingerprinting a directory that still holds the last build's
// output) and #96 (building a derived collection in a theme).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fingerprintFixture returns a generator writing into a fresh output directory
// holding one stylesheet.
func fingerprintFixture(t *testing.T) *Generator {
	t.Helper()
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "css", "style.css"), "body{color:red}")
	return &Generator{config: Config{OutputDir: out, Fingerprint: true, Quiet: true}}
}

// cssNames lists the stylesheet names in the output, for comparing builds.
func cssNames(t *testing.T, out string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(out, "css"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestFingerprintRebuildIsStable is the reported bug (#95): a second build into
// a directory that was not cleaned used to hash its own previous output, giving
// style.<hash>.<hash>.css alongside a surviving style.<hash>.css.
func TestFingerprintRebuildIsStable(t *testing.T) {
	g := fingerprintFixture(t)
	out := g.config.OutputDir

	if err := g.fingerprintAssets(); err != nil {
		t.Fatalf("first build: %v", err)
	}
	first := cssNames(t, out)
	if len(first) != 1 || !strings.HasPrefix(first[0], "style.") {
		t.Fatalf("first build produced %v, want one hashed stylesheet", first)
	}

	// A rebuild copies the source asset in again, exactly as a real build does,
	// while the previous build's output is still present.
	mustWrite(t, filepath.Join(out, "css", "style.css"), "body{color:red}")
	if err := g.fingerprintAssets(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	second := cssNames(t, out)
	if len(second) != 1 {
		t.Fatalf("rebuild produced %v, want exactly one stylesheet", second)
	}
	if second[0] != first[0] {
		t.Errorf("rebuild renamed the asset: %s → %s", first[0], second[0])
	}
	if strings.Count(second[0], ".") != 2 {
		t.Errorf("double-hashed name %q", second[0])
	}
}

// TestFingerprintRemovesSupersededOutput covers the disk leak in the same
// report: without this the directory keeps every historical hash forever.
func TestFingerprintRemovesSupersededOutput(t *testing.T) {
	g := fingerprintFixture(t)
	out := g.config.OutputDir
	if err := g.fingerprintAssets(); err != nil {
		t.Fatalf("first build: %v", err)
	}
	old := cssNames(t, out)[0]

	// Different content this time, so the hash — and the name — must change.
	mustWrite(t, filepath.Join(out, "css", "style.css"), "body{color:blue}")
	if err := g.fingerprintAssets(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	names := cssNames(t, out)
	if len(names) != 1 {
		t.Fatalf("got %v, want only the current stylesheet", names)
	}
	if names[0] == old {
		t.Errorf("changed content kept the old name %q", old)
	}
}

// TestFingerprintKeepsLookalikeAsset guards the fix against its own failure
// mode. Without a manifest the only signal is the name, and name.<8hex>.ext is
// a shape a theme may legitimately ship — so an unrecognised match must be left
// alone rather than deleted, or the fix destroys a real asset every build.
func TestFingerprintKeepsLookalikeAsset(t *testing.T) {
	out := t.TempDir()
	lookalike := filepath.Join(out, "js", "app.deadbeef.js")
	mustWrite(t, lookalike, "console.log(1)")
	g := &Generator{config: Config{OutputDir: out, Fingerprint: true, Quiet: true}}

	js, _, err := g.collectFingerprintAssets()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if _, err := os.Stat(lookalike); err != nil {
		t.Fatalf("a source asset that merely looks fingerprinted was deleted: %v", err)
	}
	for _, p := range js {
		if p == lookalike {
			t.Errorf("%s was queued for hashing, which would double-hash it", lookalike)
		}
	}
}

// TestPreviousFingerprintsTolerantOfBadManifest: a missing or corrupt manifest
// must not fail the build, only fall back to the name check.
func TestPreviousFingerprintsTolerantOfBadManifest(t *testing.T) {
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out}}
	if got := g.previousFingerprints(); got != nil {
		t.Errorf("missing manifest: got %v, want nil", got)
	}
	mustWrite(t, filepath.Join(out, "assets-manifest.json"), "{not json")
	if got := g.previousFingerprints(); got != nil {
		t.Errorf("corrupt manifest: got %v, want nil", got)
	}
	body, _ := json.Marshal(map[string]string{"css/style.css": "css/style.abcdef12.css"})
	mustWrite(t, filepath.Join(out, "assets-manifest.json"), string(body))
	if got := g.previousFingerprints(); !got["css/style.abcdef12.css"] {
		t.Errorf("valid manifest not read: %v", got)
	}
}

// --- #96: append -----------------------------------------------------------

func TestAppendAcceptsEitherArgumentOrder(t *testing.T) {
	// {{ $kids = append $kids . }} — Go's own append(slice, elems...)
	got, err := tmplAppend([]string{"a"}, "b", "c")
	if err != nil {
		t.Fatalf("collection first: %v", err)
	}
	if s, ok := got.([]string); !ok || strings.Join(s, ",") != "a,b,c" {
		t.Errorf("collection first: %#v", got)
	}

	// {{ $kids = $kids | append . }} — this file's pipeline convention, which
	// hands the collection over as the final argument.
	got, err = tmplAppend("b", []string{"a"})
	if err != nil {
		t.Fatalf("piped: %v", err)
	}
	if s, ok := got.([]string); !ok || strings.Join(s, ",") != "a,b" {
		t.Errorf("piped: %#v", got)
	}
}

// TestAppendDoesNotMutateInput: reflect.Append may write into spare capacity,
// so two templates appending to the same base would otherwise clobber one
// another.
func TestAppendDoesNotMutateInput(t *testing.T) {
	base := make([]string, 1, 8)
	base[0] = "a"

	first, err := tmplAppend(base, "b")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	second, err := tmplAppend(base, "c")
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	if len(base) != 1 || base[0] != "a" {
		t.Errorf("input mutated: %#v", base)
	}
	if got := first.([]string)[1]; got != "b" {
		t.Errorf("first result overwritten: %q", got)
	}
	if got := second.([]string)[1]; got != "c" {
		t.Errorf("second result wrong: %q", got)
	}
}

func TestAppendWidensOnMixedTypes(t *testing.T) {
	got, err := tmplAppend([]string{"a"}, 42)
	if err != nil {
		t.Fatalf("mixed types should widen, not fail: %v", err)
	}
	s, ok := got.([]any)
	if !ok || len(s) != 2 || s[1] != 42 {
		t.Errorf("got %#v, want []any{\"a\", 42}", got)
	}
}

func TestAppendRejectsNonCollections(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []any
	}{
		{"too few arguments", []any{[]string{"a"}}},
		{"no collection at either end", []any{"a", "b"}},
	} {
		if _, err := tmplAppend(tc.args...); err == nil {
			t.Errorf("%s: want an error", tc.name)
		} else if !strings.Contains(err.Error(), "--help") {
			t.Errorf("%s: error should point at --help, got %q", tc.name, err)
		}
	}
}

// --- #96: string operators for filter --------------------------------------

// TestFilterStringOperators covers the case from the report: selecting the
// sub-pages of a section. `contains` cannot express it, because it also matches
// /not-special/.
func TestFilterStringOperators(t *testing.T) {
	type page struct {
		Link  string
		Title string
	}
	pages := []page{
		{"/special/", "Special"},
		{"/special/dog-agility/", "Dog agility"},
		{"/not-special/thing/", "Decoy"},
		{"/baby-water-instructor/", "Baby water"},
	}

	got, err := tmplFilter("Link", "hasPrefix", "/special/", pages)
	if err != nil {
		t.Fatalf("hasPrefix: %v", err)
	}
	if n := len(got.([]page)); n != 2 {
		t.Errorf("hasPrefix matched %d pages, want 2 (the decoy must not match)", n)
	}

	got, err = tmplFilter("Link", "hasSuffix", "agility/", pages)
	if err != nil {
		t.Fatalf("hasSuffix: %v", err)
	}
	if n := len(got.([]page)); n != 1 {
		t.Errorf("hasSuffix matched %d, want 1", n)
	}

	got, err = tmplFilter("Title", "matches", "^Baby ", pages)
	if err != nil {
		t.Fatalf("matches: %v", err)
	}
	if n := len(got.([]page)); n != 1 {
		t.Errorf("matches matched %d, want 1", n)
	}
}

// TestFilterStringOperatorsReportMisuse: applying a text test to a list field
// is a mistake, and answering false would hide it the way `contains` does.
func TestFilterStringOperatorsReportMisuse(t *testing.T) {
	type page struct{ Tags []string }
	_, err := tmplFilter("Tags", "hasPrefix", "go", []page{{Tags: []string{"golang"}}})
	if err == nil {
		t.Fatal("want an error for a prefix test on a []string field")
	}
	if !strings.Contains(err.Error(), "--help") {
		t.Errorf("error should point at --help, got %q", err)
	}
}

// TestFilterUnknownOperatorListsTheNewOnes keeps the error message honest: it
// is the only place a template author learns what is available.
func TestFilterUnknownOperatorListsTheNewOnes(t *testing.T) {
	type page struct{ Link string }
	_, err := tmplFilter("Link", "startsWith", "/x/", []page{{Link: "/x/"}})
	if err == nil {
		t.Fatal("want an error for an unknown operator")
	}
	for _, op := range []string{"hasPrefix", "hasSuffix", "matches"} {
		if !strings.Contains(err.Error(), op) {
			t.Errorf("error does not mention %q: %s", op, err)
		}
	}
}
