package main

// `ssg cache stats|clean|gc` (GO-091): one CLI over every SSG cache namespace —
// images, external sources and AI — resolved from the project config the same
// way a build would resolve them.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/cache"
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/externalsource"
)

// aiLegacyCacheDir mirrors internal/ai's pre-GO-091 default root; stats/clean
// surface it so leftover entries are visible and removable.
const aiLegacyCacheDir = ".ai-cache"

// cacheNamespace names one cache dir the CLI operates on.
type cacheNamespace struct {
	name string
	dir  string
}

// cacheNamespaces resolves the cache directories exactly as a build would:
// explicit config values win, defaults otherwise. The legacy AI root is
// included only while it still exists on disk.
func cacheNamespaces(cfg *config.Config) []cacheNamespace {
	extDir := externalsource.DefaultCacheDir
	aiDir := cache.Dir("", "ai")
	if cfg != nil {
		if d := cfg.ExternalSources.CacheDir; d != "" {
			extDir = d
		}
		if d := cfg.AI.CacheDir; d != "" {
			aiDir = d
		}
	}
	ns := []cacheNamespace{
		{"images", cache.Dir("", "images")},
		{"external-sources", extDir},
		{"ai", aiDir},
	}
	if _, err := os.Stat(aiLegacyCacheDir); err == nil {
		ns = append(ns, cacheNamespace{"ai (legacy)", aiLegacyCacheDir})
	}
	return ns
}

// loadCacheConfig loads the project config when one is auto-detectable; the
// cache CLI works without one (defaults) — unlike a build, it needs no source.
func loadCacheConfig() *config.Config {
	path := config.FindConfigFile()
	if path == "" {
		return nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  config %s: %v (using default cache locations)\n", path, err)
		return nil
	}
	return cfg
}

// runCache dispatches `ssg cache <stats|clean|gc>`.
func runCache(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ssg cache <stats|clean|gc> [--namespace=NAME] [--dry]")
		return 2
	}
	namespaces := cacheNamespaces(loadCacheConfig())
	sub, rest := args[0], args[1:]
	switch sub {
	case "stats":
		return runCacheStats(namespaces)
	case "clean":
		return runCacheClean(namespaces, rest)
	case "gc":
		return runCacheGC(namespaces, rest)
	default:
		fmt.Fprintf(os.Stderr, "❌ unknown cache subcommand %q (use stats, clean or gc)\n", sub)
		return 2
	}
}

func runCacheStats(namespaces []cacheNamespace) int {
	var totalEntries int
	var totalBytes int64
	fmt.Printf("%-18s %10s %12s   %s\n", "NAMESPACE", "ENTRIES", "SIZE", "DIR")
	for _, ns := range namespaces {
		st, err := cache.DirStats(ns.name, ns.dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", ns.name, err)
			continue
		}
		fmt.Printf("%-18s %10d %12s   %s\n", ns.name, st.Entries, cache.HumanBytes(st.Bytes), ns.dir)
		totalEntries += st.Entries
		totalBytes += st.Bytes
	}
	fmt.Printf("%-18s %10d %12s\n", "total", totalEntries, cache.HumanBytes(totalBytes))
	return 0
}

// filterNamespaces applies --namespace=NAME; empty selector keeps all.
func filterNamespaces(namespaces []cacheNamespace, selector string) []cacheNamespace {
	if selector == "" {
		return namespaces
	}
	var out []cacheNamespace
	for _, ns := range namespaces {
		if ns.name == selector || strings.HasPrefix(ns.name, selector+" ") { // "ai" also selects "ai (legacy)"
			out = append(out, ns)
		}
	}
	return out
}

// parseCacheFlags extracts --namespace and --dry from the remaining args.
func parseCacheFlags(args []string) (namespace string, dry bool, ok bool) {
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--namespace="):
			namespace = strings.TrimPrefix(a, "--namespace=")
		case a == "--dry":
			dry = true
		default:
			fmt.Fprintf(os.Stderr, "❌ unknown flag %q\n", a)
			return "", false, false
		}
	}
	return namespace, dry, true
}

func runCacheClean(namespaces []cacheNamespace, args []string) int {
	selector, _, ok := parseCacheFlags(args)
	if !ok {
		return 2
	}
	selected := filterNamespaces(namespaces, selector)
	if len(selected) == 0 {
		fmt.Fprintf(os.Stderr, "❌ no cache namespace matches %q\n", selector)
		return 1
	}
	for _, ns := range selected {
		st, _ := cache.DirStats(ns.name, ns.dir)
		if err := cache.Clean(ns.dir); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %s: %v\n", ns.name, err)
			return 1
		}
		fmt.Printf("🧹 %s: removed %d entries (%s)\n", ns.name, st.Entries, cache.HumanBytes(st.Bytes))
	}
	return 0
}

func runCacheGC(namespaces []cacheNamespace, args []string) int {
	selector, dry, ok := parseCacheFlags(args)
	if !ok {
		return 2
	}
	verb := "reclaimed"
	if dry {
		verb = "would reclaim"
	}
	for _, ns := range filterNamespaces(namespaces, selector) {
		switch ns.name {
		case "external-sources":
			files, bytes, err := externalsource.GCExpired(ns.dir, time.Now(), dry)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ %s: %v\n", ns.name, err)
				return 1
			}
			fmt.Printf("♻️  %s: %s %d expired entries (%s)\n", ns.name, verb, files, cache.HumanBytes(bytes))
		case "images":
			// Image GC needs the build's reference manifest — only a build knows
			// which variants the site still uses.
			fmt.Printf("ℹ️  images: run a build with --images-gc (or images_gc: true); GC needs the build manifest\n")
		default: // ai + legacy: content-addressed, no expiry metadata
			fmt.Printf("ℹ️  %s: entries carry no expiry; use `ssg cache clean --namespace=ai` to drop them\n", ns.name)
		}
	}
	return 0
}
