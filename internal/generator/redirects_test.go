package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRedirects_DefaultsAndTrim(t *testing.T) {
	got := normalizeRedirects([]RedirectRule{
		{From: "  /a  ", To: "  /b  "},
		{From: "", To: "/x"},
		{From: "/c", To: "/d", Status: 302},
	})
	if len(got) != 2 {
		t.Fatalf("empty From should be dropped, got %d rules", len(got))
	}
	if got[0].From != "/a" || got[0].To != "/b" || got[0].Status != 301 {
		t.Fatalf("normalize failed: %+v", got[0])
	}
	if got[1].Status != 302 {
		t.Fatalf("explicit status overwritten: %+v", got[1])
	}
}

func TestFlattenRedirectChains_ResolvesToFinalTarget(t *testing.T) {
	rules := []RedirectRule{
		{From: "/a", To: "/b", Status: 301},
		{From: "/b", To: "/c", Status: 301},
		{From: "/c", To: "/final", Status: 301},
	}
	got, err := flattenRedirectChains(rules)
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if got[0].To != "/final" {
		t.Fatalf("expected /a to flatten to /final, got %q", got[0].To)
	}
	if got[1].To != "/final" {
		t.Fatalf("expected /b to flatten to /final, got %q", got[1].To)
	}
}

func TestFlattenRedirectChains_DetectsCycle(t *testing.T) {
	rules := []RedirectRule{
		{From: "/a", To: "/b", Status: 301},
		{From: "/b", To: "/a", Status: 301},
	}
	if _, err := flattenRedirectChains(rules); err == nil {
		t.Fatal("expected a cycle error")
	}
}

func TestFlattenRedirectChains_LeavesWildcards(t *testing.T) {
	rules := []RedirectRule{
		{From: "/old/*", To: "/new/:splat", Status: 301},
		{From: "/new/:splat", To: "/x", Status: 301},
	}
	got, err := flattenRedirectChains(rules)
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if got[0].To != "/new/:splat" {
		t.Fatalf("wildcard rule should be untouched, got %q", got[0].To)
	}
}

func TestValidateRedirects_Warnings(t *testing.T) {
	warnings := validateRedirects([]RedirectRule{
		{From: "/dup", To: "/a", Status: 301},
		{From: "/dup", To: "/b", Status: 301},
		{From: "/bad", To: "/c", Status: 418},
		{From: "/x", To: "/y/:splat", Status: 301},
	})
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"duplicate redirect source", "unsupported status 418", ":splat in destination but has no *"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing warning %q in:\n%s", want, joined)
		}
	}
}

func TestValidateRedirects_ShadowingAndCaps(t *testing.T) {
	rules := []RedirectRule{
		{From: "/blog/*", To: "/articles/:splat", Status: 301},
		{From: "/blog/post-1", To: "/articles/post-1", Status: 301},
	}
	warnings := validateRedirects(rules)
	if !strings.Contains(strings.Join(warnings, "\n"), "shadows later rule") {
		t.Fatalf("expected a shadowing warning, got: %v", warnings)
	}

	many := make([]RedirectRule, cfMaxStaticRedirects+1)
	for i := range many {
		many[i] = RedirectRule{From: "/p" + itoa(i), To: "/q", Status: 301}
	}
	if !strings.Contains(strings.Join(validateRedirects(many), "\n"), "exceed the Cloudflare Pages limit") {
		t.Fatal("expected a static-cap warning")
	}
}

func TestRenderRedirectsFile_ExactBeforeDynamicAndForce(t *testing.T) {
	rules := []RedirectRule{
		{From: "/old/*", To: "/new/:splat", Status: 301},
		{From: "/a", To: "/b", Status: 308, Force: true},
		{From: "/gone", To: "", Status: 410},
	}
	out := renderRedirectsFile(rules)
	// Exact rules render before wildcard rules regardless of input order.
	aIdx := strings.Index(out, "/a /b 308!")
	wIdx := strings.Index(out, "/old/* /new/:splat 301")
	if aIdx < 0 || wIdx < 0 || aIdx > wIdx {
		t.Fatalf("ordering/force wrong:\n%s", out)
	}
	if !strings.Contains(out, "/gone / 410") {
		t.Fatalf("410 rule with empty target should render `/`:\n%s", out)
	}
}

func TestCollectRedirects_MergesAliasesAndConfigWins(t *testing.T) {
	dir := t.TempDir()
	// A real target so the existence check passes.
	if err := os.MkdirAll(filepath.Join(dir, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new", "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &Generator{
		config: Config{
			OutputDir: dir,
			Redirects: []RedirectRule{{From: "/explicit", To: "/new", Status: 301}},
		},
		aliasRedirects: []RedirectRule{
			{From: "/alias", To: "/new", Status: 301},
			{From: "/explicit", To: "/somewhere-else", Status: 301}, // overridden
		},
	}
	rules, warnings, err := g.collectRedirects()
	if err != nil {
		t.Fatalf("collectRedirects: %v", err)
	}
	var haveExplicit, haveAlias bool
	for _, r := range rules {
		if r.From == "/explicit" {
			haveExplicit = true
			if r.To != "/new" {
				t.Fatalf("config rule should win, got %q", r.To)
			}
		}
		if r.From == "/alias" {
			haveAlias = true
		}
	}
	if !haveExplicit || !haveAlias {
		t.Fatalf("missing merged rules: %+v", rules)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "overridden by an explicit") {
		t.Fatalf("expected override warning, got: %v", warnings)
	}
}

func TestGenerateRedirectsFile_CycleErrors(t *testing.T) {
	dir := t.TempDir()
	g := &Generator{config: Config{
		OutputDir: dir,
		Redirects: []RedirectRule{
			{From: "/a", To: "/b", Status: 301},
			{From: "/b", To: "/a", Status: 301},
		},
	}}
	if err := g.generateRedirectsFile(); err == nil {
		t.Fatal("expected a cycle error to propagate")
	}
}

func TestValidateRedirects_EmptyDestAndDynamicCap(t *testing.T) {
	warnings := validateRedirects([]RedirectRule{{From: "/a", To: "", Status: 302}})
	if !strings.Contains(strings.Join(warnings, "\n"), "empty destination") {
		t.Fatalf("expected empty-destination warning, got %v", warnings)
	}
	many := make([]RedirectRule, cfMaxDynamicRedirects+1)
	for i := range many {
		many[i] = RedirectRule{From: "/p" + itoa(i) + "/*", To: "/q/:splat", Status: 301}
	}
	if !strings.Contains(strings.Join(validateRedirects(many), "\n"), "dynamic redirects exceed") {
		t.Fatal("expected a dynamic-cap warning")
	}
}

func TestCollectRedirects_WarnsMissingTarget(t *testing.T) {
	dir := t.TempDir()
	g := &Generator{config: Config{
		OutputDir: dir,
		Redirects: []RedirectRule{{From: "/a", To: "/nonexistent", Status: 301}},
	}}
	_, warnings, err := g.collectRedirects()
	if err != nil {
		t.Fatalf("collectRedirects: %v", err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "does not exist in the output") {
		t.Fatalf("expected a missing-target warning, got %v", warnings)
	}
}

func TestGenerateRedirectsFile_WritesFile(t *testing.T) {
	dir := t.TempDir()
	g := &Generator{config: Config{
		OutputDir: dir,
		Redirects: []RedirectRule{{From: "/a", To: "/b", Status: 301}},
	}}
	if err := g.generateRedirectsFile(); err != nil {
		t.Fatalf("generateRedirectsFile: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "_redirects"))
	if err != nil {
		t.Fatalf("reading _redirects: %v", err)
	}
	if !strings.Contains(string(data), "/a /b 301") {
		t.Fatalf("unexpected _redirects:\n%s", data)
	}
}

// itoa avoids strconv import churn in the cap test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
