package generator

// The order assets are fingerprinted in (#309). fingerprintOne rewrites a file
// with the hashed names known so far, so a file must be hashed after every
// asset it names — or its reference keeps the unhashed name, which the pass
// then deletes. The old order was "JS in path order, then CSS by @import
// count": playground.js importing presets.js was hashed first because "l" sorts
// before "r", and shipped `import … from "./presets.js"`, a 404.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// assetDeps returns, for each asset, the other assets its content names —
// by basename, bounded the way the rewriter matches it, so what counts as a
// dependency here is exactly what fingerprintOne would rewrite.
func assetDeps(paths []string) (map[string][]string, error) {
	byBase := make(map[string][]string, len(paths))
	for _, p := range paths {
		byBase[filepath.Base(p)] = append(byBase[filepath.Base(p)], p)
	}
	bases := make([]string, 0, len(byBase))
	for b := range byBase {
		bases = append(bases, b)
	}
	sort.Strings(bases)
	patterns := make(map[string]*regexp.Regexp, len(bases))
	for _, b := range bases {
		patterns[b] = regexp.MustCompile(`[/"'(=]` + regexp.QuoteMeta(b) + `[)"'?#\s]`)
	}
	deps := make(map[string][]string, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p) // #nosec G304 -- CLI reads its own output
		if err != nil {
			return nil, err
		}
		for _, b := range bases {
			if !patterns[b].Match(data) {
				continue
			}
			for _, dep := range byBase[b] {
				if dep != p {
					deps[p] = append(deps[p], dep)
				}
			}
		}
	}
	return deps, nil
}

// orderAssetsByReferences sorts assets so every file comes after the assets it
// references (Kahn's algorithm). Ties keep the input order, so the result is
// deterministic. Files in a reference cycle cannot all be hashed after each
// other; they are appended in input order and returned as cycle, for the build
// to name — one of them will keep an unhashed reference.
func orderAssetsByReferences(paths []string) (ordered, cycle []string, err error) {
	deps, err := assetDeps(paths)
	if err != nil {
		return nil, nil, err
	}
	pending := make(map[string]int, len(paths))
	dependents := make(map[string][]string, len(paths))
	for _, p := range paths {
		pending[p] = len(deps[p])
		for _, d := range deps[p] {
			dependents[d] = append(dependents[d], p)
		}
	}
	done := make(map[string]bool, len(paths))
	for len(ordered) < len(paths) {
		progressed := false
		for _, p := range paths {
			if done[p] || pending[p] > 0 {
				continue
			}
			done[p], progressed = true, true
			ordered = append(ordered, p)
			for _, dep := range dependents[p] {
				pending[dep]--
			}
		}
		if !progressed {
			break
		}
	}
	for _, p := range paths {
		if !done[p] {
			cycle = append(cycle, p)
		}
	}
	return append(ordered, cycle...), cycle, nil
}

// fingerprintOrder is the order fingerprintAssets hashes in, with a warning
// when a reference cycle makes a correct order impossible.
func (g *Generator) fingerprintOrder(paths []string) ([]string, error) {
	ordered, cycle, err := orderAssetsByReferences(paths)
	if err != nil {
		return nil, err
	}
	if len(cycle) > 0 && !g.config.Quiet {
		names := make([]string, len(cycle))
		for i, p := range cycle {
			rel, _ := filepath.Rel(g.config.OutputDir, p)
			names[i] = filepath.ToSlash(rel)
		}
		fmt.Printf("   ⚠️  fingerprint: %v reference each other in a cycle — one keeps an unhashed name for the other\n", names)
	}
	return ordered, nil
}
