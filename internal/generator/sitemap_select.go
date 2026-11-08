package generator

// Which declared sitemap claims a URL.
//
// The narrowings mirror `feeds:` (#86) — the same question asked of the same
// kind of set — and are combined with AND. A spec that names none of them
// claims everything still unclaimed, which is how a catch-all is written; since
// selection is a partition and the first match wins, that spec belongs last.

import (
	"fmt"
	"os"
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

// sitemapSpecMatches reports whether an entry belongs to a declared sitemap;
// sitePath is the entry's URL path as `prefix:` sees it (see sitePath).
// The narrowings are combined with AND, and a spec with none of them claims
// everything left — which is how a catch-all is written.
func sitemapSpecMatches(spec models.SitemapSpec, e sitemapEntry, sitePath string) bool {
	if len(spec.Include) > 0 && !containsKind(spec.Include, e.kind) {
		return false
	}
	if s := strings.TrimSpace(spec.Source); s != "" && !inSource(e.source, e.section, e.external, s) {
		return false
	}
	if spec.Prefix.Set() && !spec.Prefix.MatchesPath(sitePath) {
		return false
	}
	return true
}

// sitePath is the path of an absolute site URL as `prefix:` compares it: after
// the domain (which carries the base path, #306) and after a language prefix,
// so `prefix: /openbsd/` claims /en/openbsd/x/ as well (#386). A URL off the
// site has no path here and matches no prefix.
func (g *Generator) sitePath(loc string) string {
	rest, ok := strings.CutPrefix(loc, httpsScheme+g.config.Domain)
	if !ok || (rest != "" && rest[0] != '/') {
		return ""
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	if !g.config.I18n.Enabled {
		return rest
	}
	for _, lang := range g.siteData.Languages {
		prefix := g.languagePrefix(lang.Code)
		if prefix == "" {
			continue
		}
		if tail, ok := strings.CutPrefix(rest, "/"+prefix); ok && (tail == "" || tail[0] == '/') {
			return "/" + strings.TrimPrefix(tail, "/")
		}
	}
	return rest
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

// validateSitemapSpecs rejects an `include:` or `source:` naming something
// selectable by nothing — a typo there would otherwise write an empty file and
// say only that it matched nothing.
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
		if s := strings.TrimSpace(spec.Source); s != "" && !g.knownSource(s) {
			return fmt.Errorf("sitemaps: %q selects source %q, which is neither a content folder nor an external_sources entry",
				sitemapSpecPath(spec), s)
		}
	}
	return nil
}

// knownSource reports whether a `source:` name exists: an external_sources
// entry, a folder under the content source, or the root or section of some
// loaded document (#386). A folder whose documents are all drafts still counts;
// its file is skipped as empty, as before.
func (g *Generator) knownSource(source string) bool {
	if _, ok := g.config.ExternalSources.Sources[strings.Trim(source, "/")]; ok {
		return true
	}
	dir := filepath.Join(g.config.ContentDir, g.config.Source, filepath.FromSlash(strings.Trim(source, "/")))
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return true
	}
	for _, set := range [][]models.Page{g.siteData.Pages, g.siteData.Posts} {
		for _, p := range set {
			if pageInSource(p, source) {
				return true
			}
		}
	}
	return false
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
