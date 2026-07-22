package generator

// Cloudflare Pages Functions / Worker integration (GO-065): the `worker:`
// config section copies a Functions directory (or a prebuilt _worker.js) into
// the build output and writes _routes.json so static assets bypass the Worker.
// SSG deliberately does not bundle JS/TS — Pages builds Functions from source,
// and `mode: worker` expects an already-bundled file.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkerConfig wires a Worker project into the build.
type WorkerConfig struct {
	// Dir is the source directory: a Pages Functions tree (mode "functions")
	// or a directory containing a prebuilt _worker.js (mode "worker").
	Dir string
	// Mode selects the layout: "functions" (default) or "worker".
	Mode string
	// RoutesInclude/RoutesExclude become _routes.json. Defaults: include
	// ["/api/*"], exclude nothing — everything else stays static.
	RoutesInclude []string
	RoutesExclude []string
	// WranglerConfig points dev/deploy at a wrangler config file outside the
	// project root; reused by the --wrangler watch runner.
	WranglerConfig string
}

// workerModeFunctions / workerModeWorker are the two supported layouts.
const (
	workerModeFunctions = "functions"
	workerModeWorker    = "worker"
	// cfMaxRoutesRules is the combined include+exclude cap of _routes.json.
	cfMaxRoutesRules = 100
)

// routesJSON is the _routes.json document Cloudflare Pages expects.
type routesJSON struct {
	Version int      `json:"version"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// renderRoutesJSON renders _routes.json with defaults applied and warns via
// error when the combined rule count exceeds the Cloudflare cap.
func renderRoutesJSON(include, exclude []string) ([]byte, error) {
	if len(include) == 0 {
		include = []string{"/api/*"}
	}
	if exclude == nil {
		exclude = []string{}
	}
	if len(include)+len(exclude) > cfMaxRoutesRules {
		return nil, fmt.Errorf("_routes.json has %d rules, exceeding the Cloudflare Pages limit of %d", len(include)+len(exclude), cfMaxRoutesRules)
	}
	doc := routesJSON{Version: 1, Include: include, Exclude: exclude}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling _routes.json: %w", err)
	}
	return append(out, '\n'), nil
}

// generateWorkerFiles copies the configured Worker sources into the output and
// writes _routes.json. A no-op when no worker directory is configured.
func (g *Generator) generateWorkerFiles() error {
	w := g.config.Worker
	if w.Dir == "" {
		return nil
	}
	mode := w.Mode
	if mode == "" {
		mode = workerModeFunctions
	}
	var err error
	switch mode {
	case workerModeFunctions:
		err = g.copyFunctionsTree(w.Dir)
	case workerModeWorker:
		err = g.copyPrebuiltWorker(w.Dir)
	default:
		err = fmt.Errorf("worker: unknown mode %q (use %q or %q)", w.Mode, workerModeFunctions, workerModeWorker)
	}
	if err != nil {
		return err
	}
	routes, err := renderRoutesJSON(w.RoutesInclude, w.RoutesExclude)
	if err != nil {
		return err
	}
	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(filepath.Join(g.config.OutputDir, "_routes.json"), routes, 0644); err != nil {
		return fmt.Errorf("worker: writing _routes.json: %w", err)
	}
	return nil
}

// copyFunctionsTree copies a Pages Functions source tree into output/functions.
// Both the functions dir itself and its parent project dir are accepted.
func (g *Generator) copyFunctionsTree(dir string) error {
	src := dir
	if fi, err := os.Stat(filepath.Join(src, "functions")); err == nil && fi.IsDir() {
		src = filepath.Join(src, "functions")
	}
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return fmt.Errorf("worker: functions directory %q not found", dir)
	}
	if err := g.copyDir(src, filepath.Join(g.config.OutputDir, "functions")); err != nil {
		return fmt.Errorf("worker: copying functions: %w", err)
	}
	g.warnBareImports(src)
	return nil
}

// copyPrebuiltWorker copies an already-bundled _worker.js to the output root.
func (g *Generator) copyPrebuiltWorker(dir string) error {
	src := filepath.Join(dir, "_worker.js")
	data, err := os.ReadFile(src) // #nosec G304,G703 -- path from trusted local config, not attacker-controlled
	if err != nil {
		return fmt.Errorf("worker: mode %q needs a prebuilt %s: %w", workerModeWorker, src, err)
	}
	// #nosec G306,G703 -- Web content files need to be world-readable; path from trusted local config
	if err := os.WriteFile(filepath.Join(g.config.OutputDir, "_worker.js"), data, 0644); err != nil {
		return fmt.Errorf("worker: writing _worker.js: %w", err)
	}
	return nil
}

// warnBareImports scans copied function sources for bare module specifiers
// (npm packages): those need a wrangler build, not Direct Upload. Best-effort
// string scan; never fails the build.
func (g *Generator) warnBareImports(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !isWorkerSourceFile(path) {
			return nil
		}
		data, rerr := os.ReadFile(path) // #nosec G304,G122,G703 -- scanning the CLI's own configured worker sources; path from local Walk
		if rerr != nil {
			return nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			if spec, ok := bareImportSpec(line); ok {
				fmt.Printf("   ⚠️  worker: %s imports npm package %q — Direct Upload cannot bundle it; deploy via wrangler\n", filepath.Base(path), spec)
			}
		}
		return nil
	})
}

// isWorkerSourceFile reports whether a path is a JS/TS source worth scanning.
func isWorkerSourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".js", ".mjs", ".tsx":
		return true
	}
	return false
}

// bareImportSpec extracts a bare (npm) module specifier from an import line;
// relative, absolute and runtime-builtin (cloudflare:/node:) imports pass.
func bareImportSpec(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import{") {
		return "", false
	}
	from := trimmed
	if i := strings.Index(trimmed, " from "); i >= 0 {
		from = trimmed[i+len(" from "):]
	}
	spec := strings.Trim(from, "\"'; \t")
	if spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") ||
		strings.HasPrefix(spec, "cloudflare:") || strings.HasPrefix(spec, "node:") {
		return "", false
	}
	return spec, true
}
