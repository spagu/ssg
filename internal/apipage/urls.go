// Package apipage turns the API model into pages (GO-108): which pages exist,
// at which URLs, and their content as Markdown. The Markdown is the page's
// body in any theme — a theme that knows the api-* layouts renders richer
// views from the model instead, and every other theme still shows a complete
// reference through page.html.
//
// URLs (owner decision 4): a package index at <base>, one page per module,
// and a page of its own for each class and interface, whose members are
// anchors on it. Functions, types, enums and variables are anchors on their
// module's page.
package apipage

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// URLs maps IDs to page URLs for one package.
type URLs struct {
	base string // "/api/core/", always with both slashes
	ix   *apimodel.Index
}

// NewURLs builds the URL map for a package served under base.
func NewURLs(base string, ix *apimodel.Index) *URLs {
	base = "/" + strings.Trim(base, "/") + "/"
	if base == "//" {
		base = "/"
	}
	return &URLs{base: base, ix: ix}
}

// Index is the package's front page.
func (u *URLs) Index() string { return u.base }

// Module is the page of a module: <base><module path>/.
func (u *URLs) Module(moduleID string) string {
	_, path, _ := strings.Cut(moduleID, "/")
	return u.base + path + "/"
}

// ownPage reports whether a symbol has a page of its own.
func ownPage(s *apimodel.Symbol) bool {
	return s.Kind == apimodel.KindClass || s.Kind == apimodel.KindInterface
}

// Page is the URL of the page a symbol is shown on, without the anchor.
func (u *URLs) Page(id string) string {
	top := topLevel(id)
	if s, ok := u.ix.Symbol(top); ok && ownPage(s) {
		return u.Module(apimodel.ModuleOf(id)) + s.Name + "/"
	}
	return u.Module(apimodel.ModuleOf(id))
}

// Href is the full link to a symbol or member: its page plus the anchor,
// or the bare page for a class or interface itself.
func (u *URLs) Href(id string) string {
	page := u.Page(id)
	if s, ok := u.ix.Symbol(id); ok && ownPage(s) && topLevel(id) == id {
		return page
	}
	return page + "#" + u.Anchor(id)
}

// Anchor is the fragment of a symbol on its page. On a class's own page a
// member's anchor drops the class name: "#next", not "#Lexer.next".
func (u *URLs) Anchor(id string) string {
	top := topLevel(id)
	if s, ok := u.ix.Symbol(top); ok && ownPage(s) && top != id {
		return apimodel.Anchor(apimodel.ModuleOf(id) + "#" + strings.TrimPrefix(id, top+"."))
	}
	return apimodel.Anchor(id)
}

// topLevel is the ID of the top-level symbol an ID belongs to:
// "m#A.b.c" → "m#A".
func topLevel(id string) string {
	mod, local, ok := strings.Cut(id, "#")
	if !ok {
		return id
	}
	name, _, _ := strings.Cut(local, ".")
	return mod + "#" + name
}
