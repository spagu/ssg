package mcp

// The site section: questions about the published site's MODEL, answered from
// the graph the last build wrote (GO-095), rather than from a directory walk.
//
// Every other section here is file-shaped — content_list, content_read,
// designer_edit — which is right for editing and wrong for understanding:
// "which pages link to /pricing/", "what taxonomies exist", "where does
// /old/ redirect" are questions about the site, and answering them by reading
// files means re-deriving what the build already knew. These tools read
// site-graph.json and say which build they are answering from, so an agent
// can tell a fresh answer from a stale one.
//
// Read-only by design. The graph is a model of the site, not a CMS; mutation
// stays with the file-shaped tools, which are the ones that know how.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spagu/ssg/internal/sitegraph"
)

// siteTools is the section, present whenever the server knows where the
// build writes. A missing graph is reported by every tool rather than by the
// section's absence, so an agent learns what to turn on instead of never
// seeing the tools at all.
func (s *Server) siteTools() []tool {
	return []tool{
		{
			name: "site_pages",
			description: "SITE · List published pages from the last build's site graph: url, type, " +
				"title, lang, date. Filter by `type` (post|page) and `lang`; page with `limit` " +
				"(default 100) and `offset`. Every answer names the `build` it comes from. " +
				"Needs `site_graph: true` in the config and a completed build.",
			schema: objectSchema(map[string]any{
				"type":   stringProp("Optional: post or page"),
				"lang":   stringProp("Optional: language code"),
				"limit":  intProp("Maximum entries (default 100, max 1000)"),
				"offset": intProp("Entries to skip"),
			}),
			handler: s.sitePages,
		},
		{
			name: "site_page",
			description: "SITE · One page in full — its metadata, translations, outputs, and every " +
				"link out of it and into it — by `url` (site-relative, e.g. \"/pricing/\").",
			schema:  objectSchema(map[string]any{"url": stringProp("Site-relative URL of the page")}, "url"),
			handler: s.sitePage,
		},
		{
			name: "site_links",
			description: "SITE · The link graph. Filter by `from` (page url), `to` (target), and " +
				"`kind` (page|asset|external); page with `limit` (default 200) and `offset`.",
			schema: objectSchema(map[string]any{
				"from":   stringProp("Optional: only links from this page URL"),
				"to":     stringProp("Optional: only links to this target"),
				"kind":   stringProp("Optional: page, asset or external"),
				"limit":  intProp("Maximum entries (default 200, max 5000)"),
				"offset": intProp("Entries to skip"),
			}),
			handler: s.siteLinks,
		},
		{
			name:        "site_taxonomies",
			description: "SITE · Every taxonomy with its terms: name, slug, archive URL, count.",
			schema:      objectSchema(nil),
			handler:     s.siteTaxonomies,
		},
		{
			name:        "site_redirects",
			description: "SITE · Every redirect the build published: from, to, status.",
			schema:      objectSchema(nil),
			handler:     s.siteRedirects,
		},
	}
}

// intProp is a JSON Schema integer property.
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// graphCache holds the last graph read, keyed on the artifact's modification
// time so a rebuild is picked up and an unchanged file is not re-parsed per
// call.
type graphCache struct {
	mu    sync.Mutex
	mtime time.Time
	graph sitegraph.Graph
}

// loadGraph returns the last build's graph, re-reading it only when the
// artifact changed.
func (s *Server) loadGraph() (sitegraph.Graph, error) {
	if s.opts.OutputDir == "" {
		return sitegraph.Graph{}, errors.New("the server was started without an output directory, so there is no site graph to read")
	}
	path := filepath.Join(s.opts.OutputDir, sitegraph.FileName)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sitegraph.Graph{}, sitegraph.ErrNotFound
		}
		return sitegraph.Graph{}, err
	}
	s.graphs.mu.Lock()
	defer s.graphs.mu.Unlock()
	if !s.graphs.mtime.IsZero() && info.ModTime().Equal(s.graphs.mtime) {
		return s.graphs.graph, nil
	}
	g, err := sitegraph.Load(s.opts.OutputDir)
	if err != nil {
		return sitegraph.Graph{}, err
	}
	s.graphs.graph, s.graphs.mtime = g, info.ModTime()
	return g, nil
}

// graphResult renders an answer with the build it came from.
func graphResult(build sitegraph.Build, key string, value any) toolResult {
	out, err := json.MarshalIndent(map[string]any{"build": build, key: value}, "", "  ")
	if err != nil {
		return errResult(err.Error())
	}
	return textResult(string(out))
}

// window applies limit/offset to a count and returns the bounds.
func window(args map[string]any, defaultLimit, maxLimit, n int) (int, int) {
	limit, offset := defaultLimit, 0
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v, ok := args["offset"].(float64); ok && v > 0 {
		offset = int(v)
	}
	if offset > n {
		offset = n
	}
	end := offset + limit
	if end > n {
		end = n
	}
	return offset, end
}

func (s *Server) sitePages(args map[string]any) toolResult {
	g, err := s.loadGraph()
	if err != nil {
		return errResult(err.Error())
	}
	typ, _ := args["type"].(string)
	lang, _ := args["lang"].(string)
	var pages []sitegraph.Page
	for _, p := range g.Pages {
		if typ != "" && !strings.EqualFold(p.Type, typ) {
			continue
		}
		if lang != "" && !strings.EqualFold(p.Lang, lang) {
			continue
		}
		pages = append(pages, p)
	}
	from, to := window(args, 100, 1000, len(pages))
	type row struct {
		URL   string     `json:"url"`
		Type  string     `json:"type"`
		Title string     `json:"title"`
		Lang  string     `json:"lang,omitempty"`
		Date  *time.Time `json:"date,omitempty"`
	}
	rows := make([]row, 0, to-from)
	for _, p := range pages[from:to] {
		rows = append(rows, row{p.URL, p.Type, p.Title, p.Lang, p.Date})
	}
	return graphResult(g.Build, "pages", map[string]any{"total": len(pages), "offset": from, "items": rows})
}

func (s *Server) sitePage(args map[string]any) toolResult {
	g, err := s.loadGraph()
	if err != nil {
		return errResult(err.Error())
	}
	url, _ := args["url"].(string)
	url = strings.TrimSpace(url)
	for _, p := range g.Pages {
		if p.URL != url {
			continue
		}
		var out, in []sitegraph.Link
		for _, l := range g.Links {
			if l.From == url {
				out = append(out, l)
			}
			if l.To == url {
				in = append(in, l)
			}
		}
		return graphResult(g.Build, "page", map[string]any{"page": p, "links_out": out, "links_in": in})
	}
	return errResult(fmt.Sprintf("no page at %q in the site graph — site_pages lists what exists", url))
}

func (s *Server) siteLinks(args map[string]any) toolResult {
	g, err := s.loadGraph()
	if err != nil {
		return errResult(err.Error())
	}
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)
	kind, _ := args["kind"].(string)
	var links []sitegraph.Link
	for _, l := range g.Links {
		if from != "" && l.From != from {
			continue
		}
		if to != "" && l.To != to {
			continue
		}
		if kind != "" && !strings.EqualFold(l.Kind, kind) {
			continue
		}
		links = append(links, l)
	}
	lo, hi := window(args, 200, 5000, len(links))
	return graphResult(g.Build, "links", map[string]any{"total": len(links), "offset": lo, "items": links[lo:hi]})
}

func (s *Server) siteTaxonomies(map[string]any) toolResult {
	g, err := s.loadGraph()
	if err != nil {
		return errResult(err.Error())
	}
	tax := append([]sitegraph.Taxonomy(nil), g.Taxonomies...)
	sort.Slice(tax, func(i, j int) bool { return tax[i].Name < tax[j].Name })
	return graphResult(g.Build, "taxonomies", tax)
}

func (s *Server) siteRedirects(map[string]any) toolResult {
	g, err := s.loadGraph()
	if err != nil {
		return errResult(err.Error())
	}
	return graphResult(g.Build, "redirects", g.Redirects)
}
