package generator

import (
	"html/template"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apipage"
)

// Template functions for themes that draw their own API pages (GO-109).
// The bundled themes do not call them — a theme in the repository is also
// built by older ssg releases, which would not know them — and use the
// precomputed .api.nav and .api.crumbs instead.

// tmplAPIHref is {{ apiHref "pkg/mod#Name" }}: the URL of a documented
// symbol, or "" when it is not one.
func (g *Generator) tmplAPIHref(id string) string {
	if g.apiHref == nil {
		return ""
	}
	return g.apiHref(id)
}

// tmplAPIType is {{ apiType .Type }}: a type as HTML with documented names
// linked.
func (g *Generator) tmplAPIType(t *apimodel.TypeRef) template.HTML {
	// #nosec G203 -- TypeHTML escapes every name and literal it writes
	return template.HTML(apipage.TypeHTML(t, g.apiHref))
}
