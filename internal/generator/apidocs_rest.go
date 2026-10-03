package generator

// REST API reference from OpenAPI (1.8.69, GO-117/GO-118): an api_docs entry
// with `openapi:` is read into pages the same way a code package is — the
// same layouts, navigation shape and check_api reporting — with a "Try it"
// console on each operation unless try_it is false.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/apipage"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/openapi"
	"github.com/spagu/ssg/internal/openapipage"
)

// addRESTPages reads one OpenAPI file and adds its pages to the site. A file
// that cannot be read at all fails the build; problems inside it are
// check_api findings.
func (g *Generator) addRESTPages(opt APIDocsOptions) error {
	spec, diags, err := openapi.Load(opt.OpenAPI)
	if err != nil {
		return fmt.Errorf("api_docs openapi %s: %w", opt.OpenAPI, err)
	}
	for _, d := range diags {
		g.apiDiagnostics = append(g.apiDiagnostics, apisource.Diagnostic{Severity: apisource.Warning,
			File: opt.OpenAPI, Message: d.String()})
	}
	name := openapipage.Name(spec)
	if opt.Source.Name != "" {
		name = opt.Source.Name
	}
	base := opt.URL
	if base == "" {
		base = "/api/" + openapipage.Slug(firstNonEmpty(opt.Source.Name, strings.TrimSpace(spec.Title),
			strings.TrimSuffix(filepath.Base(opt.OpenAPI), filepath.Ext(opt.OpenAPI)))) + "/"
	}
	popts := openapipage.Options{Base: base, TryIt: opt.TryIt}
	nav := openapipage.Nav(spec, popts)
	for _, p := range openapipage.Build(spec, popts) {
		page := referencePage(apipage.Page{URL: p.URL, Title: p.Title, Summary: p.Summary, Markdown: p.Markdown, Layout: p.Layout},
			g.config.PageFormat, map[string]interface{}{
				"package": map[string]interface{}{"Name": name, "Version": spec.APIVersion},
				"rest":    true,
				"nav":     nav,
				"crumbs":  openapipage.Crumbs(spec, p, popts),
			})
		g.siteData.Pages = append(g.siteData.Pages, page)
		g.apiPageCount++
	}
	return nil
}
