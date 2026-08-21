package main

// The preview serves the `_headers` it just generated (#181).
//
// The same preview-parity gap as `_redirects`, and reported alongside it: the
// build writes `<output>/_headers` on every run and the server ignored it, so
// the one place a `headers:` block could be checked before deployment was after
// deployment.
//
// Semantics follow the file the generator writes (renderHeadersFile): a pattern
// on its own line, its headers indented beneath it. Patterns are exact paths or
// globs — the defaults alone use `/*`, `/css/*`, `/*.html` and a bare `/` — and
// Cloudflare applies the **first** matching value for a given header name, which
// is why the merge order in headers.go is deliberate and why it is preserved
// here rather than re-sorted.
//
// The file wins over the server's own security-header middleware. That is not a
// preference: it is what the deployed platform would serve, and a preview whose
// headers differ from production is the failure this closes.

import (
	"net/http"
	"strings"
	"sync/atomic"
)

// headerRule is one pattern block: the pattern, and its headers in file order.
type headerRule struct {
	pattern string
	headers [][2]string
}

// headerTable is the parsed file. Immutable once published.
type headerTable []headerRule

// liveHeaders holds the header blocks the server is currently applying.
var liveHeaders atomic.Pointer[headerTable]

// parseHeadersFile reads the generated format back: an unindented, non-comment
// line opens a block, indented `Name: value` lines fill it. A line that is
// neither is skipped with one warning naming it — never a reason to fail.
func parseHeadersFile(text string, warn func(string, ...any)) headerTable {
	table := headerTable{}
	for n, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			table = append(table, headerRule{pattern: trimmed})
			continue
		}
		name, value, ok := strings.Cut(trimmed, ":")
		if !ok || strings.TrimSpace(name) == "" {
			warn("⚠️  _headers:%d: expected `Name: value` — line skipped: %s", n+1, trimmed)
			continue
		}
		if len(table) == 0 {
			warn("⚠️  _headers:%d: header before any pattern — line skipped: %s", n+1, trimmed)
			continue
		}
		last := len(table) - 1
		table[last].headers = append(table[last].headers,
			[2]string{strings.TrimSpace(name), strings.TrimSpace(value)})
	}
	return table
}

// apply sets every header the matching blocks declare for path, first match
// winning per header name — Cloudflare's rule, and the reason block order is
// preserved rather than sorted.
func (t headerTable) apply(h http.Header, path string) {
	var set map[string]bool
	for _, rule := range t {
		if !matchHeaderPattern(rule.pattern, path) {
			continue
		}
		for _, kv := range rule.headers {
			key := http.CanonicalHeaderKey(kv[0])
			if set[key] {
				continue
			}
			if set == nil {
				set = map[string]bool{}
			}
			set[key] = true
			h.Set(key, kv[1])
		}
	}
}

// matchHeaderPattern matches a path against a `_headers` pattern. `*` stands for
// any run of characters and may appear anywhere, which covers both the trailing
// `/css/*` shape and the `/*.html` shape the defaults emit.
func matchHeaderPattern(pattern, path string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == path
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	rest := path[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(rest, mid)
		if i < 0 {
			return false
		}
		rest = rest[i+len(mid):]
	}
	return strings.HasSuffix(rest, parts[len(parts)-1])
}

// liveHeadersHandler applies the published blocks to a response before it is
// written. It sits inside the security-header middleware, so a header named by
// the file replaces the built-in value rather than being replaced by it.
//
// Before the first publish it is a pure pass-through.
func liveHeadersHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if table := liveHeaders.Load(); table != nil {
			table.apply(w.Header(), r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

// publishHeaders re-reads <output>/_headers and makes it the live table.
func publishHeaders(outputDir string, warn func(string, ...any)) {
	table := parseHeadersFile(readOutputRules(outputDir, "_headers"), warn)
	liveHeaders.Store(&table)
}
