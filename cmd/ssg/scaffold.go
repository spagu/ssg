package main

// `ssg new worker <template>` extracts a batteries-included Cloudflare Pages
// Functions template into ./workers/<template> and prints the worker: config
// block to paste into .ssg.yaml (GO-066). Templates are embedded in the binary.
//
// A template that ships its own wrangler.toml (newsletter-ui, GO-123) is a
// standalone Worker instead: it is deployed with `wrangler deploy` from its own
// directory and never joins the site's Pages build, so it gets deploy
// instructions rather than a worker: block.

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	ssgroot "github.com/spagu/ssg"
	"github.com/spagu/ssg/internal/modules"
)

// runNewWorker scaffolds the named worker template. Returns a process exit code.
func runNewWorker(args []string) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		errf("usage: ssg new worker <template>\n\navailable templates:\n")
		for _, name := range availableWorkerTemplates() {
			errf("  %s\n", name)
		}
		return 2
	}
	name := args[0]
	// The name indexes an embedded template and becomes a path segment, so it
	// must be a plain identifier — never a path fragment that could escape ./workers.
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		errf("❌ invalid template name %q\n", name)
		return 1
	}
	root := "workers/" + name
	if entries, err := ssgroot.EmbeddedWorkers.ReadDir(root); err != nil || len(entries) == 0 {
		errf("❌ unknown worker template %q. Available: %s\n", name, strings.Join(availableWorkerTemplates(), ", "))
		return 1
	}
	dest := filepath.Join("workers", name)
	// #nosec G703 -- name is validated above to a plain identifier under ./workers
	if _, err := os.Stat(dest); err == nil {
		errf("❌ %s already exists — refusing to overwrite\n", dest)
		return 1
	}
	// Looked up before extracting, so scaffolding core itself never counts.
	hasCore := modules.IsCore(filepath.Join("workers", modules.CoreID))
	if err := extractWorkerTemplate(root, dest); err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	fmt.Printf("✅ Scaffolded worker %q into %s/\n\n", name, dest)
	if toml, err := fs.ReadFile(ssgroot.EmbeddedWorkers, root+"/wrangler.toml"); err == nil {
		fmt.Print(standaloneWorkerInstructions(name, toml))
		fmt.Print(coreRegistrationNote(hasCore, name, toml))
		return 0
	}
	fmt.Print(workerConfigSnippet(name))
	fmt.Print(coreRegistrationNote(hasCore, name, nil))
	return 0
}

// availableWorkerTemplates lists the embedded worker template names.
func availableWorkerTemplates() []string {
	return workerTemplatesIn(ssgroot.EmbeddedWorkers)
}

// workerTemplatesIn lists the template directories under workers/ in fsys.
func workerTemplatesIn(fsys fs.FS) []string {
	entries, err := fs.ReadDir(fsys, "workers")
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
	return extractTemplateFrom(ssgroot.EmbeddedWorkers, root, dest)
}

// extractTemplateFrom copies the tree at root in fsys onto disk under dest.
// fs.WalkDir yields slash paths below root, so the relative path is a plain
// prefix cut.
func extractTemplateFrom(fsys fs.FS, root, dest string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, root), "/")))
		if d.IsDir() {
			// #nosec G301,G703 -- scaffolded dirs are world-traversable; dest is a validated ./workers path
			return os.MkdirAll(target, 0755)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		// #nosec G306,G703 -- scaffolded files are world-readable; dest is a validated ./workers path
		return os.WriteFile(target, data, 0644)
	})
}

// workerConfigSnippet returns the worker: block to add to .ssg.yaml.
func workerConfigSnippet(name string) string {
	include, exclude := templateRoutes(name)
	var routes strings.Builder
	routes.WriteString("  routes_include:\n")
	for _, r := range include {
		fmt.Fprintf(&routes, "    - %s\n", r)
	}
	if len(exclude) > 0 {
		routes.WriteString("  routes_exclude:\n")
		for _, r := range exclude {
			fmt.Fprintf(&routes, "    - %s\n", r)
		}
	}
	return fmt.Sprintf(`Add this to your .ssg.yaml:

worker:
  dir: workers/%s
  mode: functions
%s
Then set the secrets listed in workers/%s/README.md via `+"`wrangler pages secret put`"+`.
`, name, routes.String(), name)
}

// routesMarker opens the first line of a template's wrangler.snippet.toml when
// the template serves more than /api/* — auth's sign-in pages live at /auth/*,
// and its stylesheet and script under /auth/assets/ must stay plain files:
//
//	# auth — routes in .ssg.yaml: routes_include: [/api/auth/*, /auth/*], routes_exclude: [/auth/assets/*]
//
// Without the marker a template gets the generic /api/* it always got.
const routesMarker = "routes in .ssg.yaml:"

var routeListRE = regexp.MustCompile(`routes_(include|exclude):\s*\[([^\]]*)\]`)

// templateRoutes reads the routes a template declares on that first line, or
// returns the generic [/api/*] with no exclusions.
func templateRoutes(name string) (include, exclude []string) {
	include = []string{"/api/*"}
	data, err := fs.ReadFile(ssgroot.EmbeddedWorkers, "workers/"+name+"/wrangler.snippet.toml")
	if err != nil {
		return include, nil
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if !strings.Contains(first, routesMarker) {
		return include, nil
	}
	for _, m := range routeListRE.FindAllStringSubmatch(first, -1) {
		var list []string
		for _, r := range strings.Split(m[2], ",") {
			if r = strings.TrimSpace(r); r != "" {
				list = append(list, r)
			}
		}
		if m[1] == "include" && len(list) > 0 {
			include = list
		} else if m[1] == "exclude" {
			exclude = list
		}
	}
	return include, exclude
}

// standaloneWorkerInstructions tells the user how to deploy a template that is
// its own Worker. The steps come from the leading comment block of the
// template's wrangler.toml, so they live next to the config they describe and
// scaffold.go stays template-agnostic.
func standaloneWorkerInstructions(name string, wranglerToml []byte) string {
	var b strings.Builder
	dir := path.Join("workers", name)
	fmt.Fprintf(&b, "%s is a standalone Cloudflare Worker with its own wrangler.toml.\n", dir)
	b.WriteString("Do NOT add it to worker:/workers: in .ssg.yaml — that section is for Pages Functions,\n")
	b.WriteString("and `ssg new wrangler` leaves it out of the site's wrangler.toml.\n\n")
	for _, line := range strings.Split(string(wranglerToml), "\n") {
		if !strings.HasPrefix(line, "#") {
			break
		}
		b.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "#"), " "))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\nThen set the secrets listed in %s/README.md via `wrangler secret put`.\n", dir)
	return b.String()
}
