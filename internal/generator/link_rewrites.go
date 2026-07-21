package generator

import (
	"regexp"
	"sort"
	"strings"
)

// Link rewrites (LINK-002). Documentation that lives in a repository links to
// files beside it — `../examples/`, `../.ssg.yaml.example`, `../LICENSE`. Those
// links are correct in a repository view and dead in the built site, where the
// target was never published. link_rewrites maps a href prefix to whatever
// should replace it, usually a repository URL:
//
//	link_rewrites:
//	  "../examples/": "https://github.com/spagu/ssg/tree/main/examples/"
//	  "../.ssg.yaml.example": "https://github.com/spagu/ssg/blob/main/.ssg.yaml.example"
//
// The longest matching prefix wins, so a specific file can override the folder
// rule above it. An empty map (the default) rewrites nothing.

// hrefRe captures the value of any href attribute in rendered HTML.
var hrefRe = regexp.MustCompile(`href="([^"]*)"`)

// applyLinkRewrites replaces configured href prefixes in rendered content.
func (g *Generator) applyLinkRewrites(html string) string {
	if len(g.config.LinkRewrites) == 0 {
		return html
	}
	prefixes := g.linkRewritePrefixes()
	return hrefRe.ReplaceAllStringFunc(html, func(match string) string {
		href := match[6 : len(match)-1]
		for _, prefix := range prefixes {
			if strings.HasPrefix(href, prefix) {
				return `href="` + g.config.LinkRewrites[prefix] + strings.TrimPrefix(href, prefix) + `"`
			}
		}
		return match
	})
}

// linkRewritePrefixes returns the configured prefixes longest-first, so the
// most specific rule wins regardless of map iteration order. Computed once per
// build and cached: content rendering calls this for every page.
func (g *Generator) linkRewritePrefixes() []string {
	if g.linkRewriteKeys != nil {
		return g.linkRewriteKeys
	}
	keys := make([]string, 0, len(g.config.LinkRewrites))
	for prefix := range g.config.LinkRewrites {
		if prefix != "" {
			keys = append(keys, prefix)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j] // stable for equal lengths
	})
	g.linkRewriteKeys = keys
	return keys
}
