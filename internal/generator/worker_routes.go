package generator

// Cloudflare Pages rejects a _routes.json whose rules overlap (#252):
//
//	✘ [ERROR] Invalid _routes.json file found at: _routes.json
//	  Overlapping rules found. Please make sure that rules ending with a splat
//	  (eg. "/api/*") don't overlap any other rules (eg. "/api/foo").
//
// The build unioned every worker's routes and collapsed exact duplicates
// (GO-081) but not overlapping ones, so the arrangement docs/WORKERS.md
// recommends — a middleware worker beside route-owning workers — produced a
// site that could not be deployed at all. The failure arrives at
// `wrangler pages deploy`, after the upload, long after a green build.
//
// It is not avoidable by saying less, either: a worker with no routes_include
// defaults to /api/* per worker (GO-081, so it is not left unrouted), which is
// exactly the value that overlaps.
//
// The generator writes this file itself, so it can simply not write an invalid
// one — the same argument as #247, one step earlier.

import (
	"fmt"
	"strings"
)

// collapseRouteOverlaps drops every rule already covered by a splat in the same
// list, preserving first-seen order.
//
// `/api/contact` and `/api/consent/*` both fall under `/api/*`, which leaves
// `["/api/*"]` — what the site meant, what Cloudflare accepts, and three of the
// hundred allowed rules spent instead of one. Include and exclude are collapsed
// separately, because Cloudflare applies the rule to each list on its own.
func collapseRouteOverlaps(rules []string) []string {
	prefixes := make([]string, 0, len(rules))
	for _, r := range rules {
		if p, ok := splatPrefix(r); ok {
			prefixes = append(prefixes, p)
		}
	}
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		if !routeCovered(r, prefixes) {
			out = append(out, r)
		}
	}
	return out
}

// routeCovered reports whether a splat in the list already matches everything
// this rule does. A splat never covers itself: `/api/*` is the survivor of its
// own family, not a casualty of it.
func routeCovered(rule string, prefixes []string) bool {
	self, isSplat := splatPrefix(rule)
	for _, p := range prefixes {
		if isSplat && self == p {
			continue
		}
		if strings.HasPrefix(rule, p) {
			return true
		}
	}
	return false
}

// splatPrefix returns the path a splat rule covers: "/api/*" covers "/api/".
func splatPrefix(rule string) (string, bool) {
	if strings.HasSuffix(rule, "*") {
		return strings.TrimSuffix(rule, "*"), true
	}
	return "", false
}

// reportCollapsedRoutes says which declared rules were absorbed, because the
// published file no longer matches what the config asked for and silence would
// leave that to be discovered by reading the output.
func (g *Generator) reportCollapsedRoutes(kind string, before, after []string) {
	if g.config.Quiet || len(after) >= len(before) {
		return
	}
	kept := make(map[string]bool, len(after))
	for _, r := range after {
		kept[r] = true
	}
	var absorbed []string
	for _, r := range before {
		if !kept[r] {
			absorbed = append(absorbed, r)
		}
	}
	fmt.Printf("   🧭 _routes.json: %s %s already covered by a splat rule, folded into %s\n",
		strings.Join(absorbed, ", "), pluralIs(len(absorbed)), strings.Join(after, ", "))
	fmt.Println("      Cloudflare rejects overlapping rules, so the file names the covering rule only.")
}

// pluralIs keeps the sentence above grammatical for one rule or several.
func pluralIs(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
