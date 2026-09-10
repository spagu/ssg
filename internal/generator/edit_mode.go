package generator

// What a page needs to be editable in a browser (GO-102, phase 1).
//
// Two small things, and one of them is a subtraction.
//
// A theme marks the regions it is willing to have edited, with attributes on
// the elements that carry them: `data-ssg-edit="frontmatter:title"` on the
// heading, and so on. Those attributes are for the editor, not for a reader,
// so a published build takes them straight back out — a site does not ship the
// scaffolding of a tool it is not running. That is why the corpora do not move
// a byte when a theme gains them.
//
// The other is the marker that lets the browser say WHICH document it is
// looking at. Only the edit server needs it, and a published page must not
// carry it: the path of a file inside the author's repository is the project's
// structure, not the site's content — the same line GO-095 draws.

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// editAttr is the attribute a theme marks an editable region with.
const editAttr = "data-ssg-edit"

// editAttrRe matches one such attribute together with the single space in
// front of it, so removing it restores the byte-for-byte original.
var editAttrRe = regexp.MustCompile(`\s` + editAttr + `="[^"]*"`)

// stripEditAttrs removes the editing scaffolding from a published page.
func stripEditAttrs(s string) string {
	if !strings.Contains(s, editAttr) {
		return s
	}
	return editAttrRe.ReplaceAllString(s, "")
}

// editSourceMeta is the marker `ssg serve --edit` injects so the browser can
// name the document it is showing. Listings and archives have no single source
// file and get none, which is also how the editor knows not to offer a form.
func editSourceMeta(sourceDir, sourceFile, kind string) string {
	if sourceFile == "" {
		return ""
	}
	rel := sourceFile
	if sourceDir != "" {
		rel = strings.TrimSuffix(sourceDir, "/") + "/" + sourceFile
	}
	return fmt.Sprintf("\n<meta name=\"ssg:source\" content=%q>\n<meta name=\"ssg:type\" content=%q>",
		html.EscapeString(rel), html.EscapeString(kind))
}

// injectEditSource places the marker in the document head.
func injectEditSource(s, meta string) string {
	if meta == "" {
		return s
	}
	if i := strings.Index(s, "</head>"); i >= 0 {
		return s[:i] + meta + "\n" + s[i:]
	}
	return s
}
