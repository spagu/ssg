package dts

import (
	"strings"
	"testing"
)

// TestTokenize checks kinds, lines, line breaks and attached comments.
func TestTokenize(t *testing.T) {
	src := "\uFEFF// note\n/** Doc. */\nexport #x ...a => `t${ {b} }` 'q' \"d\" 1_0n\n/**/ y /* plain */ z"
	toks, err := tokenize(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind tokKind
		text string
		line int
		nl   bool
		doc  string
	}{
		{tokIdent, "export", 3, true, " Doc. "},
		{tokIdent, "#x", 3, false, ""},
		{tokPunct, "...", 3, false, ""},
		{tokIdent, "a", 3, false, ""},
		{tokPunct, "=>", 3, false, ""},
		{tokTemplate, "`t${ {b} }`", 3, false, ""},
		{tokString, "'q'", 3, false, ""},
		{tokString, `"d"`, 3, false, ""},
		{tokNumber, "1_0n", 3, false, ""},
		{tokIdent, "y", 4, true, ""},
		{tokIdent, "z", 4, false, ""},
		{tokEOF, "", 4, false, ""},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	for i, w := range want {
		g := toks[i]
		if g.kind != w.kind || g.text != w.text || g.line != w.line || g.nl != w.nl || g.doc != w.doc {
			t.Errorf("token %d = %+v, want %+v", i, g, w)
		}
	}
}

// TestTokenizeMultilineTemplate checks a template spanning lines keeps the
// line count right.
func TestTokenizeMultilineTemplate(t *testing.T) {
	toks, err := tokenize("`a\\`\nb` c")
	if err != nil {
		t.Fatal(err)
	}
	if toks[0].line != 1 || toks[1].line != 2 || toks[1].text != "c" {
		t.Errorf("tokens = %+v", toks)
	}
}

// TestTokenizeErrors checks unterminated literals and comments.
func TestTokenizeErrors(t *testing.T) {
	for _, src := range []string{`"abc`, "'a\nb'", "`abc", "/* open", `"a\"`} {
		if _, err := tokenize(src); err == nil {
			t.Errorf("tokenize(%q) did not fail", src)
		}
	}
	_, err := tokenize("a\n/* open")
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error = %v, want line 2", err)
	}
}

// TestTokenizeLineCommentAtEnd checks a file ending in a // comment.
func TestTokenizeLineCommentAtEnd(t *testing.T) {
	toks, err := tokenize("a // end")
	if err != nil || len(toks) != 2 {
		t.Fatalf("tokens = %+v, %v", toks, err)
	}
}

// TestTokenIs checks is ignores strings with the same text.
func TestTokenIs(t *testing.T) {
	if (token{kind: tokString, text: "a"}).is("a") {
		t.Error("a string token matched a word")
	}
	if !(token{kind: tokPunct, text: "{"}).is("{") {
		t.Error("punctuation did not match")
	}
}

// TestDescribe checks error wording for EOF and tokens.
func TestDescribe(t *testing.T) {
	if describe(token{kind: tokEOF}) != "end of file" || describe(token{kind: tokIdent, text: "x"}) != `"x"` {
		t.Error("describe wording changed")
	}
}
