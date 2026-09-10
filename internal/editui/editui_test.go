package editui

// The editing endpoints (GO-102, phase 1).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/mcp"
	"github.com/spagu/ssg/internal/models"
)

// fakeGit records what the editor asked git to do, so a test can check the
// branch-then-commit contract without a repository.
type fakeGit struct {
	calls  []string
	branch string
	fail   map[string]bool
}

func (f *fakeGit) run(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if f.fail[args[0]] {
		return "", fmt.Errorf("git %s refused", args[0])
	}
	switch args[0] {
	case "status":
		return "## " + f.branch + "\n M content/site/posts/news/hello.md\n", nil
	case "rev-parse":
		return f.branch, nil
	case "checkout":
		if len(args) > 2 {
			f.branch = args[2]
		}
		return "", nil
	}
	return "", nil
}

const samplePost = `---
title: Hello World
slug: hello
status: publish
type: post
date: 2026-01-15
tags: [go, ssg]
---

The body.
`

// newEditor builds an editing server over a real MCP server on a temporary
// project, so the path confinement under test is the one that ships.
func newEditor(t *testing.T, withGit bool) (*Server, string, *fakeGit) {
	t.Helper()
	root := t.TempDir()
	rel := filepath.Join("content", "site", "posts", "news", "hello.md")
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(samplePost), 0o600); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{branch: "main", fail: map[string]bool{}}
	opts := mcp.Options{
		Root:        root,
		ContentDirs: []string{"content"},
		Roles:       map[string]bool{"content": true},
		Rebuild:     func() (string, error) { return "rebuilt", nil },
	}
	if withGit {
		opts.Git = mcp.GitOptions{Local: true, Run: git.run, BranchPrefix: "edit/",
			StageDirs: []string{"content"}, Now: func() string { return "20260910-120000" }}
	}
	server := New(Options{
		MCP:   mcp.NewServer(opts),
		Token: "secret",
		Schemas: map[string]models.ContentSchema{"post": {
			Required: []string{"title", "date"},
			Fields: map[string]models.FieldRule{
				"title":  {Type: "string"},
				"date":   {Type: "date"},
				"status": {Type: "enum", Values: []string{"publish", "draft"}},
				"tags":   {Type: "list"},
				"weight": {Type: "int"},
				"pinned": {Type: "bool"},
				"source": {Type: "url"},
			},
		}},
	})
	return server, filepath.ToSlash(rel), git
}

// call sends an authorised request to the editor.
func call(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	r.Header.Set(TokenHeader, "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON (%d): %s", w.Code, w.Body.String())
	}
	return m
}

// TestTokenIsRequired: every endpoint, every method. This writes files; an
// unauthenticated request must not reach any of them.
func TestTokenIsRequired(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	for _, target := range []string{Path + "doc?path=" + rel, Path + "frontmatter", Path + "status"} {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: %d", target, w.Code)
		}
		r = httptest.NewRequest(http.MethodGet, target, nil)
		r.Header.Set(TokenHeader, "wrong")
		w = httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s with the wrong token: %d", target, w.Code)
		}
	}
	// A server with no token configured refuses everything rather than
	// accepting everything.
	open := New(Options{MCP: mcp.NewServer(mcp.Options{Root: t.TempDir()})})
	r := httptest.NewRequest(http.MethodGet, Path+"status", nil)
	w := httptest.NewRecorder()
	open.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("an editor with no token must refuse, got %d", w.Code)
	}
}

// TestDocBuildsTheFormFromTheSchema: the declared contract decides the
// controls, and the document's own keys fill in the rest.
func TestDocBuildsTheFormFromTheSchema(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	w := call(t, s, http.MethodGet, Path+"doc?path="+rel, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var got docResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "post" || got.Path != rel {
		t.Errorf("doc = %+v", got)
	}
	byName := map[string]field{}
	for _, f := range got.Fields {
		byName[f.Name] = f
	}
	if f := byName["title"]; f.Kind != "string" || !f.Required || f.Value != "Hello World" {
		t.Errorf("title = %+v", f)
	}
	if f := byName["status"]; f.Kind != "enum" || len(f.Values) != 2 {
		t.Errorf("status = %+v", f)
	}
	if f := byName["tags"]; f.Kind != "list" || f.Value != "go, ssg" {
		t.Errorf("tags = %+v", f)
	}
	if _, ok := byName["slug"]; !ok {
		t.Error("a key the document has but the schema does not declare should still be editable")
	}
	// Required fields lead the form.
	if got.Fields[0].Name != "title" || got.Fields[1].Name != "date" {
		t.Errorf("field order = %s, %s", got.Fields[0].Name, got.Fields[1].Name)
	}
}

// TestDocErrors: a missing path, a file outside the content directories, and a
// file the parser cannot read.
func TestDocErrors(t *testing.T) {
	s, _, _ := newEditor(t, false)
	if w := call(t, s, http.MethodGet, Path+"doc", ""); w.Code != http.StatusBadRequest {
		t.Errorf("no path: %d", w.Code)
	}
	if w := call(t, s, http.MethodGet, Path+"doc?path=../../etc/passwd", ""); w.Code != http.StatusNotFound {
		t.Errorf("escape attempt: %d — %s", w.Code, w.Body.String())
	}
	if w := call(t, s, http.MethodGet, Path+"doc?path=content/site/nope.md", ""); w.Code != http.StatusNotFound {
		t.Errorf("missing file: %d", w.Code)
	}
}

// TestSaveWritesValidatesAndCommits: the whole path, once.
func TestSaveWritesValidatesAndCommits(t *testing.T) {
	s, rel, git := newEditor(t, true)
	w := call(t, s, http.MethodPost, Path+"frontmatter",
		`{"path":"`+rel+`","key":"title","value":"Hello, edited"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	if body["saved"] != true {
		t.Errorf("save = %v", body)
	}
	branch, _ := body["branch"].(string)
	if !strings.HasPrefix(branch, "edit/") {
		t.Errorf("branch = %q, want one of its own", branch)
	}
	joined := strings.Join(git.calls, " | ")
	if !strings.Contains(joined, "checkout -b") || !strings.Contains(joined, "commit") {
		t.Errorf("git calls = %s", joined)
	}
	// A second save reuses the session's branch rather than making another.
	git.calls = nil
	w = call(t, s, http.MethodPost, Path+"frontmatter",
		`{"path":"`+rel+`","key":"tags","value":"go, ssg, editing"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("second save: %s", w.Body.String())
	}
	if strings.Contains(strings.Join(git.calls, " "), "checkout -b") {
		t.Errorf("a second branch was created: %v", git.calls)
	}
	// And the file says what the saves said, with its style intact.
	got := readBack(t, s, rel)
	if !strings.Contains(got, "title: Hello, edited") {
		t.Errorf("title not saved:\n%s", got)
	}
	if !strings.Contains(got, "tags: [go, ssg, editing]") {
		t.Errorf("the list style changed:\n%s", got)
	}
	if !strings.Contains(got, "The body.") {
		t.Errorf("the body was damaged:\n%s", got)
	}
}

// readBack reads the file through the same tool the editor writes with.
func readBack(t *testing.T, s *Server, rel string) string {
	t.Helper()
	text, isErr := s.opts.MCP.Call("content_read", map[string]any{"path": rel})
	if isErr {
		t.Fatal(text)
	}
	return text
}

// TestSaveRefusesWhatTheSchemaForbids: the form cannot write a value the build
// would reject, and the file is untouched when it tries.
func TestSaveRefusesWhatTheSchemaForbids(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	before := readBack(t, s, rel)
	cases := []struct{ key, value, wants string }{
		{"status", "sideways", "one of"},
		{"date", "last tuesday", "not a date"},
		{"weight", "heavy", "whole number"},
		{"pinned", "sort of", "true or false"},
		{"source", "not a url", "not a URL"},
		{"title", "  ", "required"},
	}
	for _, c := range cases {
		w := call(t, s, http.MethodPost, Path+"frontmatter",
			`{"path":"`+rel+`","key":"`+c.key+`","value":"`+c.value+`"}`)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s=%q: %d %s", c.key, c.value, w.Code, w.Body.String())
			continue
		}
		if msg, _ := decode(t, w)["error"].(string); !strings.Contains(msg, c.wants) {
			t.Errorf("%s=%q: %q does not explain the rule", c.key, c.value, msg)
		}
	}
	if readBack(t, s, rel) != before {
		t.Error("a refused save must leave the file alone")
	}
}

// TestSaveAcceptsEveryDeclaredType, writing the value in the shape the type
// calls for.
func TestSaveAcceptsEveryDeclaredType(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	cases := map[string]string{
		"weight": "3", "pinned": "true", "source": "https://example.com/x",
		"date": "2026-02-01", "status": "draft",
	}
	for key, value := range cases {
		if w := call(t, s, http.MethodPost, Path+"frontmatter",
			`{"path":"`+rel+`","key":"`+key+`","value":"`+value+`"}`); w.Code != http.StatusOK {
			t.Errorf("%s=%q: %d %s", key, value, w.Code, w.Body.String())
		}
	}
	got := readBack(t, s, rel)
	for _, want := range []string{"weight: 3", "pinned: true", "date: \"2026-02-01\"", "status: draft"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestSaveNoOpAndUnset: saving what is already there changes nothing, and a key
// can be removed.
func TestSaveNoOpAndUnset(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	w := call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"title","value":"Hello World"}`)
	if w.Code != http.StatusOK || decode(t, w)["saved"] != false {
		t.Errorf("an unchanged value should report nothing changed: %s", w.Body.String())
	}
	w = call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"slug","unset":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("unset: %s", w.Body.String())
	}
	if strings.Contains(readBack(t, s, rel), "slug:") {
		t.Error("the key was not removed")
	}
	w = call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"slug","unset":true}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("removing what is gone: %d", w.Code)
	}
}

// TestSaveRequestShapes: the ways a request can be wrong.
func TestSaveRequestShapes(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	if w := call(t, s, http.MethodGet, Path+"frontmatter", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"frontmatter", "{not json"); w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"frontmatter", `{"key":"title","value":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("no path: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","value":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("no key: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"content/site/gone.md","key":"title","value":"x"}`); w.Code != http.StatusNotFound {
		t.Errorf("missing file: %d", w.Code)
	}
}

// TestSaveWithoutGit says so plainly instead of pretending an edit is tracked.
func TestSaveWithoutGit(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	w := call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"title","value":"Changed"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%s", w.Body.String())
	}
	note, _ := decode(t, w)["git"].(string)
	if !strings.Contains(note, "no git") {
		t.Errorf("git note = %q", note)
	}
	if w := call(t, s, http.MethodGet, Path+"status", ""); decode(t, w)["git"] != false {
		t.Error("status must report that git is unavailable")
	}
}

// TestSaveWhenGitRefuses: the file is written, and the failure is reported
// rather than swallowed — the author needs to know the change is not on a
// branch.
func TestSaveWhenGitRefuses(t *testing.T) {
	s, rel, git := newEditor(t, true)
	git.fail["checkout"] = true
	w := call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"title","value":"One"}`)
	if note, _ := decode(t, w)["git"].(string); !strings.Contains(note, "branch could not be created") {
		t.Errorf("git note = %q", note)
	}
	delete(git.fail, "checkout")
	git.fail["commit"] = true
	w = call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"title","value":"Two"}`)
	if note, _ := decode(t, w)["git"].(string); !strings.Contains(note, "not committed") {
		t.Errorf("git note = %q", note)
	}
	if !strings.Contains(readBack(t, s, rel), "title: Two") {
		t.Error("the file should still hold the edit")
	}
}

// TestStatusReportsWhatWorks so the panel offers only what it can do.
func TestStatusReportsWhatWorks(t *testing.T) {
	s, rel, _ := newEditor(t, true)
	got := decode(t, call(t, s, http.MethodGet, Path+"status", ""))
	if got["git"] != true {
		t.Errorf("status = %v", got)
	}
	if got["branch"] != "" {
		t.Error("a session that has saved nothing has no branch yet")
	}
	call(t, s, http.MethodPost, Path+"frontmatter", `{"path":"`+rel+`","key":"title","value":"Once"}`)
	got = decode(t, call(t, s, http.MethodGet, Path+"status", ""))
	if branch, _ := got["branch"].(string); !strings.HasPrefix(branch, "edit/") {
		t.Errorf("branch = %v", got["branch"])
	}
}

// TestHandlesOnlyOwnPaths keeps the editor out of the site's own URLs.
func TestHandlesOnlyOwnPaths(t *testing.T) {
	if !Handles(Path + "doc") {
		t.Error("its own path")
	}
	for _, p := range []string{"/", "/blog/", "/__livereload", "/edit/"} {
		if Handles(p) {
			t.Errorf("%s is not the editor's", p)
		}
	}
}

// TestBranchFromReadsTheReply, and falls back to what it asked for.
func TestBranchFromReadsTheReply(t *testing.T) {
	if got := branchFrom(`switched to branch "edit/2026-09-10-120000".`, "2026-09-10-120000"); got != "edit/2026-09-10-120000" {
		t.Errorf("got %q", got)
	}
	if got := branchFrom("something else entirely", "2026-09-10"); got != "2026-09-10" {
		t.Errorf("fallback = %q", got)
	}
}

// TestControlFor picks a control from a declared type, or from the name when
// nothing is declared.
func TestControlFor(t *testing.T) {
	cases := map[[2]string]string{
		{"anything", "enum"}: "enum",
		{"description", ""}:  "text",
		{"date", ""}:         "date",
		{"tags", ""}:         "list",
		{"sticky", ""}:       "bool",
		{"whatever", ""}:     "string",
	}
	for in, want := range cases {
		if got := controlFor(in[0], in[1]); got != want {
			t.Errorf("controlFor(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// TestFormWithoutASchema: a site with no content contracts still gets a form.
func TestFormWithoutASchema(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	s.opts.Schemas = nil
	var got docResponse
	w := call(t, s, http.MethodGet, Path+"doc?path="+rel, "")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Fields) == 0 {
		t.Fatal("no fields")
	}
	for _, f := range got.Fields {
		if f.Required {
			t.Errorf("%s cannot be required without a schema", f.Name)
		}
	}
	// And a value still saves, typed as the text it arrived as.
	if w := call(t, s, http.MethodPost, Path+"frontmatter",
		`{"path":"`+rel+`","key":"title","value":"Untyped"}`); w.Code != http.StatusOK {
		t.Errorf("%d: %s", w.Code, w.Body.String())
	}
}

// TestScriptCarriesTheTokenSafely: it is embedded, quoted, and cannot close
// the script element it lives in.
func TestScriptCarriesTheTokenSafely(t *testing.T) {
	s := Script(`ab"c</script><script>alert(1)`)
	if strings.Contains(s, "</script><script>alert") {
		t.Errorf("the token broke out of the script:\n%s", s)
	}
	if !strings.Contains(s, `\"`) || !strings.Contains(s, `\x3c`) {
		t.Errorf("the token was not escaped:\n%s", s)
	}
	if !strings.Contains(Script("plain"), `var TOKEN="plain"`) {
		t.Error("an ordinary token should be embedded as-is")
	}
	if strings.Contains(Script("plain"), "__SSG_EDIT_TOKEN__") {
		t.Error("the placeholder survived")
	}
}

// TestTokenFromEnv lets a session be scripted.
func TestTokenFromEnv(t *testing.T) {
	t.Setenv("SSG_EDIT_TOKEN", "chosen")
	if TokenFromEnv() != "chosen" {
		t.Error("the environment token should be read")
	}
}

// TestStringOfShapes: what a frontmatter value looks like in a text box.
func TestStringOfShapes(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want string
	}{
		"nil":    {nil, ""},
		"string": {"x", "x"},
		"bool":   {true, "true"},
		"int":    {3, "3"},
		"whole":  {float64(4), "4"},
		"frac":   {1.5, "1.5"},
		"list":   {[]interface{}{"a", "b"}, "a, b"},
		"strs":   {[]string{"a", "b"}, "a, b"},
		"other":  {[]int{1}, "[1]"},
	}
	for name, c := range cases {
		if got := stringOf(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	// A nested block is shown, not offered for editing in a text box.
	if got := stringOf(map[string]interface{}{"a": 1, "b": 2}); !strings.Contains(got, "2 nested keys") {
		t.Errorf("nested = %q", got)
	}
}

// TestDescribeSummarisesAnEdit for the commit message, without letting a long
// value run away with it.
func TestDescribeSummarisesAnEdit(t *testing.T) {
	if got := describe(frontmatterRequest{Key: "title", Value: "Short"}); got != "title = Short" {
		t.Errorf("got %q", got)
	}
	if got := describe(frontmatterRequest{Key: "tags", Unset: true}); got != "tags removed" {
		t.Errorf("got %q", got)
	}
	long := describe(frontmatterRequest{Key: "description", Value: strings.Repeat("x", 200)})
	if len(long) > 80 || !strings.HasSuffix(long, "…") {
		t.Errorf("a long value should be cut: %d chars", len(long))
	}
}

// TestDocRefusesAFileWithBrokenFrontmatter rather than offering a form that
// would save over it.
func TestDocRefusesAFileWithBrokenFrontmatter(t *testing.T) {
	s, _, _ := newEditor(t, false)
	text, isErr := s.opts.MCP.Call("content_create", map[string]any{
		"path": "content/site/posts/news/broken.md", "content": "---\ntitle: x\n\nno closing fence\n"})
	if isErr {
		t.Fatal(text)
	}
	w := call(t, s, http.MethodGet, Path+"doc?path=content/site/posts/news/broken.md", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("%d: %s", w.Code, w.Body.String())
	}
	// And a save against it fails at the write rather than corrupting it.
	w = call(t, s, http.MethodPost, Path+"frontmatter",
		`{"path":"content/site/posts/news/broken.md","key":"title","value":"y"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("save: %d %s", w.Code, w.Body.String())
	}
}

// TestDocOnAFileWithNoFrontmatter: a bare Markdown file has an empty form and
// a save that explains itself.
func TestDocOnAFileWithNoFrontmatter(t *testing.T) {
	s, _, _ := newEditor(t, false)
	if text, isErr := s.opts.MCP.Call("content_create", map[string]any{
		"path": "content/site/posts/news/bare.md", "content": "# Just a heading\n"}); isErr {
		t.Fatal(text)
	}
	w := call(t, s, http.MethodGet, Path+"doc?path=content/site/posts/news/bare.md", "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	w = call(t, s, http.MethodPost, Path+"frontmatter",
		`{"path":"content/site/posts/news/bare.md","key":"title","value":"y"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("save: %d %s", w.Code, w.Body.String())
	}
}

// TestValidateWithoutAFieldRule: a key the schema does not mention is written
// as the text it arrived as, rather than guessed at.
func TestValidateWithoutAFieldRule(t *testing.T) {
	s, _, _ := newEditor(t, false)
	got, err := s.validate("post", "undeclared", "2026")
	if err != nil || got != "2026" {
		t.Errorf("got %#v, %v — an undeclared field stays text", got, err)
	}
	if got, err := s.validate("unknown-type", "anything", "true"); err != nil || got != "true" {
		t.Errorf("got %#v, %v", got, err)
	}
}

// TestBlockEndpointResolvesRenderedTextToSource (GO-102 phase 2).
func TestBlockEndpointResolvesRenderedTextToSource(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	if text, isErr := s.opts.MCP.Call("content_update", map[string]any{
		"path": rel, "content": samplePost + "\nAnother **paragraph**.\n\nThe body.\n"}); isErr {
		t.Fatal(text)
	}
	w := call(t, s, http.MethodGet, Path+"block?path="+rel+"&text="+url.QueryEscape("Another paragraph."), "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	got := decode(t, w)
	if got["source"] != "Another **paragraph**." || got["kind"] != "paragraph" {
		t.Errorf("block = %v", got)
	}
	// Text that is not a block of the source is refused, not guessed at.
	w = call(t, s, http.MethodGet, Path+"block?path="+rel+"&text="+url.QueryEscape("Read more"), "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("%d: %s", w.Code, w.Body.String())
	}
	// The arguments both matter.
	if w := call(t, s, http.MethodGet, Path+"block?path="+rel, ""); w.Code != http.StatusBadRequest {
		t.Errorf("no text: %d", w.Code)
	}
	if w := call(t, s, http.MethodGet, Path+"block?text=x", ""); w.Code != http.StatusBadRequest {
		t.Errorf("no path: %d", w.Code)
	}
	if w := call(t, s, http.MethodGet, Path+"block?path=content/site/gone.md&text=x", ""); w.Code != http.StatusNotFound {
		t.Errorf("missing file: %d", w.Code)
	}
}

// TestBodyEndpointSavesThroughContentEdit, and inherits its refusal.
func TestBodyEndpointSavesThroughContentEdit(t *testing.T) {
	s, rel, git := newEditor(t, true)
	if text, isErr := s.opts.MCP.Call("content_update", map[string]any{
		"path": rel, "content": samplePost + "\nSecond paragraph.\n"}); isErr {
		t.Fatal(text)
	}
	w := call(t, s, http.MethodPost, Path+"body",
		`{"path":"`+rel+`","old":"Second paragraph.","new":"Second paragraph, edited."}`)
	if w.Code != http.StatusOK || decode(t, w)["saved"] != true {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(readBack(t, s, rel), "Second paragraph, edited.") {
		t.Error("the edit did not land")
	}
	if !strings.Contains(strings.Join(git.calls, " "), "commit") {
		t.Errorf("git calls = %v", git.calls)
	}
	// An anchor that appears twice is refused by content_edit, with the count.
	if text, isErr := s.opts.MCP.Call("content_update", map[string]any{
		"path": rel, "content": samplePost + "\nSame.\n\nSame.\n"}); isErr {
		t.Fatal(text)
	}
	w = call(t, s, http.MethodPost, Path+"body", `{"path":"`+rel+`","old":"Same.","new":"Changed."}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("%d: %s", w.Code, w.Body.String())
	}
	if msg, _ := decode(t, w)["error"].(string); !strings.Contains(msg, "2 times") {
		t.Errorf("the refusal should say the count: %q", msg)
	}
	// The request shapes that are simply wrong.
	if w := call(t, s, http.MethodGet, Path+"body", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"body", "{nope"); w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"body", `{"path":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("no anchor: %d", w.Code)
	}
	w = call(t, s, http.MethodPost, Path+"body", `{"path":"`+rel+`","old":"Same.","new":"Same."}`)
	if w.Code != http.StatusOK || decode(t, w)["saved"] != false {
		t.Errorf("an unchanged block should report nothing changed: %s", w.Body.String())
	}
}

// TestAIActionsProposeAndNeverSave (GO-102 phase 3).
func TestAIActionsProposeAndNeverSave(t *testing.T) {
	s, rel, _ := newEditor(t, false)
	before := readBack(t, s, rel)
	var asked string
	s.opts.AI = func(question string, _ time.Duration) (string, error) {
		asked = question
		return "  A shorter line.  ", nil
	}
	s.opts.ExcerptLimit = 160

	w := call(t, s, http.MethodPost, Path+"ai", `{"action":"shorten","text":"A very long line indeed."}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	got := decode(t, w)
	if got["proposal"] != "A shorter line." {
		t.Errorf("proposal = %v (it should be trimmed)", got["proposal"])
	}
	if !strings.Contains(asked, "160 characters") || !strings.Contains(asked, "A very long line indeed.") {
		t.Errorf("the prompt did not carry the budget and the text: %q", asked)
	}
	if readBack(t, s, rel) != before {
		t.Error("an AI action must not write to the file")
	}

	// A translation carries its target language.
	call(t, s, http.MethodPost, Path+"ai", `{"action":"translate","text":"Hello","lang":"Polish"}`)
	if !strings.Contains(asked, "Polish") {
		t.Errorf("prompt = %q", asked)
	}
	// An explicit limit overrides the action's own.
	call(t, s, http.MethodPost, Path+"ai", `{"action":"title","text":"x","limit":42}`)
	if !strings.Contains(asked, "42 characters") {
		t.Errorf("prompt = %q", asked)
	}
}

// TestAIActionErrors: no model, an unknown action, no text, and a model that
// does not answer.
func TestAIActionErrors(t *testing.T) {
	s, _, _ := newEditor(t, false)
	if w := call(t, s, http.MethodPost, Path+"ai", `{"action":"shorten","text":"x"}`); w.Code != http.StatusNotImplemented {
		t.Errorf("no model: %d", w.Code)
	}
	s.opts.AI = func(string, time.Duration) (string, error) { return "", errAIUnavailable }
	if w := call(t, s, http.MethodPost, Path+"ai", `{"action":"shorten","text":"x"}`); w.Code != http.StatusBadGateway {
		t.Errorf("a model that fails: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"ai", `{"action":"nonsense","text":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("unknown action: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"ai", `{"action":"shorten","text":"  "}`); w.Code != http.StatusBadRequest {
		t.Errorf("no text: %d", w.Code)
	}
	if w := call(t, s, http.MethodGet, Path+"ai", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, Path+"ai", "{nope"); w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", w.Code)
	}
}

// errAIUnavailable stands in for a model that does not answer.
var errAIUnavailable = fmt.Errorf("connection refused")

// TestStatusDescribesTheAIButtons so the panel and the server cannot disagree
// about what exists.
func TestStatusDescribesTheAIButtons(t *testing.T) {
	s, _, _ := newEditor(t, false)
	got := decode(t, call(t, s, http.MethodGet, Path+"status", ""))
	if got["ai"] != false {
		t.Errorf("ai = %v", got["ai"])
	}
	actions, _ := got["aiActions"].([]any)
	if len(actions) != 4 {
		t.Fatalf("actions = %v", got["aiActions"])
	}
	first, _ := actions[0].(map[string]any)
	if first["name"] != "shorten" || first["label"] == "" {
		t.Errorf("first action = %v", first)
	}
	s.opts.AI = func(string, time.Duration) (string, error) { return "x", nil }
	if got := decode(t, call(t, s, http.MethodGet, Path+"status", "")); got["ai"] != true {
		t.Error("a configured model should be reported")
	}
}
