// Package openapipage turns an OpenAPI document into documentation pages
// (GO-117): a front page with servers, authentication and every endpoint, a
// page per tag with its operations, and a page of schemas. Pages are
// Markdown with a little raw HTML — anchors, method labels and the marker the
// "Try it" console grows from (GO-118) — so they render in every theme and
// travel through search, llms.txt and markdown_publish like any page.
package openapipage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apipage"
	"github.com/spagu/ssg/internal/openapi"
)

// Options shape the pages of one API.
type Options struct {
	Base  string // URL prefix, e.g. "/api/petstore/"
	TryIt bool   // add the Try it console to every operation
}

// Page is one page to publish. Layout reuses the code reference's roles, so a
// theme with api-* layouts shows a REST API the same way.
type Page struct {
	URL      string
	Title    string
	Summary  string
	Markdown string
	Layout   string
}

// site holds what every page needs: the spec, options and URLs.
type site struct {
	spec    *openapi.Spec
	opts    Options
	tagURL  map[string]string
	schemas map[string]*openapi.Schema
}

func newSite(spec *openapi.Spec, opts Options) *site {
	s := &site{spec: spec, opts: opts, tagURL: map[string]string{}, schemas: map[string]*openapi.Schema{}}
	used := map[string]bool{"schemas": true}
	for _, t := range spec.Tags {
		slug := Slug(t.Name)
		for used[slug] {
			slug += "-tag"
		}
		used[slug] = true
		s.tagURL[t.Name] = opts.Base + slug + "/"
	}
	for _, n := range spec.Schemas {
		s.schemas[n.Name] = n.Schema
	}
	return s
}

// Name is how the API is called in titles and navigation.
func Name(spec *openapi.Spec) string {
	if strings.TrimSpace(spec.Title) != "" {
		return strings.TrimSpace(spec.Title)
	}
	return "REST API"
}

// schemasURL is the schemas page.
func (s *site) schemasURL() string { return s.opts.Base + "schemas/" }

// schemaHref links to one schema on the schemas page.
func (s *site) schemaHref(name string) string { return s.schemasURL() + "#" + schemaAnchor(name) }

// Build returns the API's pages in URL order.
func Build(spec *openapi.Spec, opts Options) []Page {
	s := newSite(spec, opts)
	pages := []Page{s.indexPage()}
	for _, t := range spec.Tags {
		pages = append(pages, s.tagPage(t))
	}
	if len(spec.Schemas) > 0 {
		pages = append(pages, s.schemasPage())
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].URL < pages[j].URL })
	return pages
}

// Nav is the tree beside every page: overview, tags, schemas.
func Nav(spec *openapi.Spec, opts Options) []apipage.NavItem {
	s := newSite(spec, opts)
	nav := []apipage.NavItem{{Title: "Overview", URL: opts.Base, Kind: "index"}}
	for _, t := range spec.Tags {
		nav = append(nav, apipage.NavItem{Title: t.Name, URL: s.tagURL[t.Name], Kind: "tag"})
	}
	if len(spec.Schemas) > 0 {
		nav = append(nav, apipage.NavItem{Title: "Schemas", URL: s.schemasURL(), Kind: "schemas"})
	}
	return nav
}

// Crumbs is the trail to a page: the API, then the page itself.
func Crumbs(spec *openapi.Spec, p Page, opts Options) []apipage.NavItem {
	crumbs := []apipage.NavItem{{Title: Name(spec), URL: opts.Base, Kind: "index"}}
	if p.URL != opts.Base {
		crumbs = append(crumbs, apipage.NavItem{Title: strings.TrimSuffix(p.Title, " — "+Name(spec)), URL: p.URL})
	}
	return crumbs
}

// indexPage is the front page: description, servers, authentication and the
// list of endpoints by tag.
func (s *site) indexPage() Page {
	w := &writer{site: s}
	w.text(s.spec.Summary)
	w.text(s.spec.Description)
	if v := s.spec.APIVersion; v != "" {
		w.line("**Version** %s · OpenAPI %s\n", v, s.spec.Version)
	}
	if len(s.spec.Servers) > 0 {
		w.heading(2, "servers", "Servers")
		for _, sv := range s.spec.Servers {
			w.line("- `%s`%s", sv.URL, suffix(" — ", sv.Description))
			for _, name := range sortedKeys(sv.Variables) {
				v := sv.Variables[name]
				w.line("  - `{%s}` default `%s`%s%s", name, v.Default, enumNote(v.Enum), suffix(" — ", v.Description))
			}
		}
		w.line("")
	}
	if len(s.spec.SecuritySchemes) > 0 {
		w.heading(2, "authentication", "Authentication")
		for _, sc := range s.spec.SecuritySchemes {
			w.line("- **%s** — %s%s", sc.Name, describeScheme(sc), suffix(". ", oneLine(sc.Description)))
		}
		w.line("")
	}
	w.heading(2, "endpoints", "Endpoints")
	for _, t := range s.spec.Tags {
		w.line("**[%s](%s)**%s\n", t.Name, s.tagURL[t.Name], suffix(" — ", oneLine(t.Description)))
		ops := s.opsOf(t.Name)
		if len(ops) == 0 {
			w.line("No operations yet.\n")
			continue
		}
		w.line("| Method | Path | Summary |\n|---|---|---|")
		for _, op := range ops {
			w.line("| %s | [`%s`](%s) | %s |", methodLabel(op.Method), cell(op.Path), s.tagURL[t.Name]+"#"+Slug(op.ID), cell(op.Summary))
		}
		w.line("")
	}
	if len(s.spec.Schemas) > 0 {
		w.line("Data types: [Schemas](%s).\n", s.schemasURL())
	}
	return Page{URL: s.opts.Base, Title: Name(s.spec), Summary: summary(s.spec.Summary, s.spec.Description,
		Name(s.spec)+" REST API reference."), Markdown: w.String(), Layout: apipage.LayoutIndex}
}

// tagPage holds every operation of one tag.
func (s *site) tagPage(t openapi.Tag) Page {
	w := &writer{site: s}
	w.text(t.Description)
	ops := s.opsOf(t.Name)
	if len(ops) == 0 {
		w.line("No operations yet.\n")
	}
	for _, op := range ops {
		w.operation(op)
	}
	return Page{URL: s.tagURL[t.Name], Title: t.Name + " — " + Name(s.spec),
		Summary:  summary(t.Description, "", fmt.Sprintf("The %s endpoints of %s.", t.Name, Name(s.spec))),
		Markdown: w.String(), Layout: apipage.LayoutModule}
}

// schemasPage describes every named schema.
func (s *site) schemasPage() Page {
	w := &writer{site: s}
	for _, n := range s.spec.Schemas {
		w.heading(2, schemaAnchor(n.Name), "<code>"+escape(n.Name)+"</code>")
		w.schema(n.Schema)
	}
	return Page{URL: s.schemasURL(), Title: "Schemas — " + Name(s.spec),
		Summary: fmt.Sprintf("The data types %s sends and receives.", Name(s.spec)), Markdown: w.String(), Layout: apipage.LayoutModule}
}

// opsOf lists the operations carrying a tag.
func (s *site) opsOf(tag string) []*openapi.Operation {
	var out []*openapi.Operation
	for _, op := range s.spec.Operations {
		for _, t := range op.Tags {
			if t == tag {
				out = append(out, op)
				break
			}
		}
	}
	return out
}
