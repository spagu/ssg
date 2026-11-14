package main

// The one line `ssg new worker` adds when the project already has the core
// worker (GO-144): how the new template reaches the module registry. Without
// workers/core the scaffold prints exactly what it always did.

import (
	"fmt"
	"regexp"
)

// routePatternRe finds the path of the first `pattern = "<host>/<path>*"` in
// a standalone Worker's wrangler.toml — the base path it owns on the site.
var routePatternRe = regexp.MustCompile(`(?m)^\s*\{?\s*pattern\s*=\s*"[^"/]*(/[^"*]*)\*?"`)

// coreRegistrationNote is empty without core. A Pages template joins the
// registry by being listed in workers:; a standalone Worker (wranglerToml
// set) needs a modules: entry, whose url is taken from its route.
func coreRegistrationNote(hasCore bool, name string, wranglerToml []byte) string {
	if !hasCore {
		return ""
	}
	if wranglerToml == nil {
		return fmt.Sprintf("\nworkers/core is in this project: once workers/%s is in workers:, the next build adds it to the core worker's module registry.\n", name)
	}
	url := "/" + name
	if m := routePatternRe.FindSubmatch(wranglerToml); m != nil && len(m[1]) > 1 {
		url = string(m[1])
	}
	return fmt.Sprintf(`
workers/core is in this project: to register %s with it, add this to .ssg.yaml:

modules:
  - id: %s
    url: %s
    dir: workers/%s
`, name, name, url, name)
}
