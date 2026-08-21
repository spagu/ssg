package mcp

// Finding a place in the theme without reading it (#190).

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const findCSS = `:root {
  --ink: #111111;
}

body {
  background: #ffffff;
  color: var(--ink);
}

.card {
  background: #f5f5f5;
}
`

// TestTheReportedQuestionIsAnsweredWithoutReadingTheFile: "where is the page
// background set?" used to mean listing the theme and reading files until the
// line turned up.
func TestTheReportedQuestionIsAnsweredWithoutReadingTheFile(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/css/style.css", findCSS)
	writeProjectFile(t, root, "templates/base.html", "<body class=\"page\"></body>\n")

	out := text(call(t, s, "designer_find", map[string]any{"query": "background"}))

	if !strings.Contains(out, "static/css/style.css:") {
		t.Fatalf("the file must be named:\n%s", out)
	}
	// The line range is what makes the answer actionable — it is the anchor
	// designer_edit needs, so no read is required between find and edit.
	if !strings.Contains(out, ":4-8") {
		t.Errorf("expected the range around line 6:\n%s", out)
	}
	if !strings.Contains(out, "background: #ffffff;") {
		t.Errorf("the fragment must carry the matching line:\n%s", out)
	}
	// Two separate regions in one file stay separate.
	if !strings.Contains(out, ".card {") {
		t.Errorf("the second occurrence must be reported too:\n%s", out)
	}
}

// TestAdjacentMatchesBecomeOneRegion: five consecutive matching lines are one
// place, not five answers, or the reply is longer than the file.
func TestAdjacentMatchesBecomeOneRegion(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/css/a.css",
		"a{}\nx: 1;\nx: 2;\nx: 3;\nx: 4;\nb{}\n")

	out := text(call(t, s, "designer_find", map[string]any{"query": "^x:"}))
	if !strings.Contains(out, "1 match ") {
		t.Errorf("four adjacent lines are one region:\n%s", out)
	}
}

// TestTheQueryIsARegexWhenItCanBeAndLiteralWhenItCannot. A model pasting a CSS
// snippet must get an answer, not a syntax error from a stray bracket.
func TestTheQueryIsARegexWhenItCanBeAndLiteralWhenItCannot(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/css/a.css", "h1 { color: red; }\nh2 { color: blue; }\n")

	if out := text(call(t, s, "designer_find", map[string]any{"query": "h[12]"})); !strings.Contains(out, "h1") {
		t.Errorf("a valid regex must be used as one:\n%s", out)
	}
	// Unbalanced: not a regex, so it must be matched literally rather than fail.
	out := text(call(t, s, "designer_find", map[string]any{"query": "color: red; }"}))
	if !strings.Contains(out, "h1 { color: red; }") {
		t.Errorf("an invalid regex must be matched literally:\n%s", out)
	}
}

// TestCaseIsIgnored, because a model asking for "Background" means the same
// thing the stylesheet spells in lower case.
func TestCaseIsIgnored(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "templates/base.html", "<DIV class=Hero></DIV>\n")
	if out := text(call(t, s, "designer_find", map[string]any{"query": "hero"})); !strings.Contains(out, "Hero") {
		t.Errorf("search must be case-insensitive:\n%s", out)
	}
}

// TestNoMatchSaysSoAndExplainsHow: a bare "no results" invites the model to
// retry the same query with different quoting.
func TestNoMatchSaysSoAndExplainsHow(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "templates/base.html", "<body></body>\n")

	out := text(call(t, s, "designer_find", map[string]any{"query": "nowhere"}))
	if !strings.Contains(out, "No match") || !strings.Contains(out, "regular expression") {
		t.Errorf("result = %q", out)
	}
}

// TestAnEmptyQueryIsRefused rather than matching every line of the theme.
func TestAnEmptyQueryIsRefused(t *testing.T) {
	s, _ := newTestServer(t, nil)
	res := call(t, s, "designer_find", map[string]any{"query": "   "})
	if !res.IsError || !strings.Contains(text(res), "`query` is required") {
		t.Errorf("result = %q", text(res))
	}
}

// TestTheLimitIsHonoured, including the JSON-number shape an argument arrives in.
func TestTheLimitIsHonoured(t *testing.T) {
	s, root := newTestServer(t, nil)
	body := strings.Repeat("hit\n\n\n\n\n", 40) // spaced so no two merge
	writeProjectFile(t, root, "static/css/many.css", body)

	out := text(call(t, s, "designer_find", map[string]any{"query": "hit", "limit": float64(3)}))
	if !strings.Contains(out, "3 matches") {
		t.Errorf("limit ignored:\n%s", out[:min(len(out), 200)])
	}
	// Nonsense limits fall back to the default rather than returning nothing.
	if got := intArg(map[string]any{"limit": float64(0)}, "limit", 20); got != 20 {
		t.Errorf("zero limit = %d, want the default", got)
	}
	if got := intArg(map[string]any{"limit": "many"}, "limit", 20); got != 20 {
		t.Errorf("non-numeric limit = %d, want the default", got)
	}
	if got := intArg(map[string]any{}, "limit", 20); got != 20 {
		t.Errorf("absent limit = %d", got)
	}
}

// TestEachSectionSearchesItsOwnDirectories: the find tools must not become a
// way around the confinement the read and write tools enforce.
func TestEachSectionSearchesItsOwnDirectories(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "templates/base.html", "SECRETMARKER in a template\n")
	writeProjectFile(t, root, "content/posts/a.md", "SECRETMARKER in content\n")

	designer := text(call(t, s, "designer_find", map[string]any{"query": "SECRETMARKER"}))
	if strings.Contains(designer, "content/posts") {
		t.Errorf("the designer must not see content:\n%s", designer)
	}
	content := text(call(t, s, "content_find", map[string]any{"query": "SECRETMARKER"}))
	if strings.Contains(content, "templates/") {
		t.Errorf("content must not see templates:\n%s", content)
	}
	if !strings.Contains(content, "content/posts/a.md") {
		t.Errorf("content_find found nothing:\n%s", content)
	}
}

// TestContentFindOnlySearchesMarkdown, matching what content_* may edit.
func TestContentFindOnlySearchesMarkdown(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "content/notes.txt", "MARKER\n")
	writeProjectFile(t, root, "content/posts/a.md", "MARKER\n")

	out := text(call(t, s, "content_find", map[string]any{"query": "MARKER"}))
	if strings.Contains(out, "notes.txt") {
		t.Errorf("a non-Markdown file must not be searched:\n%s", out)
	}
	if !strings.Contains(out, "posts/a.md") {
		t.Errorf("out = %q", out)
	}
}

// TestAHugeFileIsSkipped: a minified bundle matches everything and helps
// nobody, and its "fragment" would swamp the reply.
func TestAHugeFileIsSkipped(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/js/bundle.js", strings.Repeat("needle;", findMaxFileSize/7+1))
	writeProjectFile(t, root, "static/js/app.js", "needle;\n")

	out := text(call(t, s, "designer_find", map[string]any{"query": "needle"}))
	if strings.Contains(out, "bundle.js") {
		t.Error("an oversized file must be skipped")
	}
	if !strings.Contains(out, "app.js") {
		t.Errorf("the ordinary file must still be found:\n%s", out)
	}
}

// TestABackendAnswerIsPreferredAndLabelled: an index that understands a phrase
// is the reason the hook exists, and the reply must say where it came from.
func TestABackendAnswerIsPreferredAndLabelled(t *testing.T) {
	var askedFor string
	s, root := newTestServer(t, func(o *Options) {
		o.Search = func(q string, limit int) ([]FindHit, error) {
			askedFor = q
			return []FindHit{{Path: "static/css/style.css", From: 5, To: 7,
				Fragment: "body { background: #fff; }", Note: "score 0.91"}}, nil
		}
	})
	writeProjectFile(t, root, "static/css/style.css", findCSS)

	out := text(call(t, s, "designer_find", map[string]any{"query": "background colour of the page"}))
	if askedFor != "background colour of the page" {
		t.Errorf("the backend got %q", askedFor)
	}
	for _, want := range []string{"static/css/style.css:5-7", "score 0.91", "search backend"} {
		if !strings.Contains(out, want) {
			t.Errorf("reply missing %q:\n%s", want, out)
		}
	}
}

// TestABrokenBackendFallsBackToTheLocalScan. A search server that is down must
// not take the ability to work on the site down with it.
func TestABrokenBackendFallsBackToTheLocalScan(t *testing.T) {
	var logged strings.Builder
	s, root := newTestServer(t, func(o *Options) {
		o.Logf = func(f string, a ...any) { logged.WriteString(f) }
		o.Search = func(string, int) ([]FindHit, error) { return nil, errors.New("connection refused") }
	})
	writeProjectFile(t, root, "static/css/style.css", findCSS)

	out := text(call(t, s, "designer_find", map[string]any{"query": "background"}))
	if !strings.Contains(out, "static/css/style.css") {
		t.Errorf("the local scan must still answer:\n%s", out)
	}
	if strings.Contains(out, "search backend") {
		t.Error("a failed backend must not be credited with the answer")
	}
	if !strings.Contains(logged.String(), "search backend") {
		t.Errorf("the failure must be logged: %q", logged.String())
	}
}

// TestAnEmptyBackendAnswerAlsoFallsBack: "found nothing" from an index that has
// not been synced is indistinguishable from "not there", and the local scan
// knows better.
func TestAnEmptyBackendAnswerAlsoFallsBack(t *testing.T) {
	s, root := newTestServer(t, func(o *Options) {
		o.Search = func(string, int) ([]FindHit, error) { return nil, nil }
	})
	writeProjectFile(t, root, "static/css/style.css", findCSS)

	if out := text(call(t, s, "designer_find", map[string]any{"query": "background"})); !strings.Contains(out, "style.css") {
		t.Errorf("an empty backend answer must fall through:\n%s", out)
	}
}

// TestTheFindToolsAreAdvertised and the contract tells the model to use them.
func TestTheFindToolsAreAdvertised(t *testing.T) {
	s, _ := newTestServer(t, nil)
	names := map[string]bool{}
	for _, tl := range s.tools {
		names[tl.name] = true
	}
	for _, want := range []string{"designer_find", "content_find"} {
		if !names[want] {
			t.Errorf("%s is not registered", want)
		}
	}
	if help := text(call(t, s, "help", map[string]any{})); !strings.Contains(help, "designer_find") {
		t.Errorf("help must point at the find tools:\n%s", help)
	}
}

// TestFindReportsAreStable: two identical queries must answer identically, or a
// model cannot tell a changed site from a reshuffled reply.
func TestFindReportsAreStable(t *testing.T) {
	s, root := newTestServer(t, nil)
	for _, name := range []string{"z.css", "a.css", "m.css"} {
		writeProjectFile(t, root, "static/css/"+name, "marker\n")
	}
	first := text(call(t, s, "designer_find", map[string]any{"query": "marker"}))
	if second := text(call(t, s, "designer_find", map[string]any{"query": "marker"})); first != second {
		t.Errorf("unstable order:\n%s\n---\n%s", first, second)
	}
	if strings.Index(first, "a.css") > strings.Index(first, "z.css") {
		t.Errorf("results must be sorted:\n%s", first)
	}
}

// TestALiteralFallbackIsFlagged: when the query could not be a regex the "no
// match" note must not blame regex syntax, or the model rewrites a query that
// was never treated as one.
func TestALiteralFallbackIsFlagged(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "templates/base.html", "<body></body>\n")

	out := text(call(t, s, "designer_find", map[string]any{"query": "a(b"}))
	if !strings.Contains(out, "No match") {
		t.Fatalf("out = %q", out)
	}
	if strings.Contains(out, "regular expression") {
		t.Errorf("a literal query must not be described as a regex: %q", out)
	}
}

// TestSearchStopsOnceTheLimitIsReached, without opening the remaining files.
func TestSearchStopsOnceTheLimitIsReached(t *testing.T) {
	s, root := newTestServer(t, nil)
	for _, n := range []string{"a.css", "b.css", "c.css"} {
		writeProjectFile(t, root, "static/css/"+n, "marker\n")
	}
	out := text(call(t, s, "designer_find", map[string]any{"query": "marker", "limit": float64(1)}))
	if !strings.Contains(out, "1 match ") || strings.Contains(out, "b.css") {
		t.Errorf("out = %q", out)
	}
}

// TestAnUnreadableFileIsSkippedRatherThanFatal: a search that dies on one
// unreadable file answers nothing about the rest of the theme.
func TestAnUnreadableFileIsSkippedRatherThanFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	s, root := newTestServer(t, nil)
	locked := writeProjectFile(t, root, "static/css/locked.css", "marker\n")
	writeProjectFile(t, root, "static/css/open.css", "marker\n")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	out := text(call(t, s, "designer_find", map[string]any{"query": "marker"}))
	if !strings.Contains(out, "open.css") {
		t.Errorf("the readable file must still answer:\n%s", out)
	}
}

// TestABadPatternInAnEmptyQueryIsCaughtBeforeSearching.
func TestCompileQueryRejectsBlank(t *testing.T) {
	if _, err := compileQuery(""); err == nil {
		t.Error("an empty query must be refused")
	}
	if q, err := compileQuery("a(b"); err != nil || !q.literal {
		t.Errorf("q = %+v, err = %v — want a literal fallback", q, err)
	}
}
