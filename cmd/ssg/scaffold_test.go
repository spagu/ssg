package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAvailableWorkerTemplates(t *testing.T) {
	names := availableWorkerTemplates()
	want := map[string]bool{"contact-form": false, "stripe-checkout": false, "dynamic-price": false, "conversions-proxy": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("embedded worker template %q missing from %v", name, names)
		}
	}
}

func TestExtractWorkerTemplate(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "contact-form")
	if err := extractWorkerTemplate("workers/contact-form", dest); err != nil {
		t.Fatalf("extractWorkerTemplate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "functions", "api", "contact.ts")); err != nil {
		t.Fatalf("expected the function file to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("expected the README to be extracted: %v", err)
	}
}

func TestRunNewWorker_UnknownTemplate(t *testing.T) {
	if code := runNewWorker([]string{"does-not-exist"}); code != 1 {
		t.Fatalf("unknown template should exit 1, got %d", code)
	}
}

func TestRunNewWorker_NoArgs(t *testing.T) {
	if code := runNewWorker(nil); code != 2 {
		t.Fatalf("missing template should exit 2, got %d", code)
	}
}

func TestRunNewWorker_ScaffoldsIntoCwd(t *testing.T) {
	tmp := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	if code := runNewWorker([]string{"dynamic-price"}); code != 0 {
		t.Fatalf("scaffold should succeed, got exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(tmp, "workers", "dynamic-price", "README.md")); err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}
	// A second run must refuse to overwrite.
	if code := runNewWorker([]string{"dynamic-price"}); code != 1 {
		t.Fatalf("re-scaffold should refuse, got exit %d", code)
	}
}

func TestWorkerConfigSnippet(t *testing.T) {
	snip := workerConfigSnippet("contact-form")
	if !strings.Contains(snip, "dir: workers/contact-form") || !strings.Contains(snip, "routes_include") {
		t.Fatalf("snippet missing worker config:\n%s", snip)
	}
}

// TestTheRateLimitTemplateShipsWhatItPromises (#220).
//
// The other templates are presented as ready to deploy, and one thing was
// missing from all of them: `ssg new worker contact-form` gives you a public,
// unauthenticated POST that sends email, with nothing bounding how often it can
// be called. This template is the deployed counterpart of the `rate_limit` that
// only ever covered the preview server.
func TestTheRateLimitTemplateShipsWhatItPromises(t *testing.T) {
	if !slices.Contains(availableWorkerTemplates(), "rate-limit") {
		t.Fatal("rate-limit is not offered by `ssg new worker`")
	}
	dest := filepath.Join(t.TempDir(), "rate-limit")
	if err := extractWorkerTemplate("workers/rate-limit", dest); err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Middleware, not a library: it has to wrap routes the project adds later,
	// including ones from another template scaffolded beside it.
	middleware := mustReadFile(t, filepath.Join(dest, "functions", "_middleware.ts"))
	if !strings.Contains(middleware, "export const onRequest") {
		t.Error("the template must export middleware, or it wraps nothing")
	}

	// The guarantees the report asked to have pinned down. Each is a decision
	// someone would otherwise have to rediscover by being caught by it.
	for _, want := range []struct{ needle, why string }{
		{"retry-after", "a 429 without Retry-After makes a well-behaved client retry immediately"},
		{"no-store", "a cached 429 answers somebody else's request"},
		{"RATE_LIMITER", "the exact, free binding must be the default backend"},
		{"RATE_LIMIT_FAIL", "fail open suits a contact form and not a checkout, so it must be settable"},
		{"header:", "cf-connecting-ip buckets a whole office together; a site with something better must be able to say so"},
	} {
		if !strings.Contains(middleware, want.needle) {
			t.Errorf("missing %q — %s", want.needle, want.why)
		}
	}

	// A project with no backend bound must behave exactly as it did. A limiter
	// that turns visitors away because nobody finished configuring it is worse
	// than no limiter.
	if !strings.Contains(middleware, "return null") || !strings.Contains(middleware, "allowed === null") {
		t.Error("an unconfigured project must pass through untouched")
	}

	// The KV path must not count a request it is rejecting: otherwise a bot
	// locks a real visitor out of a shared address indefinitely.
	put := strings.Index(middleware, "RATE_LIMIT_KV.put")
	over := strings.Index(middleware, "used >= max")
	if put < 0 || over < 0 || over > put {
		t.Error("the KV backend must refuse before it counts, not after")
	}

	// And the README has to say what the KV weakness is, because choosing it
	// unknowingly is how a cap gets overshot by the burst it was meant to stop.
	// Matched on collapsed whitespace: the prose is hard-wrapped, so asserting
	// on a phrase would break whenever a paragraph is rewrapped — a test failing
	// for the width of a line teaches people to stop reading it.
	readme := strings.Join(strings.Fields(mustReadFile(t, filepath.Join(dest, "README.md"))), " ")
	if !strings.Contains(readme, "eventually consistent") {
		t.Error("the README must state why KV is a fallback rather than a choice")
	}
	if !strings.Contains(readme, "fallback, not a choice") {
		t.Error("the README must not present KV as an equal option")
	}
}

// mustReadFile reads a scaffolded file or fails the test.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}
