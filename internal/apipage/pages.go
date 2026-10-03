package apipage

import (
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// Options shape the pages of one package.
type Options struct {
	Base string // URL prefix, e.g. "/api/core/"
	// Visibility: "public" (default) hides internal, private and hidden
	// symbols; "internal" shows internal ones; "all" shows everything.
	Visibility string
	// Stability lists the levels to show besides stable ("beta", "alpha",
	// "experimental"); empty shows them all.
	Stability []string
	// Links rewrites inline links in documentation Markdown written in module
	// (the page's module ID, "" on the package index); nil leaves them.
	Links func(md, module string) string
	// SourceURL turns a definition's location into a link; nil adds none.
	SourceURL func(*apimodel.Source) string
	// Readme shows the package README on its index page.
	Readme bool
	// Playground is the URL of the package as an ES module; set, every
	// JavaScript @example becomes editable and runnable in the browser (GO-116).
	Playground string

	pkgName string // set by Build: the import name the playground maps
}

// links applies the configured link rewriter for a module.
func (o *Options) links(md, module string) string {
	if o.Links == nil {
		return md
	}
	return o.Links(md, module)
}

// Layout names the theme template a page asks for; a theme without it
// renders the page through page.html.
const (
	LayoutIndex  = "api-index"
	LayoutModule = "api-module"
	LayoutSymbol = "api-symbol"
)

// Page is one page to publish.
type Page struct {
	URL      string
	Title    string
	Summary  string // plain first paragraph, for description and listings
	Markdown string
	Layout   string
	Module   *apimodel.Module // the page's module; nil on the index
	Symbol   *apimodel.Symbol // the class or interface of a symbol page
}

// Build returns the pages of pkg: the index, every module, and every class
// and interface, in URL order. Hidden symbols are filtered out first, so they
// leave no page, no anchor and no listing behind.
func Build(pkg *apimodel.Package, opts Options) []Page {
	pkg = Visible(pkg, opts.Visibility, opts.Stability)
	opts.pkgName = pkg.Name
	ix := apimodel.NewIndex(&apimodel.API{Packages: []*apimodel.Package{pkg}})
	urls := NewURLs(opts.Base, ix)
	pages := []Page{indexPage(pkg, &opts, urls)}
	for _, m := range pkg.Modules {
		pages = append(pages, modulePage(pkg, m, &opts, urls))
		for _, s := range m.Symbols {
			if ownPage(s) {
				pages = append(pages, symbolPage(pkg, m, s, &opts, urls))
			}
		}
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].URL < pages[j].URL })
	return pages
}

// indexPage is the package's front page: README and the module list.
func indexPage(pkg *apimodel.Package, opts *Options, urls *URLs) Page {
	w := &writer{opts: opts, urls: urls}
	title := pkg.Name
	if pkg.Version != "" {
		title += " " + pkg.Version
	}
	if opts.Readme {
		w.text(stripFirstHeading(pkg.Readme))
	}
	w.line("## Modules\n")
	for _, m := range pkg.Modules {
		w.line("- [`%s`](%s)%s", m.Path, urls.Module(m.ID), docSuffix(w, m.Doc))
	}
	return Page{URL: urls.Index(), Title: title + " API", Summary: "API reference for " + title + ".",
		Markdown: w.b.String(), Layout: LayoutIndex}
}

// modulePage lists a module's exports and documents those without a page
// of their own.
func modulePage(pkg *apimodel.Package, m *apimodel.Module, opts *Options, urls *URLs) Page {
	w := &writer{opts: opts, urls: urls, module: m.ID}
	w.doc(m.Doc)
	for _, g := range groups(m.Symbols) {
		w.section(g.title)
		for _, s := range g.syms {
			if ownPage(s) {
				w.line("- [`%s`](%s)%s", s.Name, urls.Href(s.ID), summarySuffix(w, s))
				continue
			}
			w.symbol(3, s)
		}
		w.line("")
	}
	return Page{URL: urls.Module(m.ID), Title: "Module " + m.Path + " — " + pkg.Name,
		Summary: docSummary(m.Doc, "Module "+m.Path+" of "+pkg.Name+"."), Markdown: w.b.String(),
		Layout: LayoutModule, Module: m}
}

// symbolPage documents a class or interface and its members.
func symbolPage(pkg *apimodel.Package, m *apimodel.Module, s *apimodel.Symbol, opts *Options, urls *URLs) Page {
	w := &writer{opts: opts, urls: urls, module: m.ID}
	w.line("Module [`%s`](%s)\n", m.Path, urls.Module(m.ID))
	w.badges(s)
	for _, ext := range s.Extends {
		w.line("Extends `%s`\n", ext.String())
	}
	for _, impl := range s.Implements {
		w.line("Implements `%s`\n", impl.String())
	}
	w.doc(s.Doc)
	w.source(s.Source)
	for _, g := range groups(s.Members) {
		w.section(g.title)
		for _, mem := range g.syms {
			w.symbol(3, mem)
		}
	}
	return Page{URL: urls.Page(s.ID), Title: kindTitle(s.Kind) + " " + s.Name + " — " + pkg.Name,
		Summary: docSummary(s.Doc, string(s.Kind)+" "+s.Name+" in "+m.Path+"."), Markdown: w.b.String(),
		Layout: LayoutSymbol, Module: m, Symbol: s}
}

// docSummary is the doc's summary, or fallback.
func docSummary(d *apimodel.Doc, fallback string) string {
	if d != nil && d.Summary != "" {
		return d.Summary
	}
	return fallback
}

// docSuffix is " — summary" for a module list entry, or "".
func docSuffix(w *writer, d *apimodel.Doc) string {
	if d == nil || d.Summary == "" {
		return ""
	}
	return " — " + cell(w.links(d.Summary))
}

// stripFirstHeading drops a README's leading "# Title" line: the page has
// its own.
func stripFirstHeading(md string) string {
	md = strings.TrimSpace(md)
	if strings.HasPrefix(md, "# ") {
		_, rest, _ := strings.Cut(md, "\n")
		return rest
	}
	return md
}

// kindTitle is a kind as a word for a title: "class" → "Class".
func kindTitle(k apimodel.Kind) string {
	if k == "" {
		return ""
	}
	return strings.ToUpper(string(k[:1])) + string(k[1:])
}
