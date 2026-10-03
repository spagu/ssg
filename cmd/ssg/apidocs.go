package main

// Wiring for documentation from code (1.8.69, GO-107): `api_docs:` becomes
// the generator's per-package options.

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// buildAPIDocsOptions turns each api_docs entry into generator options.
func buildAPIDocsOptions(entries []config.APIDocsConfig) []generator.APIDocsOptions {
	out := make([]generator.APIDocsOptions, 0, len(entries))
	for _, e := range entries {
		root := e.Root
		if root == "" {
			root = "."
		}
		out = append(out, generator.APIDocsOptions{
			Source: apisource.Config{Name: e.Name, Root: root, Entries: e.Entry, Include: e.Include, Exclude: e.Exclude,
				Language: strings.ToLower(strings.TrimSpace(e.Language))},
			URL: e.URL, Visibility: strings.ToLower(strings.TrimSpace(e.Visibility)), Stability: e.Stability,
			Readme:    e.Readme == nil || *e.Readme,
			SourceURL: e.SourceURL, SourceRef: sourceRef(e.SourceRef, root),
			Playground: strings.TrimSpace(e.Playground),
			OpenAPI:    strings.TrimSpace(e.OpenAPI), TryIt: e.TryIt == nil || *e.TryIt,
		})
	}
	return out
}

// sourceRef is the {ref} of source links: the configured tag or branch, or —
// for "auto" and empty — the commit git reports for root, so a link points
// at the code the pages were built from. Without git it is "HEAD".
func sourceRef(configured, root string) string {
	if c := strings.TrimSpace(configured); c != "" && c != "auto" {
		return c
	}
	// #nosec G204 -- fixed git arguments; root is the site's own configuration
	cmd := exec.Command("git", "-C", filepath.Clean(root), "rev-parse", "HEAD") // NOSONAR S4036: git is intentionally resolved from PATH (portable), reviewed
	out, err := cmd.Output()
	if err != nil {
		return "HEAD"
	}
	return strings.TrimSpace(string(out))
}
