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
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/fetch"
)

// WorkerConfig wires a Worker project into the build.
type WorkerConfig struct {
	// Name identifies the worker in a plural workers: list (logging, output
	// collision messages).
	Name string
	// Dir is the source directory: a Pages Functions tree (mode "functions")
	// or a directory containing a prebuilt _worker.js (mode "worker"). When
	// Source is set, Dir is where the fetched worker lands.
	Dir string
	// Source optionally fetches the worker from a repo/zip URL into Dir before
	// building; Auth covers private sources (GO-076).
	Source string
	Auth   fetch.Auth
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

// generateWorkerFiles builds every configured worker into the output and writes
// one _routes.json. A no-op when none is configured. The workers are
// independent definitions; because Cloudflare Pages serves a single functions/
// tree and one _routes.json per project, their functions are copied into the
// shared tree (a collision between two workers is a hard error, never a silent
// overwrite) and their routes are combined (GO-076).
func (g *Generator) generateWorkerFiles() error {
	workers := g.config.ResolvedWorkers()
	if len(workers) == 0 {
		return nil
	}
	seen := map[string]string{}       // output-relative functions path -> worker
	seenPublic := map[string]string{} // output-relative public asset -> worker
	var include, exclude []string
	wroteWorkerJS := ""
	for i, w := range workers {
		label := workerLabel(w, i)
		dir, err := g.resolveWorkerDir(w, label)
		if err != nil {
			return err
		}
		mode := w.Mode
		if mode == "" {
			mode = workerModeFunctions
		}
		switch mode {
		case workerModeFunctions:
			if err := g.copyFunctionsTree(dir, label, seen); err != nil {
				return err
			}
		case workerModeWorker:
			if wroteWorkerJS != "" {
				return fmt.Errorf("worker %s and %s both use mode %q: a project can have only one _worker.js", label, wroteWorkerJS, workerModeWorker)
			}
			if err := g.copyPrebuiltWorker(dir); err != nil {
				return err
			}
			wroteWorkerJS = label
		default:
			return fmt.Errorf("worker %s: unknown mode %q (use %q or %q)", label, w.Mode, workerModeFunctions, workerModeWorker)
		}
		// A worker's public/ holds client assets (a consent banner's js/css)
		// served as static files; copy it to the output root (GO-076).
		if err := g.copyWorkerPublic(dir, label, seenPublic); err != nil {
			return err
		}
		// A worker with no explicit routes_include still needs its functions
		// routed, so default it to /api/* per worker — not only when the combined
		// list is empty. Otherwise a worker that omits routes_include next to one
		// that sets its own (e.g. /consent/*) would be left entirely unrouted, its
		// Functions never invoked (GO-081).
		inc := w.RoutesInclude
		if len(inc) == 0 {
			inc = []string{"/api/*"}
		}
		include = append(include, inc...)
		exclude = append(exclude, w.RoutesExclude...)
	}
	// Two workers can legitimately name the same route; collapse duplicates so
	// they don't count twice against the Cloudflare rule cap (GO-081).
	include = dedupeStrings(include)
	exclude = dedupeStrings(exclude)
	routes, err := renderRoutesJSON(include, exclude)
	if err != nil {
		return err
	}
	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(filepath.Join(g.config.OutputDir, "_routes.json"), routes, 0644); err != nil {
		return fmt.Errorf("worker: writing _routes.json: %w", err)
	}
	return nil
}

// dedupeStrings removes duplicate entries while preserving first-seen order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// workerLabel names a worker for messages: its Name, else worker[i].
func workerLabel(w WorkerConfig, i int) string {
	if w.Name != "" {
		return fmt.Sprintf("%q", w.Name)
	}
	return fmt.Sprintf("worker[%d]", i)
}

// resolveWorkerDir returns the local directory to build a worker from, fetching
// a remote Source into Dir first when needed. A cached (already-present) Dir is
// reused rather than re-fetched, so a build is not gated on the network.
func (g *Generator) resolveWorkerDir(w WorkerConfig, label string) (string, error) {
	if w.Source == "" {
		if w.Dir == "" {
			return "", fmt.Errorf("worker %s: neither dir nor source is set", label)
		}
		return w.Dir, nil
	}
	dir := w.Dir
	if dir == "" {
		// The vendor directory is derived from the name; without one, two
		// unnamed remote sources would both resolve to workers/worker and the
		// second would silently reuse the first's files (GO-081).
		if w.Name == "" {
			return "", fmt.Errorf("worker %s: a remote source needs a name (or an explicit dir) to vendor into", label)
		}
		dir = filepath.Join("workers", sanitizeWorkerName(w.Name))
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return dir, nil // already fetched/vendored — reuse
	}
	auth, err := fetch.ExpandAuth(w.Auth)
	if err != nil {
		return "", fmt.Errorf("worker %s: %w", label, err)
	}
	g.log(fmt.Sprintf("   📥 worker %s: fetching %s", label, w.Source))
	if err := fetch.Archive(w.Source, auth, dir); err != nil {
		return "", fmt.Errorf("worker %s: %w", label, err)
	}
	return dir, nil
}

// sanitizeWorkerName reduces a worker name to a safe single path segment; a
// missing/odd name falls back to "worker".
func sanitizeWorkerName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	if name == "" || strings.Trim(name, "-") == "" {
		return "worker"
	}
	return name
}

// copyFunctionsTree copies a Pages Functions source tree into output/functions.
// Both the functions dir itself and its parent project dir are accepted. Files
// are recorded in seen so a second worker writing the same output path is caught
// as a collision rather than silently overwriting the first (GO-076).
func (g *Generator) copyFunctionsTree(dir, label string, seen map[string]string) error {
	src := dir
	if fi, err := os.Stat(filepath.Join(src, "functions")); err == nil && fi.IsDir() {
		src = filepath.Join(src, "functions")
	}
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return fmt.Errorf("worker %s: functions directory %q not found", label, dir)
	}
	if err := g.claimFunctionPaths(src, label, seen); err != nil {
		return err
	}
	if err := g.copyDir(src, filepath.Join(g.config.OutputDir, "functions")); err != nil {
		return fmt.Errorf("worker %s: copying functions: %w", label, err)
	}
	g.warnBareImports(src)
	return nil
}

// copyWorkerPublic copies a worker's public/ tree (its client-side assets, e.g.
// a consent banner's js/css) into the output root, so they are served as static
// files. A no-op when the worker has no public/. Two workers shipping the same
// asset path is a hard error, never a silent overwrite.
func (g *Generator) copyWorkerPublic(dir, label string, seen map[string]string) error {
	src := filepath.Join(dir, "public")
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return nil
	}
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		key := filepath.ToSlash(rel)
		if other, clash := seen[key]; clash {
			return fmt.Errorf("worker %s and %s both provide public asset %s", label, other, key)
		}
		seen[key] = label
		return nil
	})
	if err != nil {
		return err
	}
	if err := g.copyDir(src, g.config.OutputDir); err != nil {
		return fmt.Errorf("worker %s: copying public assets: %w", label, err)
	}
	return nil
}

// claimFunctionPaths records each function file this worker will write, erroring
// when another worker already claimed the same output path.
func (g *Generator) claimFunctionPaths(src, label string, seen map[string]string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		key := filepath.ToSlash(filepath.Join("functions", rel))
		if other, clash := seen[key]; clash {
			return fmt.Errorf("worker %s and %s both provide %s — give them distinct routes", label, other, key)
		}
		seen[key] = label
		return nil
	})
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
		for _, spec := range bareModuleSpecs(string(data)) {
			fmt.Printf("   ⚠️  worker: %s imports npm package %q — Direct Upload cannot bundle it; deploy via wrangler\n", filepath.Base(path), spec)
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

// importFromRE captures the specifier of any `… from "spec"` clause (import-from
// or export-from), even when the `import { … }` spans several lines — matching on
// the `from` clause rather than the opening line is what avoids mis-reading a
// bare `import {` as a package. sideEffectImportRE captures `import "spec"`.
var (
	importFromRE       = regexp.MustCompile(`\bfrom\s*["']([^"']+)["']`)
	sideEffectImportRE = regexp.MustCompile(`(?m)^\s*import\s*["']([^"']+)["']`)
)

// bareModuleSpecs returns the distinct bare (npm) module specifiers a source file
// imports. Relative ("./x", "../x"), absolute ("/x"), URL and runtime-builtin
// (cloudflare:/node:) specifiers are not bare and are skipped. Best-effort string
// scan over the whole file, so a multi-line `import { … } from "pkg"` is read
// from its `from` clause, not its `import {` opening line.
func bareModuleSpecs(content string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(spec string) {
		if isBareModuleSpec(spec) && !seen[spec] {
			seen[spec] = true
			out = append(out, spec)
		}
	}
	for _, m := range importFromRE.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	for _, m := range sideEffectImportRE.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	return out
}

// isBareModuleSpec reports whether spec names an external package (not a
// relative/absolute path, URL or runtime builtin).
func isBareModuleSpec(spec string) bool {
	if spec == "" {
		return false
	}
	for _, p := range []string{".", "/", "cloudflare:", "node:", "http:", "https:"} {
		if strings.HasPrefix(spec, p) {
			return false
		}
	}
	return true
}
