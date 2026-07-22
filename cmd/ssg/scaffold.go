package main

// `ssg new worker <template>` extracts a batteries-included Cloudflare Pages
// Functions template into ./workers/<template> and prints the worker: config
// block to paste into .ssg.yaml (GO-066). Templates are embedded in the binary.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ssgroot "github.com/spagu/ssg"
)

// runNewWorker scaffolds the named worker template. Returns a process exit code.
func runNewWorker(args []string) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(os.Stderr, "usage: ssg new worker <template>\n\navailable templates:\n")
		for _, name := range availableWorkerTemplates() {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		return 2
	}
	name := args[0]
	// The name indexes an embedded template and becomes a path segment, so it
	// must be a plain identifier — never a path fragment that could escape ./workers.
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		fmt.Fprintf(os.Stderr, "❌ invalid template name %q\n", name)
		return 1
	}
	root := "workers/" + name
	if entries, err := ssgroot.EmbeddedWorkers.ReadDir(root); err != nil || len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "❌ unknown worker template %q. Available: %s\n", name, strings.Join(availableWorkerTemplates(), ", "))
		return 1
	}
	dest := filepath.Join("workers", name)
	// #nosec G703 -- name is validated above to a plain identifier under ./workers
	if _, err := os.Stat(dest); err == nil {
		fmt.Fprintf(os.Stderr, "❌ %s already exists — refusing to overwrite\n", dest)
		return 1
	}
	if err := extractWorkerTemplate(root, dest); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return 1
	}
	fmt.Printf("✅ Scaffolded worker %q into %s/\n\n", name, dest)
	fmt.Print(workerConfigSnippet(name))
	return 0
}

// availableWorkerTemplates lists the embedded worker template names.
func availableWorkerTemplates() []string {
	entries, err := ssgroot.EmbeddedWorkers.ReadDir("workers")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// extractWorkerTemplate copies an embedded worker tree onto disk under dest.
func extractWorkerTemplate(root, dest string) error {
	return fs.WalkDir(ssgroot.EmbeddedWorkers, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			// #nosec G301,G703 -- scaffolded dirs are world-traversable; dest is a validated ./workers path
			return os.MkdirAll(target, 0755)
		}
		data, err := ssgroot.EmbeddedWorkers.ReadFile(path)
		if err != nil {
			return err
		}
		// #nosec G306,G703 -- scaffolded files are world-readable; dest is a validated ./workers path
		return os.WriteFile(target, data, 0644)
	})
}

// workerConfigSnippet returns the worker: block to add to .ssg.yaml.
func workerConfigSnippet(name string) string {
	return fmt.Sprintf(`Add this to your .ssg.yaml:

worker:
  dir: workers/%s
  mode: functions
  routes_include:
    - /api/*

Then set the secrets listed in workers/%s/README.md via `+"`wrangler pages secret put`"+`.
`, name, name)
}
