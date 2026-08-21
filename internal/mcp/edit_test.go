package mcp

// Anchored partial edits (#187).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProjectFile puts a file in the temp project and returns its path.
func writeProjectFile(t *testing.T, root, rel, body string) string {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return abs
}

// readProjectFile returns a file's current bytes.
func readProjectFile(t *testing.T, abs string) string {
	t.Helper()
	b, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const themeCSS = `:root {
  --ink: #111;
}

body {
  background: #fff;
  color: var(--ink);
}

footer {
  color: #666;
}
`

// TestTheReportedCaseCostsOneCall: change one CSS value without reading or
// rewriting the file. The reply carries the proof, so there is no third call.
func TestTheReportedCaseCostsOneCall(t *testing.T) {
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "static/css/style.css", themeCSS)

	res := call(t, s, "designer_edit", map[string]any{
		"path": "static/css/style.css",
		"old":  "  background: #fff;",
		"new":  "  background: #0b1220;",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", text(res))
	}
	if got := readProjectFile(t, abs); !strings.Contains(got, "background: #0b1220;") ||
		strings.Contains(got, "background: #fff;") {
		t.Fatalf("file not edited:\n%s", got)
	}
	// Everything else is byte-for-byte intact — the point of an anchor.
	if got := readProjectFile(t, abs); !strings.Contains(got, "--ink: #111;") ||
		!strings.Contains(got, "footer {") {
		t.Errorf("the rest of the file changed:\n%s", got)
	}

	out := text(res)
	if !strings.Contains(out, "line 6") {
		t.Errorf("the reply must say where the change landed: %q", out)
	}
	// The context IS the verification, so it must show the new text with its
	// neighbours and mark which line moved.
	for _, want := range []string{"→ ", "background: #0b1220;", "color: var(--ink);", "body {"} {
		if !strings.Contains(out, want) {
			t.Errorf("reply missing %q:\n%s", want, out)
		}
	}
	// The claim is O(change), not O(file): the reply is bounded by the context
	// window regardless of how big the file is. Asserting it against this
	// fixture would prove nothing — a 12-line file is smaller than any report
	// with line numbers — so assert the bound itself.
	if n := len(strings.Split(out, "\n")); n > 2*editContextLines+1+2 {
		t.Errorf("the reply is %d lines, want the change plus %d either side:\n%s",
			n, editContextLines, out)
	}
}

// TestTheReplyDoesNotGrowWithTheFile is the measurement the report turns on:
// the same one-line change in a file 100× larger must cost the same reply.
func TestTheReplyDoesNotGrowWithTheFile(t *testing.T) {
	s, root := newTestServer(t, nil)
	big := strings.Repeat("/* filler filler filler filler filler */\n", 500) +
		"body { background: #fff; }\n" +
		strings.Repeat("/* filler filler filler filler filler */\n", 500)
	writeProjectFile(t, root, "static/css/big.css", big)

	res := call(t, s, "designer_edit", map[string]any{
		"path": "static/css/big.css",
		"old":  "body { background: #fff; }",
		"new":  "body { background: #0b1220; }",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", text(res))
	}
	out := text(res)
	if len(out) > len(big)/20 {
		t.Errorf("reply is %d bytes against a %d-byte file — the cost still tracks the file",
			len(out), len(big))
	}
	if !strings.Contains(out, "#0b1220") {
		t.Errorf("reply must show the change:\n%s", out)
	}
}

// TestAnAmbiguousAnchorIsRefusedWithTheCount: the safety the full-file contract
// bought. Replacing "the first #666" silently would be the fuzzy behaviour this
// deliberately does not have.
func TestAnAmbiguousAnchorIsRefusedWithTheCount(t *testing.T) {
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "static/css/style.css", "a { color: #666; }\nb { color: #666; }\n")
	before := readProjectFile(t, abs)

	res := call(t, s, "designer_edit", map[string]any{
		"path": "static/css/style.css", "old": "color: #666;", "new": "color: #000;",
	})
	if !res.IsError {
		t.Fatal("an anchor matching twice must be refused")
	}
	if !strings.Contains(text(res), "2 times") {
		t.Errorf("the refusal must name the count: %q", text(res))
	}
	if readProjectFile(t, abs) != before {
		t.Error("a refused edit must leave the file untouched")
	}
}

// TestAnAnchorThatIsNotThereIsRefused, rather than appended or guessed at.
func TestAnAnchorThatIsNotThereIsRefused(t *testing.T) {
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "templates/base.html", "<body></body>\n")

	res := call(t, s, "designer_edit", map[string]any{
		"path": "templates/base.html", "old": "<header>", "new": "<header class=x>",
	})
	if !res.IsError || !strings.Contains(text(res), "does not appear") {
		t.Fatalf("result = %q", text(res))
	}
	if readProjectFile(t, abs) != "<body></body>\n" {
		t.Error("the file must be untouched")
	}
}

// TestWhitespaceInTheAnchorIsSignificant: arguments are trimmed everywhere else
// in this server, and trimming here would make an indented anchor — the normal
// case in CSS and HTML — fail to find text that is plainly present.
func TestWhitespaceInTheAnchorIsSignificant(t *testing.T) {
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "templates/base.html", "<ul>\n    <li>one</li>\n</ul>\n")

	res := call(t, s, "designer_edit", map[string]any{
		"path": "templates/base.html", "old": "    <li>one</li>", "new": "    <li>two</li>",
	})
	if res.IsError {
		t.Fatalf("an indented anchor must match: %s", text(res))
	}
	if got := readProjectFile(t, abs); got != "<ul>\n    <li>two</li>\n</ul>\n" {
		t.Errorf("file = %q", got)
	}
}

// TestAnEmptyReplacementDeletes, which is how a stray line goes away without a
// whole-file write.
func TestAnEmptyReplacementDeletes(t *testing.T) {
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "templates/base.html", "<p>keep</p>\n<p>drop</p>\n")

	res := call(t, s, "designer_edit", map[string]any{
		"path": "templates/base.html", "old": "<p>drop</p>\n", "new": "",
	})
	if res.IsError {
		t.Fatalf("empty `new` must be allowed: %s", text(res))
	}
	if got := readProjectFile(t, abs); got != "<p>keep</p>\n" {
		t.Errorf("file = %q", got)
	}
}

// TestAMissingArgumentIsNamed: `old` absent and `new` absent are different
// mistakes, and an empty string is a legitimate value for exactly one of them.
func TestAMissingArgumentIsNamed(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "templates/base.html", "x\n")

	res := call(t, s, "designer_edit", map[string]any{"path": "templates/base.html", "new": "y"})
	if !res.IsError || !strings.Contains(text(res), "`old` is required") {
		t.Errorf("missing old = %q", text(res))
	}
	res = call(t, s, "designer_edit", map[string]any{"path": "templates/base.html", "old": "x"})
	if !res.IsError || !strings.Contains(text(res), "`new` is required") {
		t.Errorf("missing new = %q", text(res))
	}
	res = call(t, s, "designer_edit", map[string]any{"path": "templates/base.html", "old": "", "new": "y"})
	if !res.IsError || !strings.Contains(text(res), "`old` is required") {
		t.Errorf("empty old = %q", text(res))
	}
}

// TestAnEditCannotEscapeItsSection: the anchor is new, the confinement is not,
// and a path check that only guarded the old tools would be a hole.
func TestAnEditCannotEscapeItsSection(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "content/posts/hello.md", "---\ntitle: Hi\n---\nbody\n")

	// The designer may not reach content…
	res := call(t, s, "designer_edit", map[string]any{
		"path": "content/posts/hello.md", "old": "body", "new": "nope",
	})
	if !res.IsError {
		t.Error("designer_edit must not reach the content directories")
	}
	// …nor anywhere above the project.
	res = call(t, s, "designer_edit", map[string]any{
		"path": "../escape.html", "old": "a", "new": "b",
	})
	if !res.IsError {
		t.Error("designer_edit must refuse a path that escapes the project")
	}
}

// TestContentEditWorksAndKeepsItsRules: same anchor, same Markdown-only and
// must-exist guarantees the other content tools enforce.
func TestContentEditWorksAndKeepsItsRules(t *testing.T) {
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "content/posts/hello.md",
		"---\ntitle: Helo\n---\n\nThe body.\n")

	res := call(t, s, "content_edit", map[string]any{
		"path": "content/posts/hello.md", "old": "title: Helo", "new": "title: Hello",
	})
	if res.IsError {
		t.Fatalf("content_edit failed: %s", text(res))
	}
	if got := readProjectFile(t, abs); !strings.Contains(got, "title: Hello") {
		t.Errorf("file = %q", got)
	}

	if res := call(t, s, "content_edit", map[string]any{
		"path": "content/posts/nope.md", "old": "a", "new": "b",
	}); !res.IsError || !strings.Contains(text(res), "does not exist") {
		t.Errorf("a missing file = %q", text(res))
	}
	if res := call(t, s, "content_edit", map[string]any{
		"path": "content/posts/style.css", "old": "a", "new": "b",
	}); !res.IsError {
		t.Error("content_edit must refuse a non-Markdown path")
	}
}

// TestAReadFailureIsReported: resolveIn is happy with a path whose file cannot
// be read, and the edit must say so rather than write a new file over it.
func TestAReadFailureIsReported(t *testing.T) {
	s, root := newTestServer(t, nil)
	if err := os.MkdirAll(filepath.Join(root, "templates", "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := call(t, s, "designer_edit", map[string]any{
		"path": "templates/adir", "old": "a", "new": "b",
	})
	if !res.IsError || !strings.Contains(text(res), "read failed") {
		t.Errorf("result = %q", text(res))
	}
}

// TestAMultiLineReplacementIsReportedInFull, so a model can see every line it
// added without reading the file back.
func TestAMultiLineReplacementIsReportedInFull(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "templates/base.html", "<body>\n  <main></main>\n</body>\n")

	res := call(t, s, "designer_edit", map[string]any{
		"path": "templates/base.html",
		"old":  "  <main></main>",
		"new":  "  <main>\n    <h1>Title</h1>\n  </main>",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", text(res))
	}
	out := text(res)
	for _, want := range []string{"<main>", "<h1>Title</h1>", "</main>"} {
		if !strings.Contains(out, want) {
			t.Errorf("reply missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "→ ") != 3 {
		t.Errorf("all three new lines must be marked:\n%s", out)
	}
}

// TestTheEditToolsAreAdvertised: a tool a model cannot discover saves nobody
// anything, and the help contract has to stop insisting on full files.
func TestTheEditToolsAreAdvertised(t *testing.T) {
	s, _ := newTestServer(t, nil)
	names := map[string]bool{}
	for _, tl := range s.tools {
		names[tl.name] = true
	}
	for _, want := range []string{"designer_edit", "content_edit"} {
		if !names[want] {
			t.Errorf("%s is not registered", want)
		}
	}
	help := text(call(t, s, "help", map[string]any{}))
	if !strings.Contains(help, "designer_edit") || !strings.Contains(help, "content_edit") {
		t.Errorf("help does not mention the edit tools:\n%s", help)
	}
	if strings.Contains(help, "always send FULL file contents") {
		t.Error("the contract still tells the model to send whole files for every change")
	}
}

// TestTheReportClampsToTheFile: a change on the first or last line must not
// index outside the slice.
func TestTheReportClampsToTheFile(t *testing.T) {
	got := editReport("only line", 1, 1)
	if got != "→    1 | only line" {
		t.Errorf("single-line report = %q", got)
	}
	if r := editReport("a\nb\nc", 3, 1); !strings.Contains(r, "c") || strings.Contains(r, "   4 |") {
		t.Errorf("last-line report = %q", r)
	}
}

// TestAWriteFailureIsReportedNotSwallowed: the anchor matched, so the model
// believes the change landed unless the failure comes back.
func TestAWriteFailureIsReportedNotSwallowed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s, root := newTestServer(t, nil)
	// The file, not its directory: modifying a file in place needs write
	// permission on the file. A read-only directory would still let this
	// succeed, which is why the first version of this test passed vacuously.
	abs := writeProjectFile(t, root, "templates/locked/base.html", "<body>old</body>\n")
	if err := os.Chmod(abs, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(abs, 0o600) })

	res := call(t, s, "designer_edit", map[string]any{
		"path": "templates/locked/base.html", "old": "old", "new": "new",
	})
	if !res.IsError || !strings.Contains(text(res), "write failed") {
		t.Errorf("result = %q", text(res))
	}
}
