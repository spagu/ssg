package main

// `ssg graph` — what a change pulls with it (GO-094).
//
// The incremental build is only half of the feature. The other half is being
// able to see why it did what it did, because the two questions people actually
// have are "why did editing one post rebuild everything?" and "what will this
// change touch?", and neither is answerable from a build log.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/generator"
)

// isGraphSubcommand keeps the verb+noun dispatch rule loose here: `ssg graph`
// takes a path or nothing, so anything that is not a flag is a path.
func isGraphSubcommand(arg string) bool {
	return !strings.HasPrefix(arg, "-")
}

// runGraph implements `ssg graph [path] [--dot] [--json]`.
func runGraph(args []string) int {
	format := "text"
	var target string
	for _, a := range args {
		switch {
		case a == "--dot":
			format = "dot"
		case a == "--json":
			format = "json"
		case strings.HasPrefix(a, "-"):
			errf("❌ unknown option %s (try --dot or --json)\n", a)
			return 2
		default:
			target = a
		}
	}
	dir := graphDir()
	graph := depgraph.Load(dir)
	if graph == nil {
		errf("❌ no dependency graph in %s — run a build first.\n", dir)
		return 1
	}
	switch format {
	case "dot":
		fmt.Print(graph.DOT())
		return 0
	case "json":
		return printGraphJSON(graph, target)
	}
	return printGraphText(graph, target)
}

// graphDir is where a build persists its graph.
func graphDir() string {
	return ".ssg-cache/" + generator.GraphDirName
}

// printGraphText is the answer a person asked for.
func printGraphText(graph *depgraph.Graph, target string) int {
	stats := graph.Stats()
	if target == "" {
		fmt.Printf("Dependency graph: %d inputs, %d outputs, %d edges\n", stats.Inputs, stats.Outputs, stats.Edges)
		if reasons := graph.Reasons(); len(reasons) > 0 {
			fmt.Println("\nThis site's builds cannot be narrowed:")
			for _, r := range reasons {
				fmt.Printf("  · %s\n", r)
			}
			fmt.Println("\nEvery change rebuilds the whole site. That is correct, not a bug —")
			fmt.Println("but it is also why --incremental saves nothing here.")
			return 0
		}
		fmt.Println("\nPass a file to see what changing it rebuilds:")
		fmt.Println("  ssg graph content/site/posts/hello.md")
		return 0
	}

	plan := graph.Explain(target)
	if plan.Full {
		fmt.Printf("%s → a full rebuild\n\n  %s\n", target, plan.Reason)
		return 0
	}
	if len(plan.Outputs) == 0 {
		fmt.Printf("%s → nothing\n\n  The last build recorded no output that depends on it.\n", target)
		return 0
	}
	fmt.Printf("%s → %d output(s)\n\n", target, len(plan.Outputs))
	shown := plan.Outputs
	const limit = 40
	truncated := false
	if len(shown) > limit {
		shown, truncated = shown[:limit], true
	}
	for _, o := range shown {
		fmt.Printf("  %s\n", o)
	}
	if truncated {
		fmt.Printf("  … and %d more (--json for the whole list)\n", len(plan.Outputs)-limit)
	}
	return 0
}

// printGraphJSON is the same answer for a script.
func printGraphJSON(graph *depgraph.Graph, target string) int {
	if target == "" {
		stats := graph.Stats()
		return writeGraphJSON(map[string]any{
			"inputs": stats.Inputs, "outputs": stats.Outputs, "edges": stats.Edges,
			"global": graph.Reasons(),
		})
	}
	plan := graph.Explain(target)
	sort.Strings(plan.Outputs)
	return writeGraphJSON(map[string]any{
		"path": target, "full": plan.Full, "reason": plan.Reason, "outputs": plan.Outputs,
	})
}

// writeGraphJSON prints one JSON document.
func writeGraphJSON(v map[string]any) int {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	_, _ = os.Stdout.Write(append(data, '\n'))
	return 0
}
