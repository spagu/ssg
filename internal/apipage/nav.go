package apipage

import "github.com/spagu/ssg/internal/apimodel"

// NavItem is one entry of a package's navigation tree, precomputed so a
// theme renders it with a plain range — no template function a theme built
// by an older ssg would not know (the 1.8.65 lesson).
type NavItem struct {
	Title    string
	URL      string
	Kind     string // "index", "module", or a symbol kind
	Children []NavItem
}

// Nav is the package's tree: the index, then each module with its classes
// and interfaces.
func Nav(pkg *apimodel.Package, urls *URLs) []NavItem {
	nav := []NavItem{{Title: "Overview", URL: urls.Index(), Kind: "index"}}
	for _, m := range pkg.Modules {
		item := NavItem{Title: m.Path, URL: urls.Module(m.ID), Kind: "module"}
		for _, s := range m.Symbols {
			if ownPage(s) {
				item.Children = append(item.Children, NavItem{Title: s.Name, URL: urls.Href(s.ID), Kind: string(s.Kind)})
			}
		}
		nav = append(nav, item)
	}
	return nav
}

// Crumbs is the trail to a page: package, module, symbol (as far as known).
func Crumbs(pkg *apimodel.Package, p Page, urls *URLs) []NavItem {
	crumbs := []NavItem{{Title: pkg.Name, URL: urls.Index(), Kind: "index"}}
	if p.Module != nil {
		crumbs = append(crumbs, NavItem{Title: p.Module.Path, URL: urls.Module(p.Module.ID), Kind: "module"})
	}
	if p.Symbol != nil {
		crumbs = append(crumbs, NavItem{Title: p.Symbol.Name, URL: p.URL, Kind: string(p.Symbol.Kind)})
	}
	return crumbs
}
