package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// RouteEntry is one entry in routes.json: a generated URL and its provenance.
type RouteEntry struct {
	Path   string `json:"path"`             // site-relative URL, e.g. /blog/my-post/
	Type   string `json:"type"`             // post | page | category | tag | series | author | <taxonomy> | <taxonomy>-index
	Title  string `json:"title,omitempty"`  // display title / term name
	Source string `json:"source,omitempty"` // source file, for posts and pages
	Lang   string `json:"lang,omitempty"`   // language bucket (i18n builds)
}

// RouteManifest is the routes.json document: a machine-readable contract of every
// route the build emits, for external tooling and typed clients (#62).
type RouteManifest struct {
	Count  int          `json:"count"`
	Routes []RouteEntry `json:"routes"`
}

// writeRouteManifest emits routes.json — every post, page and taxonomy archive —
// so a renamed slug or a changed route surfaces as a diff in a checked-in
// contract. A no-op unless route_manifest (or --route-manifest) is set (#62).
func (g *Generator) writeRouteManifest() error {
	if !g.config.RouteManifest {
		return nil
	}
	var routes []RouteEntry
	for _, p := range g.siteData.Posts {
		routes = append(routes, RouteEntry{Path: p.GetURL(), Type: "post", Title: p.Title, Source: p.SourceFile, Lang: p.Lang})
	}
	for _, p := range g.siteData.Pages {
		routes = append(routes, RouteEntry{Path: p.GetURL(), Type: "page", Title: p.Title, Source: p.SourceFile, Lang: p.Lang})
	}
	routes = append(routes, g.taxonomyRoutes()...)

	routes = dedupeRoutesByPath(routes)
	sort.Slice(routes, func(i, j int) bool { return routes[i].Path < routes[j].Path })

	doc := RouteManifest{Count: len(routes), Routes: routes}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out := filepath.Join(g.config.OutputDir, "routes.json")
	if err := g.ensureWithinOutput(out); err != nil {
		return err
	}
	g.log("🧭 Writing route manifest...")
	// #nosec G306 -- manifest is a public build artifact served alongside the site
	return os.WriteFile(out, append(data, '\n'), 0644)
}

// taxonomyRoutes enumerates the archive routes: the index page of each custom
// taxonomy (folded built-ins have none), each term archive, and the author
// archives (kept out of the registry, driven by the author slug map).
func (g *Generator) taxonomyRoutes() []RouteEntry {
	var out []RouteEntry
	if g.taxonomies != nil {
		for _, name := range g.taxonomies.Names {
			def := g.taxonomies.Definitions[name]
			if !def.Archive {
				continue
			}
			for _, lang := range g.taxonomyLangs() {
				base := g.termBase(def, lang)
				if !def.Folded { // custom taxonomies render a term-index page
					out = append(out, RouteEntry{Path: base, Type: def.Name + "-index", Lang: lang})
				}
				for _, t := range g.taxonomies.Terms(name, lang) {
					out = append(out, RouteEntry{Path: base + t.Slug + "/", Type: def.Name, Title: t.Name, Lang: lang})
				}
			}
		}
	}
	for slug := range g.authorSlugs {
		out = append(out, RouteEntry{Path: "/author/" + slug + "/", Type: "author"})
	}
	return out
}

// dedupeRoutesByPath keeps the first entry per path: folded built-ins render one
// language-agnostic archive, so per-language buckets can yield the same URL.
func dedupeRoutesByPath(routes []RouteEntry) []RouteEntry {
	seen := make(map[string]bool, len(routes))
	out := routes[:0]
	for _, r := range routes {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		out = append(out, r)
	}
	return out
}
