package generator

// Which declared sitemap claims a URL.
//
// The narrowings mirror `feeds:` (#86) — the same question asked of the same
// kind of set — and are combined with AND. A spec that names none of them
// claims everything still unclaimed, which is how a catch-all is written; since
// selection is a partition and the first match wins, that spec belongs last.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// sitemapSpecPath is where a declared sitemap is written, sanitized into the
// output tree.
func sitemapSpecPath(spec models.SitemapSpec) string {
	rel := models.SanitizeRelPath(strings.TrimSpace(spec.Path))
	if rel == "" {
		return defaultSitemapName
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".xml") {
		rel += ".xml"
	}
	return filepath.ToSlash(rel)
}

// sitemapSpecMatches reports whether an entry belongs to a declared sitemap.
// The narrowings are combined with AND, and a spec with none of them claims
// everything left — which is how a catch-all is written.
func sitemapSpecMatches(spec models.SitemapSpec, e sitemapEntry) bool {
	if len(spec.Include) > 0 && !containsKind(spec.Include, e.kind) {
		return false
	}
	if s := strings.TrimSpace(spec.Source); s != "" && !sourceDirMatches(e.source, s) {
		return false
	}
	return true
}

// containsKind reports whether a declared include list names this kind.
func containsKind(include []string, kind sitemapKind) bool {
	for _, name := range include {
		if strings.EqualFold(strings.TrimSpace(name), string(kind)) {
			return true
		}
	}
	return false
}

// validateSitemapSpecs rejects an `include:` naming something selectable by
// nothing — a typo there would otherwise write an empty file and say only that
// it matched nothing.
func (g *Generator) validateSitemapSpecs() error {
	known := make(map[string]bool, len(sitemapKinds))
	for _, k := range sitemapKinds {
		known[string(k)] = true
	}
	for _, spec := range g.config.Sitemaps {
		for _, name := range spec.Include {
			if !known[strings.ToLower(strings.TrimSpace(name))] {
				return fmt.Errorf("sitemaps: %q includes unknown kind %q; valid: %s",
					sitemapSpecPath(spec), name, strings.Join(kindNames(), ", "))
			}
		}
	}
	return nil
}

// kindNames lists the selectable kinds, sorted, for an error message.
func kindNames() []string {
	out := make([]string, 0, len(sitemapKinds))
	for _, k := range sitemapKinds {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}
