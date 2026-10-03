package golang

import (
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/printer"
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// linkScheme marks, in Markdown fresh from the comment printer, a doc link
// to rewrite as {@link target}.
const linkScheme = "ssg-link:"

// linkPattern finds the Markdown links the printer wrote for doc links.
var linkPattern = regexp.MustCompile(`\[((?:\\.|[^\]\\])*)\]\(` + linkScheme + `([^)]*)\)`)

// unescapePattern finds the backslash escapes of the Markdown printer.
var unescapePattern = regexp.MustCompile(`\\(.)`)

// pkgSite is where links to packages outside the extraction point.
const pkgSite = "https://pkg.go.dev"

// commenter turns doc comments into apimodel.Doc for one package: its
// parser knows the package's names and imports, so it recognises doc links.
type commenter struct {
	ix      *index
	parser  *comment.Parser
	printer *comment.Printer
}

// newCommenter returns the commenter of the module being built.
func (b *modBuilder) newCommenter() *commenter {
	return &commenter{ix: b.ix, parser: b.d.docPkg.Parser(), printer: &comment.Printer{
		DocLinkURL: b.linkURL,
		HeadingID:  func(*comment.Heading) string { return "" },
	}}
}

// linkURL is where a doc link points: a {@link} target (behind
// linkScheme) for a symbol of this extraction, pkg.go.dev for the rest, ""
// (plain text) for a package of this extraction.
func (b *modBuilder) linkURL(l *comment.DocLink) string {
	local := l.Name
	if l.Recv != "" {
		local = l.Recv + "." + l.Name
	}
	if l.ImportPath == "" || l.ImportPath == b.d.importPath {
		return linkScheme + local
	}
	d := b.ix.byPath[l.ImportPath]
	switch {
	case d == nil:
		return l.DefaultURL(pkgSite)
	case l.Name == "":
		return ""
	}
	return linkScheme + d.modPath + "#" + local
}

// doc parses a doc comment: the first paragraph is the summary, a
// "Deprecated:" paragraph the deprecation, the rest the body. nil when
// there is nothing to show.
func (c *commenter) doc(text string, examples []*doc.Example) *apimodel.Doc {
	out := &apimodel.Doc{}
	var body []string
	for i, blk := range c.parser.Parse(text).Content {
		md := c.block(blk)
		if _, ok := blk.(*comment.Paragraph); ok {
			if rest, found := strings.CutPrefix(md, "Deprecated: "); found {
				out.Deprecated = &rest
				continue
			}
			if i == 0 {
				out.Summary = md
				continue
			}
		}
		body = append(body, md)
	}
	out.Body = strings.Join(body, "\n\n")
	for _, ex := range examples {
		out.Examples = append(out.Examples, c.example(ex))
	}
	if out.Summary == "" && out.Body == "" && out.Deprecated == nil && len(out.Examples) == 0 {
		return nil
	}
	return out
}

// block renders one block of a comment as Markdown: code as a fenced Go
// block, the rest with doc links as {@link} targets.
func (c *commenter) block(blk comment.Block) string {
	if code, ok := blk.(*comment.Code); ok {
		return "```go\n" + strings.TrimRight(code.Text, "\n") + "\n```"
	}
	md := string(c.printer.Markdown(&comment.Doc{Content: []comment.Block{blk}}))
	return strings.TrimSpace(rewriteLinks(md))
}

// rewriteLinks turns the printer's marked links into {@link target}, with
// the link text when it differs from the target.
func rewriteLinks(md string) string {
	return linkPattern.ReplaceAllStringFunc(md, func(m string) string {
		sub := linkPattern.FindStringSubmatch(m)
		text, target := unescapePattern.ReplaceAllString(sub[1], "$1"), sub[2]
		if text == target {
			return "{@link " + target + "}"
		}
		return "{@link " + target + " | " + text + "}"
	})
}

// example renders an example function's body, its Output comment kept, as
// a fenced Go block.
func (c *commenter) example(ex *doc.Example) string {
	code := c.ix.print(&printer.CommentedNode{Node: ex.Code, Comments: ex.Comments})
	if _, ok := ex.Code.(*ast.BlockStmt); ok {
		code = unindent(strings.TrimSuffix(strings.TrimPrefix(code, "{"), "}"))
	}
	return "```go\n" + strings.Trim(code, "\n") + "\n```"
}

// unindent removes one tab from the start of every line.
func unindent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "\t")
	}
	return strings.Join(lines, "\n")
}
