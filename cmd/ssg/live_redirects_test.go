package main

// The preview serves the `_redirects` it generates (#181).

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// resetLiveRules clears the published tables between tests.
func resetLiveRules(t *testing.T) {
	t.Helper()
	liveRedirects.Store(nil)
	liveHeaders.Store(nil)
	t.Cleanup(func() {
		liveRedirects.Store(nil)
		liveHeaders.Store(nil)
	})
}

// quietWarn collects the warnings a parse produced.
func quietWarn(into *[]string) func(string, ...any) {
	return func(f string, a ...any) { *into = append(*into, sprintfLine(f, a...)) }
}

// publishRules parses text and mounts a handler over a stand-in file server.
func publishRules(t *testing.T, text string) (http.Handler, []string) {
	t.Helper()
	var warnings []string
	table := parseRedirectsFile(text, quietWarn(&warnings))
	liveRedirects.Store(&table)
	return liveRedirectHandler(staticSaying("static")), warnings
}

// TestAnExactRedirectIsAnswered: the migration case — a URL somebody else
// published, carried over from the old CMS, answered 404 by the preview until
// production proved whether the rule was right.
func TestAnExactRedirectIsAnswered(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/old-page/ /new-page/ 301\n")

	rec := getPath(h, "/old-page/")
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/new-page/" {
		t.Errorf("Location = %q", got)
	}
}

// TestAStatusOtherThan301IsHonoured: a rule that says 302 must not be served
// as a 301 — the difference is whether a browser ever asks again.
func TestAStatusOtherThan301IsHonoured(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/a /b 302\n/c /d 307\n/e /f 308\n/g /h 303\n")
	for path, want := range map[string]int{
		"/a": http.StatusFound, "/c": http.StatusTemporaryRedirect,
		"/e": http.StatusPermanentRedirect, "/g": http.StatusSeeOther,
	} {
		if got := getPath(h, path).Code; got != want {
			t.Errorf("%s = %d, want %d", path, got, want)
		}
	}
}

// TestADefaultedStatusIs301: the generator writes a status on every line, but a
// hand-edited file may not, and 301 is what the format defaults to.
func TestADefaultedStatusIs301(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/old /new\n")
	if got := getPath(h, "/old").Code; got != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", got)
	}
}

// TestASplatCarriesTheRemainderIntoTheDestination: `/blog/* /articles/:splat`,
// the rule a whole section move produces.
func TestASplatCarriesTheRemainderIntoTheDestination(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/blog/* /articles/:splat 301\n")

	if got := getPath(h, "/blog/2014/05/hello/").Header().Get("Location"); got != "/articles/2014/05/hello/" {
		t.Errorf("Location = %q", got)
	}
	// A splat that captures nothing still matches, as it does on the platform.
	if got := getPath(h, "/blog/").Header().Get("Location"); got != "/articles/" {
		t.Errorf("empty splat Location = %q", got)
	}
	if rec := getPath(h, "/elsewhere/"); rec.Body.String() != "static" {
		t.Errorf("an unrelated path must fall through: %q", rec.Body)
	}
}

// TestAPlaceholderCapturesOneSegment: the `:placeholder` form the generator's
// own RedirectRule documents, so the preview does not diverge from what the
// build was willing to write.
func TestAPlaceholderCapturesOneSegment(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/news/:year/:slug /:year/posts/:slug 301\n")

	if got := getPath(h, "/news/2019/launch").Header().Get("Location"); got != "/2019/posts/launch" {
		t.Errorf("Location = %q", got)
	}
	// One segment means one: a deeper path is not this rule's business.
	if rec := getPath(h, "/news/2019/launch/extra"); rec.Body.String() != "static" {
		t.Errorf("a deeper path must fall through: %q", rec.Body)
	}
	if rec := getPath(h, "/news/2019"); rec.Body.String() != "static" {
		t.Errorf("a shorter path must fall through: %q", rec.Body)
	}
}

// TestALongerPlaceholderIsNotEatenByAShorterOne: `:id` must not consume the
// front of `:idx` when both are substituted into one destination.
func TestALongerPlaceholderIsNotEatenByAShorterOne(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/x/:id/:idx /y/:idx/:id 301\n")
	if got := getPath(h, "/x/aaa/bbb").Header().Get("Location"); got != "/y/bbb/aaa" {
		t.Errorf("Location = %q", got)
	}
}

// TestAGoneRuleAnswers410WithNoLocation: 410 is an answer, not a destination.
func TestAGoneRuleAnswers410WithNoLocation(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/retired/ / 410\n")

	rec := getPath(h, "/retired/")
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("410 must carry no Location, got %q", loc)
	}
	if rec.Body.Len() == 0 {
		t.Error("410 may carry a body; an empty one tells a reviewer nothing")
	}
}

// TestTheFirstMatchWins: the order in the file is the order that is honoured,
// which is why the generator emits exact rules before wildcard ones.
func TestTheFirstMatchWins(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/blog/special/ /kept/ 301\n/blog/* /archive/:splat 301\n")

	if got := getPath(h, "/blog/special/").Header().Get("Location"); got != "/kept/" {
		t.Errorf("the specific rule must win: %q", got)
	}
	if got := getPath(h, "/blog/other/").Header().Get("Location"); got != "/archive/other/" {
		t.Errorf("the wildcard still covers the rest: %q", got)
	}
}

// TestARedirectShadowsAFileAtTheSamePath: Cloudflare Pages evaluates redirect
// rules before static assets, and that is the platform this mirrors.
func TestARedirectShadowsAFileAtTheSamePath(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/about/ /team/ 301\n")
	if got := getPath(h, "/about/").Code; got != http.StatusMovedPermanently {
		t.Errorf("status = %d — the rule must shadow the asset", got)
	}
}

// TestAForceMarkerIsParsedRatherThanChokedOn: `!` is Netlify's shadow marker.
// Under these semantics everything shadows, so it changes nothing — but a file
// the build wrote must never take the server down.
func TestAForceMarkerIsParsedRatherThanChokedOn(t *testing.T) {
	resetLiveRules(t)
	var warnings []string
	table := parseRedirectsFile("/a /b 301!\n", quietWarn(&warnings))
	if len(warnings) != 0 {
		t.Fatalf("`!` must parse cleanly: %v", warnings)
	}
	if len(table) != 1 || !table[0].forced || table[0].status != http.StatusMovedPermanently {
		t.Fatalf("rule = %+v", table)
	}
}

// TestAMalformedLineIsSkippedAndTheRestStaysLive: never fail the server over a
// file the build itself wrote.
func TestAMalformedLineIsSkippedAndTheRestStaysLive(t *testing.T) {
	resetLiveRules(t)
	h, warnings := publishRules(t, strings.Join([]string{
		"# Cloudflare Pages Redirects",
		"",
		"/lonely",                 // no destination
		"relative/from /to 301",   // source is not a path
		"/a /b notanumber",        // status is not a number
		"/c /d 404",               // not a redirect status
		"/good/ /still-here/ 301", // and this one must survive all of it
	}, "\n"))

	if len(warnings) != 4 {
		t.Fatalf("want one warning per bad line, got %d: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "_redirects:") {
			t.Errorf("a warning must name the line: %q", w)
		}
	}
	if got := getPath(h, "/good/").Header().Get("Location"); got != "/still-here/" {
		t.Errorf("the good rule must still be live: %q", got)
	}
}

// TestNoTableMeansPureFallThrough: a server built before anything was published
// behaves exactly as it did.
func TestNoTableMeansPureFallThrough(t *testing.T) {
	resetLiveRules(t)
	h := liveRedirectHandler(staticSaying("static"))
	if rec := getPath(h, "/anything"); rec.Body.String() != "static" {
		t.Errorf("body = %q", rec.Body)
	}
}

// TestTheRedirectTableSwapIsAtomic mirrors TestSwapIsAtomic from the endpoint
// table: under -race, a request sees one whole table or the other.
func TestTheRedirectTableSwapIsAtomic(t *testing.T) {
	resetLiveRules(t)
	h := liveRedirectHandler(staticSaying("static"))
	first := parseRedirectsFile("/go /a/ 301\n", func(string, ...any) {})
	liveRedirects.Store(&first)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			target := "/a/"
			if i%2 == 0 {
				target = "/b/"
			}
			table := parseRedirectsFile("/go "+target+" 301\n", func(string, ...any) {})
			liveRedirects.Store(&table)
		}
	}()
	for i := 0; i < 200; i++ {
		rec := getPath(h, "/go")
		if rec.Code != http.StatusMovedPermanently {
			t.Fatalf("status = %d during a swap", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/a/" && loc != "/b/" {
			t.Fatalf("Location = %q during a swap", loc)
		}
	}
	<-done
}

// TestAMissingFileIsAnEmptyTable: a server can be built before a build has
// written anything, and that is not a warning.
func TestAMissingFileIsAnEmptyTable(t *testing.T) {
	resetLiveRules(t)
	var warnings []string
	publishRedirects(filepath.Join(t.TempDir(), "nowhere"), quietWarn(&warnings))
	if len(warnings) != 0 {
		t.Errorf("a missing file must be silent: %v", warnings)
	}
	if table := liveRedirects.Load(); table == nil || len(*table) != 0 {
		t.Errorf("table = %v, want empty", table)
	}
}

// TestTheRealGeneratedFileIsServed is the end-to-end claim: a site declaring
// `redirects:` builds, and the very bytes the build wrote are what the preview
// answers with. Anything less tests a format rather than the feature.
func TestTheRealGeneratedFileIsServed(t *testing.T) {
	resetLiveRules(t)
	dir := t.TempDir()
	t.Chdir(dir)
	source := filepath.Join("content", "src")
	for _, sub := range []string{
		filepath.Join(source, "pages"), filepath.Join(source, "posts"),
		filepath.Join("templates", "simple"),
	} {
		if err := os.MkdirAll(sub, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "pages", "about.md"),
		[]byte("---\ntitle: About\n---\nabout\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"base.html":  `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html": `{{define "content"}}Index{{end}}`,
		"page.html":  `{{define "content"}}Page{{end}}`,
		"post.html":  `{{define "content"}}Post{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join("templates", "simple", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.Quiet = true
	cfg.Source, cfg.Template, cfg.Domain = "src", "simple", "example.com"
	cfg.OutputDir = "output"
	cfg.Redirects = []config.RedirectRule{
		{From: "/old-page/", To: "/about/", Status: 301},
		{From: "/blog/*", To: "/:splat", Status: 302},
	}
	genCfg := createGeneratorConfig(cfg)
	if err := build(genCfg, cfg); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(filepath.Join("output", "_redirects")); err != nil {
		t.Fatalf("the build must write _redirects: %v", err)
	}

	var warnings []string
	republishOutputRules(cfg)
	publishRedirects(cfg.OutputDir, quietWarn(&warnings))
	if len(warnings) != 0 {
		t.Fatalf("the generator's own output must parse cleanly: %v", warnings)
	}
	h := liveRedirectHandler(staticSaying("static"))
	if got := getPath(h, "/old-page/").Header().Get("Location"); got != "/about/" {
		t.Errorf("Location = %q", got)
	}
	rec := getPath(h, "/blog/a/b/")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/a/b/" {
		t.Errorf("splat: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	_ = generator.RedirectRule{} // the type the config rules become
}

// TestAnEmptySegmentDoesNotSatisfyAPlaceholder: `//launch` must not be read as
// a year of "" — a capture that matched nothing would substitute nothing and
// send the visitor somewhere the rule never named.
func TestAnEmptySegmentDoesNotSatisfyAPlaceholder(t *testing.T) {
	resetLiveRules(t)
	h, _ := publishRules(t, "/news/:year/:slug /:year/posts/:slug 301\n")
	if rec := getPath(h, "/news//launch"); rec.Body.String() != "static" {
		t.Errorf("an empty capture must not match: %d %q", rec.Code, rec.Body)
	}
}

// TestWarningsAreSilencedByQuiet: a warning belongs on stderr, and --quiet
// means --quiet. Also the path where they are printed at all.
func TestWarningsAreSilencedByQuiet(t *testing.T) {
	resetLiveRules(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_redirects"), []byte("/lonely\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.OutputDir = dir

	cfg.Quiet = false
	stderr := captureStderr(t, func() { republishOutputRules(cfg) })
	if !strings.Contains(stderr, "_redirects:1") {
		t.Errorf("a malformed line must be reported: %q", stderr)
	}

	cfg.Quiet = true
	if got := captureStderr(t, func() { republishOutputRules(cfg) }); got != "" {
		t.Errorf("--quiet must stay quiet, got %q", got)
	}
}
