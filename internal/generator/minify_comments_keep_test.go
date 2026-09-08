package generator

// minify_all deleted every comment but the conditional one, which silently
// undid the remedy #256 had just documented (#263).

import (
	"strings"
	"testing"
)

// TestMinifyKeepsHostDirectives: the comments something downstream is meant to
// read survive, on a build with minification on — which is every production
// build.
func TestMinifyKeepsHostDirectives(t *testing.T) {
	kept := []string{
		"<!--email_off-->",
		"<!--/email_off-->",
		"<!-- email_off -->", // a formatter's spacing is the same directive
		"<!--[if lt IE 9]><p>old</p><![endif]-->",
		`<!--#include virtual="/header.html" -->`,
		`<!--esi <esi:include src="/x"/> -->`,
		"<!--googleoff: index-->",
		"<!--noindex-->",
	}
	for _, c := range kept {
		html := "<html><head>" + c + "</head><body> <p>x</p> </body></html>"
		if out := minifyHTMLString(html, nil); !strings.Contains(out, strings.TrimSpace(c)) {
			t.Errorf("minify dropped %q:\n%s", c, out)
		}
	}
}

// TestMinifyStillDropsOrdinaryComments: the feature is still minification. A
// note to the author is not a directive and goes, as it always did.
func TestMinifyStillDropsOrdinaryComments(t *testing.T) {
	html := "<html><body><!-- TODO: rewrite this section --><p>x</p></body></html>"
	out := minifyHTMLString(html, nil)

	if strings.Contains(out, "TODO") {
		t.Errorf("an ordinary comment survived:\n%s", out)
	}
	if !strings.Contains(out, "<p>x</p>") {
		t.Errorf("content was damaged:\n%s", out)
	}
}

// TestMinifyKeepCommentsIsConfigurable: a host that invents another directive
// should not need a release.
func TestMinifyKeepCommentsIsConfigurable(t *testing.T) {
	html := "<html><body><!--acme:begin--><p>x</p><!--acme:end--></body></html>"

	if out := minifyHTMLString(html, nil); strings.Contains(out, "acme") {
		t.Errorf("an unknown directive survived by default:\n%s", out)
	}
	// Named as the directive, or as the whole opening — both are the same ask.
	for _, keep := range [][]string{{"acme:"}, {"<!--acme:"}} {
		out := minifyHTMLString(html, keep)
		if !strings.Contains(out, "<!--acme:begin-->") || !strings.Contains(out, "<!--acme:end-->") {
			t.Errorf("keep %v did not preserve the directive:\n%s", keep, out)
		}
	}
	// An empty entry is not a prefix that matches everything.
	if out := minifyHTMLString("<html><body><!-- note --><p>x</p></body></html>", []string{"", "   "}); strings.Contains(out, "note") {
		t.Errorf("an empty keep entry kept everything:\n%s", out)
	}
}

// TestKeepsHTMLCommentIsPrefixMatched: a directive carries arguments, so the
// rule matches how a comment opens rather than what it says in full.
func TestKeepsHTMLCommentIsPrefixMatched(t *testing.T) {
	if !keepsHTMLComment(`<!--#echo var="DATE_LOCAL" -->`, nil) {
		t.Error("an SSI directive with arguments is not recognised")
	}
	if keepsHTMLComment("<!-- this mentions email_off in passing -->", nil) {
		t.Error("a comment merely containing a directive name was kept")
	}
}
