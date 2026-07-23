package main

// `ssg new wrangler` generates a starter wrangler.toml for a project that uses
// workers, deriving the name, output dir and each worker's bindings from the
// config. It never overwrites an existing wrangler config (GO-077).

import (
	"fmt"
	"os"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// runNewWrangler writes wrangler.toml from the loaded config. Returns an exit code.
func runNewWrangler(args []string) int {
	cfg := loadConfig(args)
	dirs := workerDirsOf(cfg)
	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "❌ no workers configured — add a worker: or workers: block first (see docs/WORKERS.md)")
		return 1
	}
	path, created, err := generator.EnsureWranglerConfig(".", cfg.Domain, cfg.OutputDir, dirs, explicitWranglerConfig(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return 1
	}
	if created {
		fmt.Printf("✅ Wrote %s — review the binding stubs, then `wrangler pages dev %s`.\n", path, cfg.OutputDir)
	} else {
		fmt.Printf("ℹ️  %s already exists; leaving it untouched.\n", path)
	}
	return 0
}

// workerDirsOf returns the local directory of every resolved worker (a
// remote-source-only worker with no dir yet is skipped).
func workerDirsOf(cfg *config.Config) []string {
	var dirs []string
	for _, w := range cfg.ResolvedWorkers() {
		if w.Dir != "" {
			dirs = append(dirs, w.Dir)
		}
	}
	return dirs
}

// ensureWranglerForWorkers writes a starter wrangler.toml before the watch
// runner starts, when workers are configured and none exists. Non-fatal: a
// failure only means `wrangler pages dev` has no bindings, not that the build
// cannot proceed.
func ensureWranglerForWorkers(cfg *config.Config) {
	dirs := workerDirsOf(cfg)
	if len(dirs) == 0 {
		return
	}
	path, created, err := generator.EnsureWranglerConfig(".", cfg.Domain, cfg.OutputDir, dirs, explicitWranglerConfig(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  could not generate wrangler.toml: %v\n", err)
		return
	}
	if created && !cfg.Quiet {
		fmt.Printf("   🧩 Generated %s for your worker(s) — review the binding stubs.\n", path)
	}
}

// explicitWranglerConfig returns the first worker's wrangler_config, if any, so
// a user who already points at their own config is not handed a generated one.
func explicitWranglerConfig(cfg *config.Config) string {
	for _, w := range cfg.ResolvedWorkers() {
		if w.WranglerConfig != "" {
			return w.WranglerConfig
		}
	}
	return ""
}
