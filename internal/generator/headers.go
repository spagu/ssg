package generator

// Configurable _headers generation for Cloudflare Pages (GO-064). The default
// blocks reproduce the historical hardcoded output byte-for-byte; the `headers:`
// config section overrides a default block per pattern or adds new patterns.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// headerBlock is one pattern block in the generated _headers file. Order
// matters twice: Cloudflare applies the first matching value per header, and
// the rendered file must be deterministic across builds.
type headerBlock struct {
	Comment string      // optional comment line rendered above the pattern
	Pattern string      // path pattern, e.g. "/*", "/css/*"
	Headers [][2]string // ordered name/value pairs
}

// defaultHeaderBlocks returns the built-in _headers blocks. An empty `headers:`
// config writes exactly these.
func defaultHeaderBlocks() []headerBlock {
	cacheYear := [2]string{"Cache-Control", "public, max-age=31536000, immutable"}
	// Pages revalidate on every request (#290). A deploy is atomic, so a copy
	// of a page is only ever worth a conditional request: an hour of max-age
	// kept a withdrawn post or a correction in front of readers for that hour,
	// and on the edge for as long as the zone liked.
	cacheHTML := [2]string{"Cache-Control", "public, max-age=0, must-revalidate"}
	return []headerBlock{
		{
			Comment: "Security headers for all pages",
			Pattern: "/*",
			Headers: [][2]string{
				{"X-Content-Type-Options", "nosniff"},
				{"X-Frame-Options", "DENY"},
				{"X-XSS-Protection", "1; mode=block"},
				{"Referrer-Policy", "strict-origin-when-cross-origin"},
				// (self), not (): the empty allowlist disabled the API for the
				// site's own pages too, so a "use my location" button was refused
				// without the visitor ever seeing a prompt — in preview and in
				// production alike, with nothing in the build to point at the
				// header (#287). (self) still blocks every framed third party,
				// which is what the policy is for; the browser still asks.
				{"Permissions-Policy", "geolocation=(self), microphone=(self), camera=(self)"},
			},
		},
		{Comment: "Cache static assets for 1 year", Pattern: "/css/*", Headers: [][2]string{cacheYear}},
		{Pattern: "/js/*", Headers: [][2]string{cacheYear}},
		{Pattern: "/images/*", Headers: [][2]string{cacheYear}},
		{Pattern: "/media/*", Headers: [][2]string{cacheYear}},
		{Comment: "HTML pages revalidate on every request", Pattern: "/*.html", Headers: [][2]string{cacheHTML}},
		// A page_format: directory site is requested as /blog/, not
		// /blog/index.html, and patterns match the request path: without this
		// block the rule above covered the front page and nothing else (#290).
		// It overlaps no asset block, so the year above is untouched.
		{Pattern: "/*/", Headers: [][2]string{cacheHTML}},
		{Pattern: "/", Headers: [][2]string{cacheHTML}},
	}
}

// mergeHeaderBlocks merges config overrides over the default blocks. A pattern
// present in overrides is merged header by header into that default block,
// keeping its position (#287): a header it names takes the new value, an empty
// value removes the header, and one the default lacks is added after the rest,
// sorted. Unknown patterns are appended, sorted alphabetically, with their
// headers sorted too — YAML maps carry no order, and the output must be
// reproducible. defaultsOff drops the defaults entirely.
//
// The override used to replace the block, so changing one Permissions-Policy
// meant restating four unrelated security headers — and forgetting one dropped
// X-Frame-Options without a word.
func mergeHeaderBlocks(defaults []headerBlock, overrides map[string]map[string]string, defaultsOff bool) []headerBlock {
	var blocks []headerBlock
	used := make(map[string]bool, len(overrides))
	if !defaultsOff {
		for _, b := range defaults {
			if hdrs, ok := overrides[b.Pattern]; ok {
				used[b.Pattern] = true
				blocks = append(blocks, headerBlock{Comment: b.Comment, Pattern: b.Pattern, Headers: mergeHeaderPairs(b.Headers, hdrs)})
				continue
			}
			blocks = append(blocks, b)
		}
	}
	extra := make([]string, 0, len(overrides))
	for pattern := range overrides {
		if !used[pattern] {
			extra = append(extra, pattern)
		}
	}
	sort.Strings(extra)
	for _, pattern := range extra {
		blocks = append(blocks, headerBlock{Pattern: pattern, Headers: sortedHeaderPairs(overrides[pattern])})
	}
	return blocks
}

// mergeHeaderPairs applies overrides to a default block's pairs. Header names
// match case-insensitively, as HTTP matches them; the override's spelling wins.
func mergeHeaderPairs(defaults [][2]string, overrides map[string]string) [][2]string {
	byName := make(map[string]string, len(overrides)) // lower-case name → override key
	for name := range overrides {
		byName[strings.ToLower(name)] = name
	}
	merged := make([][2]string, 0, len(defaults)+len(overrides))
	for _, pair := range defaults {
		name, overridden := byName[strings.ToLower(pair[0])]
		if !overridden {
			merged = append(merged, pair)
			continue
		}
		delete(byName, strings.ToLower(pair[0]))
		if value := overrides[name]; strings.TrimSpace(value) != "" {
			merged = append(merged, [2]string{name, value})
		}
	}
	added := make(map[string]string, len(byName))
	for _, name := range byName {
		if strings.TrimSpace(overrides[name]) != "" {
			added[name] = overrides[name]
		}
	}
	return append(merged, sortedHeaderPairs(added)...)
}

// sortedHeaderPairs flattens a header map into name/value pairs sorted by name.
func sortedHeaderPairs(hdrs map[string]string) [][2]string {
	names := make([]string, 0, len(hdrs))
	for name := range hdrs {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([][2]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, [2]string{name, hdrs[name]})
	}
	return pairs
}

// renderHeadersFile renders blocks in the Cloudflare/Netlify _headers format.
func renderHeadersFile(blocks []headerBlock) string {
	var b strings.Builder
	b.WriteString("# Cloudflare Pages Headers\n# Generated by SSG\n")
	for _, block := range blocks {
		b.WriteString("\n")
		if block.Comment != "" {
			b.WriteString("# " + block.Comment + "\n")
		}
		b.WriteString(block.Pattern + "\n")
		for _, h := range block.Headers {
			b.WriteString("  " + h[0] + ": " + h[1] + "\n")
		}
	}
	return b.String()
}

// generateHeadersFile writes the merged _headers file into the output root.
func (g *Generator) generateHeadersFile() error {
	blocks := mergeHeaderBlocks(defaultHeaderBlocks(), g.config.Headers, g.config.HeadersDefaultsOff)
	content := renderHeadersFile(blocks)
	headersPath := filepath.Join(g.config.OutputDir, "_headers")
	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(headersPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing _headers: %w", err)
	}
	return nil
}
