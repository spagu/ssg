package generator

// Tests for #95 (fingerprinting a directory that still holds the last build's
// output) and #96 (building a derived collection in a theme).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
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

// --- #98: formatDate -------------------------------------------------------

// TestFormatDateFormats is the reported bug: Page.Date is a time.Time, so every
// call fell through to Sprintf("%v") and themes rendered Go's debug form.
func TestFormatDateFormats(t *testing.T) {
	d := time.Date(2017, 5, 13, 20, 36, 46, 0, time.UTC)

	if got := tmplFormatDate(d); got != "13 May 2017" {
		t.Errorf("default layout: got %q", got)
	}
	if got := tmplFormatDate(d, "2006-01-02"); got != "2017-05-13" {
		t.Errorf("explicit layout: got %q", got)
	}
	if got := tmplFormatDate(&d, "2006-01-02"); got != "2017-05-13" {
		t.Errorf("pointer: got %q", got)
	}
	if strings.Contains(tmplFormatDate(d), "+0000 UTC") {
		t.Error("still rendering Go's default time.Time string")
	}
}

// TestFormatDateEmptyForNoDate: a page without a date should show nothing, not
// a placeholder that looks like real data.
func TestFormatDateEmptyForNoDate(t *testing.T) {
	for name, in := range map[string]any{
		"zero time": time.Time{},
		"nil":       nil,
		"nil ptr":   (*time.Time)(nil),
	} {
		if got := tmplFormatDate(in); got != "" {
			t.Errorf("%s: got %q, want empty", name, got)
		}
	}
}

// TestFormatDatePassesStringsThrough keeps the one documented behaviour that
// always held, so a theme handing over a pre-formatted date is unaffected.
func TestFormatDatePassesStringsThrough(t *testing.T) {
	if got := tmplFormatDate("13 May 2017"); got != "13 May 2017" {
		t.Errorf("got %q", got)
	}
	if got := tmplFormatDate("13 May 2017", "2006-01-02"); got != "13 May 2017" {
		t.Errorf("a layout must not reinterpret a string: got %q", got)
	}
}

// --- #99: one name, one function -------------------------------------------

// TestTemplateFuncsHaveNoDuplicateNames is the structural guard. Two functions
// shared the name "related", the merge silently won, and the documented
// three-argument form became unreachable — every post then failed to render
// with an arity error while the build still reported success.
func TestTemplateFuncsHaveNoDuplicateNames(t *testing.T) {
	g := &Generator{config: Config{Quiet: true}, siteData: &models.SiteData{}}
	seen := map[string]bool{}
	for _, group := range []map[string]interface{}{
		g.imageFuncs(), g.taxonomyFuncs(), g.externalFuncs(), g.relatedFuncs(),
	} {
		for name := range group {
			if seen[name] {
				t.Errorf("helper %q is registered by two groups", name)
			}
			seen[name] = true
		}
	}
	// The literal must not collide with the merged groups either — that is the
	// exact shape of the reported bug.
	funcs := g.buildTemplateFuncs(nil)
	for name := range seen {
		if funcs[name] == nil {
			t.Errorf("helper %q vanished from the final map", name)
		}
	}
}

// TestRelatedFormsAreBothReachable: `related` keeps the two-argument behaviour
// that actually ran, and the collection form is reachable under its own name.
func TestRelatedFormsAreBothReachable(t *testing.T) {
	g := &Generator{config: Config{Quiet: true}, siteData: &models.SiteData{}}
	funcs := g.buildTemplateFuncs(nil)

	if funcs["related"] == nil || funcs["relatedIn"] == nil {
		t.Fatal("both related and relatedIn must be registered")
	}
	if _, ok := funcs["relatedIn"].(func(models.Page, int, any) ([]models.Page, error)); !ok {
		t.Errorf("relatedIn is not the three-argument collection form: %T", funcs["relatedIn"])
	}
	if _, ok := funcs["related"].(func(models.Page, int) []models.Page); !ok {
		t.Errorf("related is not the two-argument form that shipped: %T", funcs["related"])
	}
}

// --- #102: soft-404s and a status the host drops ---------------------------

// TestGenerateNotFound: without a 404.html a static host answers unmatched
// paths with index.html and a 200, so every dead URL reads as a live copy of
// the home page.
func TestGenerateNotFound(t *testing.T) {
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Domain: "example.com", Quiet: true}}
	if err := g.generateNotFound(); err != nil {
		t.Fatalf("generateNotFound: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(out, "404.html"))
	if err != nil {
		t.Fatalf("404.html not written: %v", err)
	}
	for _, want := range []string{"404", `name="robots" content="noindex"`, "example.com"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("404.html missing %q", want)
		}
	}
}

// TestGenerateNotFoundYieldsToTheSite: a page slugged "404" already renders to
// /404.html, and the generated fallback must not overwrite it.
func TestGenerateNotFoundYieldsToTheSite(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "404.html"), "<h1>the theme's own</h1>")
	g := &Generator{config: Config{OutputDir: out, Domain: "example.com", Quiet: true}}
	if err := g.generateNotFound(); err != nil {
		t.Fatalf("generateNotFound: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(out, "404.html"))
	if !strings.Contains(string(body), "the theme's own") {
		t.Error("the site's own 404.html was overwritten")
	}
}

// TestRedirect410WarnsOnlyForCloudflare: 410 is a Netlify extension. Cloudflare
// Pages drops it silently, so the rule reads as handled while the path keeps
// answering 200 — but warning about it on Netlify would be noise, since there
// it works.
func TestRedirect410WarnsOnlyForCloudflare(t *testing.T) {
	rules := []RedirectRule{{From: "/category/*", To: "/", Status: 410}}

	joined := strings.Join(validateRedirects(rules, "cloudflare"), "\n")
	if !strings.Contains(joined, "Cloudflare Pages ignores") {
		t.Errorf("cloudflare: no warning for a 410 rule:\n%s", joined)
	}

	for _, platform := range []string{"netlify", ""} {
		joined := strings.Join(validateRedirects(rules, platform), "\n")
		if strings.Contains(joined, "Cloudflare Pages ignores") {
			t.Errorf("platform %q: warned about a 410 that is valid there:\n%s", platform, joined)
		}
	}
}

// TestRedirect303IsAccepted: both hosts support it, and it was missing from the
// accepted set, so a legitimate rule drew an "unsupported status" warning.
func TestRedirect303IsAccepted(t *testing.T) {
	joined := strings.Join(validateRedirects([]RedirectRule{{From: "/a", To: "/b", Status: 303}}, "cloudflare"), "\n")
	if strings.Contains(joined, "unsupported status") {
		t.Errorf("303 reported as unsupported:\n%s", joined)
	}
}

// --- #103: what a page says about itself ------------------------------------

// TestServedCanonicalFollowsTheMode: pretty_urls fed link checking only, so a
// site on a host that strips extensions published canonical tags, og:url and a
// sitemap naming URLs that 308 — which is the one thing a canonical must not do.
func TestServedCanonicalFollowsTheMode(t *testing.T) {
	page := models.Page{Link: "/malta/marsaskala/batterija.html", Title: "B"}
	for _, tc := range []struct {
		mode models.PrettyURLMode
		want string
	}{
		{models.PrettyOff, "https://zonqor.com/malta/marsaskala/batterija.html"},
		{models.PrettyStrip, "https://zonqor.com/malta/marsaskala/batterija"},
		{models.PrettyStripSlash, "https://zonqor.com/malta/marsaskala/batterija/"},
	} {
		g := &Generator{config: Config{Domain: "zonqor.com", PrettyURLs: tc.mode, Quiet: true}}
		if got := g.servedCanonical(page); got != tc.want {
			t.Errorf("mode %q: got %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestServedURLLeavesForeignURLsAlone: only this site's own URLs are ours to
// normalise.
func TestServedURLLeavesForeignURLsAlone(t *testing.T) {
	g := &Generator{config: Config{Domain: "zonqor.com", PrettyURLs: models.PrettyStrip, Quiet: true}}
	external := "https://example.com/other/page.html"
	if got := g.servedURL(external); got != external {
		t.Errorf("rewrote an external URL: %q", got)
	}
}

// TestPrettyURLModeParsesBooleans keeps every config written before modes
// existed meaning exactly what it meant: true was strip-and-slash.
func TestPrettyURLModeParsesBooleans(t *testing.T) {
	for in, want := range map[string]models.PrettyURLMode{
		"true":        models.PrettyStripSlash,
		"strip-slash": models.PrettyStripSlash,
		"false":       models.PrettyOff,
		"off":         models.PrettyOff,
		"":            models.PrettyOff,
		"strip":       models.PrettyStrip,
	} {
		got, err := models.ParsePrettyURLMode(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if _, err := models.ParsePrettyURLMode("nonsense"); err == nil {
		t.Error("an unknown mode should be reported, not silently treated as off")
	} else if !strings.Contains(err.Error(), "--help") {
		t.Errorf("error should point at --help: %v", err)
	}
}

// TestRedirectTargetMatchesTheMode: under `strip` the host serves the bare
// form, so suggesting a trailing slash would name a URL it would itself
// redirect — the correction being wrong is worse than no correction.
func TestRedirectTargetMatchesTheMode(t *testing.T) {
	for _, tc := range []struct {
		mode models.PrettyURLMode
		want string
	}{
		{models.PrettyStripSlash, "/malta/window.html"},
		{models.PrettyStrip, "/malta/window.html"},
	} {
		g := &Generator{config: Config{Domain: "z.com", PrettyURLs: tc.mode, Quiet: true}}
		got := g.redirectTargetOf(tc.want)
		wantTarget := "/malta/window/"
		if tc.mode == models.PrettyStrip {
			wantTarget = "/malta/window"
		}
		if got != wantTarget {
			t.Errorf("mode %q: got %q, want %q", tc.mode, got, wantTarget)
		}
	}
}

// TestTrimHelpersRegistered: a theme could test for a suffix but not remove
// one, so stripping ".html" was impossible in a template.
func TestTrimHelpersRegistered(t *testing.T) {
	g := &Generator{config: Config{Quiet: true}, siteData: &models.SiteData{}}
	funcs := g.buildTemplateFuncs(nil)
	for _, name := range []string{"trimPrefix", "trimSuffix"} {
		if funcs[name] == nil {
			t.Errorf("%s is not registered", name)
		}
	}
}
