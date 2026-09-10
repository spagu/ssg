package generator

// Building the site graph from what the build already knows (GO-095).
//
// Nothing here is computed for the graph's sake. Pages, sections, taxonomies,
// redirects and translations are the build's own records; links are the
// href/src values the link checker was already parsing out of every output
// file and discarding once validated. The graph gathers them into one model —
// and routes.json and llms.txt are then generated FROM that model, which is
// the point: four manifests that could drift become views of one thing, and
// the golden corpora prove the views did not change by a byte.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/sitegraph"
)

// buildSiteGraph assembles the model. withLinks controls the one expensive
// part — parsing every output file — which only the artifact needs; the
// derived manifests do not.
func (g *Generator) buildSiteGraph(withLinks bool) sitegraph.Graph {
	graph := sitegraph.Graph{
		Schema: sitegraph.Schema,
		Build:  sitegraph.Build{Version: g.config.Version, Time: g.buildTime.UTC()},
		Domain: g.config.Domain,
	}
	// Posts before pages: routes.json has always listed them in that order,
	// and its byte-identity is the proof the derivation is faithful.
	for _, p := range g.siteData.Posts {
		graph.Pages = append(graph.Pages, g.graphPage(p, "post"))
	}
	for _, p := range g.siteData.Pages {
		graph.Pages = append(graph.Pages, g.graphPage(p, "page"))
	}
	graph.Sections = g.graphSections()
	graph.Taxonomies = g.graphTaxonomies()
	graph.Redirects = g.graphRedirects()
	if withLinks {
		graph.Links = g.graphLinks()
	}
	// Never nil in the artifact: a consumer iterates, it does not null-check.
	for _, slot := range []*[]sitegraph.Link{&graph.Links} {
		if *slot == nil {
			*slot = []sitegraph.Link{}
		}
	}
	if graph.Pages == nil {
		graph.Pages = []sitegraph.Page{}
	}
	if graph.Sections == nil {
		graph.Sections = []sitegraph.Section{}
	}
	if graph.Taxonomies == nil {
		graph.Taxonomies = []sitegraph.Taxonomy{}
	}
	if graph.Redirects == nil {
		graph.Redirects = []sitegraph.Redirect{}
	}
	return graph
}

// graphPage is one document as the graph describes it.
func (g *Generator) graphPage(p models.Page, kind string) sitegraph.Page {
	node := sitegraph.Page{
		URL:         p.GetURL(),
		Type:        kind,
		Title:       p.Title,
		Slug:        p.Slug,
		Lang:        p.Lang,
		Description: p.Description,
		Canonical:   g.servedCanonical(p),
		Image:       p.FeaturedImage,
		Tags:        p.Tags,
		Series:      p.Series,
		Taxonomies:  p.Taxonomies,
		WordCount:   p.WordCount,
		ReadingTime: p.ReadingTime,
		Sticky:      p.Sticky,
		Source:      p.SourceFile,
		Outputs:     sitegraph.Outputs{HTML: p.GetURL()},
	}
	if !p.Date.IsZero() {
		d := p.Date
		node.Date = &d
	}
	if !p.Modified.IsZero() {
		m := p.Modified
		node.Modified = &m
	}
	for _, id := range p.Categories {
		if cat, ok := g.siteData.Categories[id]; ok {
			node.Categories = append(node.Categories, cat.Name)
		}
	}
	if p.Category != "" && len(node.Categories) == 0 {
		node.Categories = []string{p.Category}
	}
	// The components this page renders (GO-093). Read from the page's own
	// content rather than from a per-build tally, because the graph describes
	// pages and a tally describes the build (GO-095 phase 2).
	node.Components = g.componentsInContent(p.Content)
	// Declared relations are edges an author wrote down, which is exactly what
	// a graph is for (GO-096).
	for _, name := range sortedKeys(p.RelatedPages) {
		for _, target := range p.RelatedPages[name] {
			node.Relations = append(node.Relations, sitegraph.Relation{Name: name, URL: target.GetURL()})
		}
	}
	for _, tr := range g.translationsFor(p) {
		if tr.IsCurrent {
			continue
		}
		node.Translations = append(node.Translations, sitegraph.Translation{Lang: tr.Lang, URL: tr.URL})
	}
	if g.config.MarkdownPublish {
		node.Outputs.Markdown = markdownURLFor(p, g.config.Domain)
	}
	if g.pageWantsOutput(p, "json") && strings.HasSuffix(node.URL, "/") {
		node.Outputs.JSON = node.URL + "index.json"
	}
	return node
}

// graphSections lists the generated listings: the archive routes, the author
// archives and the post listing(s). taxonomyRoutes is the same enumeration
// routes.json has always used, which is what keeps that file identical.
func (g *Generator) graphSections() []sitegraph.Section {
	var out []sitegraph.Section
	for _, r := range g.taxonomyRoutes() {
		out = append(out, sitegraph.Section{Path: r.Path, Kind: r.Type, Title: r.Title, Lang: r.Lang})
	}
	prefixes := make([]string, 0, len(g.postsListings))
	for prefix := range g.postsListings {
		if prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		out = append(out, sitegraph.Section{Path: "/" + prefix + "/", Kind: "listing"})
	}
	return out
}

// graphTaxonomies reads the registry: every definition with an archive, every
// term with its archive URL and count, per language under i18n.
func (g *Generator) graphTaxonomies() []sitegraph.Taxonomy {
	if g.taxonomies == nil {
		return nil
	}
	var out []sitegraph.Taxonomy
	for _, name := range g.taxonomies.Names {
		def := g.taxonomies.Definitions[name]
		if !def.Archive {
			continue
		}
		t := sitegraph.Taxonomy{Name: def.Name, Label: def.Label, Path: def.Path, Terms: []sitegraph.Term{}}
		for _, lang := range g.taxonomyLangs() {
			base := g.termBase(def, lang)
			for _, term := range g.taxonomies.Terms(name, lang) {
				t.Terms = append(t.Terms, sitegraph.Term{
					Name: term.Name, Slug: term.Slug, URL: base + term.Slug + "/", Lang: lang, Count: term.Count,
				})
			}
		}
		out = append(out, t)
	}
	return out
}

// graphRedirects is the merged rule set the host will apply, as _redirects
// gets it — aliases included, chains flattened.
func (g *Generator) graphRedirects() []sitegraph.Redirect {
	rules, _, err := g.collectRedirects()
	if err != nil {
		return nil
	}
	out := make([]sitegraph.Redirect, 0, len(rules))
	for _, r := range rules {
		out = append(out, sitegraph.Redirect{From: r.From, To: r.To, Status: r.Status})
	}
	return out
}

// graphLinks turns the parsed references of every output file into edges from
// the page URL that file serves. Internal references are resolved against the
// page they appear on, so "../x/" on /a/b/ becomes /a/x/; fragments and
// queries are dropped, because the target is the document, not a position in
// it. mailto:, tel:, data: and javascript: are not links to anywhere.
func (g *Generator) graphLinks() []sitegraph.Link {
	refs, _ := g.outputRefs() // an unreadable tree yields an empty link set; the link check reports it
	files := make([]string, 0, len(refs))
	for rel := range refs {
		files = append(files, rel)
	}
	sort.Strings(files)
	var out []sitegraph.Link
	for _, rel := range files {
		from := urlForOutputFile(rel)
		seen := map[string]bool{}
		for _, ref := range refs[rel] {
			to, kind := g.classifyRef(from, ref)
			if kind == "" {
				continue
			}
			key := kind + "|" + to
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, sitegraph.Link{From: from, To: to, Kind: kind})
		}
	}
	return out
}

// classifyRef resolves one reference to a graph target and its kind, or ""
// for a reference that is not a link to anywhere.
func (g *Generator) classifyRef(from, ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	lower := strings.ToLower(ref)
	switch {
	case ref == "", strings.HasPrefix(ref, "#"):
		return "", ""
	case strings.HasPrefix(lower, "mailto:"), strings.HasPrefix(lower, "tel:"),
		strings.HasPrefix(lower, "data:"), strings.HasPrefix(lower, "javascript:"):
		return "", ""
	}
	ref = g.stripOwnDomain(ref)
	if !isInternalRef(ref) {
		if strings.HasPrefix(ref, "//") {
			ref = "https:" + ref
		}
		return ref, "external"
	}
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" {
		return "", ""
	}
	if !strings.HasPrefix(ref, "/") {
		ref = path.Join(path.Dir(from), ref)
		if strings.HasSuffix(ref, "/") || !strings.Contains(path.Base(ref), ".") {
			ref = strings.TrimSuffix(ref, "/") + "/"
		}
	}
	ext := strings.ToLower(path.Ext(ref))
	if ext != "" && ext != ".html" && ext != ".htm" {
		return ref, "asset"
	}
	return ref, "page"
}

// outputRefs parses every output HTML file once per build and returns its
// href/src values keyed by output-relative path. The link checker and the
// graph both read this; the checker used to be the only reader and dropped
// the result after validating it.
func (g *Generator) outputRefs() (map[string][]string, error) {
	g.outputRefsMu.Lock()
	defer g.outputRefsMu.Unlock()
	if g.outputRefsCache != nil {
		return g.outputRefsCache, nil
	}
	refs := map[string][]string{}
	root := g.config.OutputDir
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if p == root {
				return err // no output tree at all is a real error
			}
			return nil // one unreadable entry is skipped, not fatal
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(p), ".html") {
			return nil
		}
		found, e := extractRefs(p)
		if e != nil {
			return nil // unreadable file is not a link error
		}
		rel, _ := filepath.Rel(root, p)
		refs[filepath.ToSlash(rel)] = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	g.outputRefsCache = refs
	g.outputRefsParses++
	return refs, nil
}

// resetOutputRefs clears the per-build cache, so a watch-mode rebuild parses
// the files this build wrote rather than reusing last build's references.
func (g *Generator) resetOutputRefs() {
	g.outputRefsMu.Lock()
	defer g.outputRefsMu.Unlock()
	g.outputRefsCache = nil
}

// writeSiteGraph emits the artifact when the site asked for it. Runs after the
// link check, so the two share one parse of the output.
func (g *Generator) writeSiteGraph() error {
	if !g.config.SiteGraph {
		return nil
	}
	g.log("🕸️  Writing site graph...")
	graph := g.buildSiteGraph(true)
	if err := graph.Stamp(); err != nil {
		return fmt.Errorf("site graph: %w", err)
	}
	files, err := sitegraph.Write(g.config.OutputDir, graph, 0)
	if err != nil {
		return fmt.Errorf("site graph: %w", err)
	}
	if !g.config.Quiet {
		fmt.Printf("   🕸️  %d pages, %d sections, %d links → %s (build %s)\n",
			len(graph.Pages), len(graph.Sections), len(graph.Links), strings.Join(files, ", "), graph.Build.Hash)
	}
	return nil
}
