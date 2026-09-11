package mcp

// What a page was built from, and what an edit to a file will rebuild
// (GO-095 phase 3, on top of GO-094).
//
// This is the one part of the site model that is deliberately NOT in the
// public site-graph.json. Pages, links, taxonomies and redirects all follow
// from the published HTML, so publishing them costs nothing. Which template
// rendered a page, and which data file it read, is the shape of the PROJECT,
// not of the site — it belongs in .ssg-cache, where only an agent working on
// the project can see it.
//
// The question an agent actually has before an edit is "what does this
// change?", and the dependency graph is the only thing that knows. Answering
// it from a directory walk would mean re-deriving what the build recorded, and
// getting it subtly wrong.

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/depgraph"
)

// jsonResult renders an answer as indented JSON.
func jsonResult(v map[string]any) toolResult {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult(err.Error())
	}
	return textResult(string(out))
}

// GraphDirName mirrors the generator's, so the server reads the same cache the
// build writes without importing the generator.
const GraphDirName = "graph"

// dependencyTools is the section, present with the rest of site_*.
func (s *Server) dependencyTools() []tool {
	return []tool{
		{
			name: "site_dependencies",
			description: "SITE · What a page was built from, or what editing a file will rebuild. " +
				"Pass `url` for a page's inputs (its source file, the templates and data the " +
				"build read); pass `path` for the outputs a change to that file reaches, or the " +
				"reason it rebuilds everything. Neither answers whether this site's builds can " +
				"be narrowed at all. Reads the last build's dependency graph.",
			schema: objectSchema(map[string]any{
				"url":  stringProp("A page's site-relative URL, e.g. \"/pricing/\""),
				"path": stringProp("A file in the project, e.g. \"content/site/posts/hello.md\""),
			}),
			handler: s.siteDependencies,
		},
	}
}

// depGraphDir is where the build persisted its dependency graph.
func (s *Server) depGraphDir() string {
	root := s.opts.CacheDir
	if root == "" {
		root = filepath.Join(s.opts.Root, ".ssg-cache")
	}
	return filepath.Join(root, GraphDirName)
}

// siteDependencies answers in one of three shapes, depending on what was asked.
func (s *Server) siteDependencies(args map[string]any) toolResult {
	graph := depgraph.Load(s.depGraphDir())
	if graph == nil {
		return errResult("no dependency graph from a previous build in " + s.depGraphDir() +
			" — run a build first; the graph is recorded by every build")
	}
	url, _ := strArg(args, "url")
	path, _ := strArg(args, "path")
	switch {
	case url != "" && path != "":
		return errResult("ask about a page (`url`) or about a file (`path`), not both — they are different questions")
	case path != "":
		return dependencyPlan(graph, path)
	case url != "":
		return dependencyInputs(graph, url)
	}
	return dependencyOverview(graph)
}

// dependencyOverview says how big the graph is and whether this site's builds
// can be narrowed — the thing to know before trusting either other answer.
func dependencyOverview(graph *depgraph.Graph) toolResult {
	stats := graph.Stats()
	return jsonResult(map[string]any{
		"inputs":  stats.Inputs,
		"outputs": stats.Outputs,
		"edges":   stats.Edges,
		"narrowable": map[string]any{
			"yes":     !graph.IsGlobal(),
			"reasons": graph.Reasons(),
		},
		"ask": "pass `url` for a page's inputs, or `path` for what a file change rebuilds",
	})
}

// dependencyPlan answers what editing one file rebuilds.
func dependencyPlan(graph *depgraph.Graph, path string) toolResult {
	plan := graph.Explain(filepath.ToSlash(path))
	out := map[string]any{
		"path":   path,
		"full":   plan.Full,
		"reason": plan.Reason,
	}
	if !plan.Full {
		sort.Strings(plan.Outputs)
		out["outputs"] = plan.Outputs
		out["count"] = len(plan.Outputs)
	}
	return jsonResult(out)
}

// dependencyInputs answers what one page was built from.
//
// A page is asked for by URL and the graph holds output paths, so the match is
// on the tail: the caller knows "/pricing/", the build wrote
// "output/pricing/index.html", and the output directory is not the caller's
// business.
func dependencyInputs(graph *depgraph.Graph, url string) toolResult {
	tail := strings.TrimPrefix(url, "/")
	if !strings.HasSuffix(tail, ".html") {
		tail = strings.TrimSuffix(tail, "/") + "/index.html"
	}
	tail = strings.TrimPrefix(tail, "/")
	byKind := map[string][]string{}
	var matched string
	for input, outputs := range graph.Edges {
		for _, out := range outputs {
			if !strings.HasSuffix(out, tail) {
				continue
			}
			matched = out
			kind := string(graph.Nodes[input].Kind)
			if kind == "" {
				kind = "unknown"
			}
			byKind[kind] = append(byKind[kind], input)
			break
		}
	}
	if matched == "" {
		return errResult("no page at " + url + " in the last build's dependency graph — " +
			"call site_pages for what the build did publish, or site_dependencies with no argument " +
			"to see whether this site's builds record edges at all")
	}
	for _, list := range byKind {
		sort.Strings(list)
	}
	return jsonResult(map[string]any{
		"url":    url,
		"output": matched,
		"inputs": byKind,
		"global": graph.Reasons(),
		"caveat": "these are the edges the last build recorded; a site whose builds cannot be narrowed has fewer of them than it has real dependencies",
	})
}
