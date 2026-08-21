package main

// The preview serves the `_redirects` it just generated (#181).
//
// `ssg` writes `<output>/_redirects` on every build — from `redirects:` and from
// frontmatter `aliases:` — and the documentation is clear that Cloudflare Pages
// and Netlify serve that file as it stands. The built-in server did not read it.
//
// So a redirect could not be checked before it was published. A site carrying
// fifty redirects from an old CMS — the normal case after a migration, since
// every one of them is a URL somebody else published — served all fifty as 404s
// locally, and the first anyone knew about whether the rules were right was
// production. Redirects are the part of a migration whose absence is invisible
// from the inside: the new site looks perfect and everybody arriving from the
// old one gets a 404.
//
// It also split the preview from the deployment in a way nothing else here
// does. `endpoints:` went deliberately the other way — the built-in server runs
// the same declaration the platform compiles — and that is exactly the property
// that makes endpoints testable. Nothing new needs declaring to close this: the
// file is already written on every build, so serving it is reading what is
// there.
//
// # Which platform's semantics
//
// Cloudflare Pages'. `_redirects` generation targets it first
// (generateCloudflareFiles), and there **redirect rules are evaluated before
// static assets** — a rule shadows a file sitting at the same path. Choosing one
// platform's rules and naming it is more useful than averaging two. Within the
// file, order is honoured as written and the first match wins, which is also
// Netlify's rule; the generator already emits exact rules before wildcard ones
// for exactly that reason.
//
// A `!` force marker is parsed and accepted. Under these semantics everything
// shadows already, so it changes nothing here — but a file the build wrote must
// never be a file the server chokes on.
//
// The table is swapped whole, never mutated, on the same atomic-pointer pattern
// as the endpoint routes (#180): a request sees the rules from before the
// rebuild or the rules from after it, never a half-parsed file.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/spagu/ssg/internal/config"
)

// redirectRule is one parsed line of the generated file.
type redirectRule struct {
	from    string
	to      string
	status  int
	forced  bool // the `!` marker: parsed, and inert under Cloudflare semantics
	dynamic bool // contains a `*` splat or a `:placeholder` segment
}

// redirectTable is an ordered rule list. Immutable once published.
type redirectTable []redirectRule

// liveRedirects holds the rules the server is currently answering with.
var liveRedirects atomic.Pointer[redirectTable]

// redirectStatuses are the codes a rule may carry. It is the set
// validateRedirects accepts, so the server honours exactly what the build was
// willing to write.
var redirectStatuses = map[int]bool{301: true, 302: true, 303: true, 307: true, 308: true, 410: true}

// parseRedirectsFile reads the `/from /to status[!]` format back. A malformed
// line is skipped with one warning naming it and the rest of the file stays
// live — the server must never fail over a file the build itself wrote.
func parseRedirectsFile(text string, warn func(string, ...any)) redirectTable {
	table := redirectTable{}
	for n, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		rule, err := parseRedirectLine(trimmed)
		if err != nil {
			warn("⚠️  _redirects:%d: %v — rule skipped: %s", n+1, err, trimmed)
			continue
		}
		table = append(table, rule)
	}
	return table
}

// parseRedirectLine parses one rule.
func parseRedirectLine(line string) (redirectRule, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return redirectRule{}, fmt.Errorf("expected `/from /to [status]`")
	}
	rule := redirectRule{from: fields[0], to: fields[1], status: http.StatusMovedPermanently}
	if !strings.HasPrefix(rule.from, "/") {
		return redirectRule{}, fmt.Errorf("source %q does not start with /", rule.from)
	}
	if len(fields) > 2 {
		code := fields[2]
		if forced, ok := strings.CutSuffix(code, "!"); ok {
			rule.forced, code = true, forced
		}
		n, err := strconv.Atoi(code)
		if err != nil {
			return redirectRule{}, fmt.Errorf("status %q is not a number", fields[2])
		}
		rule.status = n
	}
	if !redirectStatuses[rule.status] {
		return redirectRule{}, fmt.Errorf("status %d is not a redirect status", rule.status)
	}
	rule.dynamic = strings.Contains(rule.from, "*") || strings.Contains(rule.from, ":")
	return rule, nil
}

// match returns the destination for path under the first matching rule.
func (t redirectTable) match(path string) (redirectRule, string, bool) {
	for _, rule := range t {
		if dest, ok := rule.match(path); ok {
			return rule, dest, true
		}
	}
	return redirectRule{}, "", false
}

// match tests one rule against a request path, substituting `:splat` and any
// `:placeholder` captures into the destination.
func (r redirectRule) match(path string) (string, bool) {
	if !r.dynamic {
		return r.to, path == r.from
	}
	fromSegs := strings.Split(strings.TrimPrefix(r.from, "/"), "/")
	pathSegs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	captures := map[string]string{}
	for i, seg := range fromSegs {
		if seg == "*" { // a splat is always last and swallows the remainder
			return substituteRedirect(r.to, captures, strings.Join(pathSegs[min(i, len(pathSegs)):], "/")), true
		}
		if i >= len(pathSegs) {
			return "", false
		}
		if name, ok := strings.CutPrefix(seg, ":"); ok && name != "" {
			if pathSegs[i] == "" {
				return "", false
			}
			captures[name] = pathSegs[i]
			continue
		}
		if seg != pathSegs[i] {
			return "", false
		}
	}
	if len(pathSegs) != len(fromSegs) {
		return "", false
	}
	return substituteRedirect(r.to, captures, ""), true
}

// substituteRedirect fills `:splat` and `:name` into a destination. Longer names
// are replaced first so `:id` never eats the front of `:idx`.
func substituteRedirect(to string, captures map[string]string, splat string) string {
	names := make([]string, 0, len(captures))
	for name := range captures {
		names = append(names, name)
	}
	sortByLengthDesc(names)
	for _, name := range names {
		to = strings.ReplaceAll(to, ":"+name, captures[name])
	}
	return strings.ReplaceAll(to, ":splat", splat)
}

// sortByLengthDesc orders names longest-first; the lists are a handful of
// segments, so an insertion sort is the honest amount of machinery.
func sortByLengthDesc(names []string) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && len(names[j]) > len(names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}

// liveRedirectHandler answers a request the published rules match, and passes
// everything else on. Before the first publish it is a pure pass-through, so a
// server built without one behaves exactly as it did.
func liveRedirectHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if table := liveRedirects.Load(); table != nil {
			if rule, dest, ok := table.match(r.URL.Path); ok {
				if rule.status == http.StatusGone {
					// 410 is an answer, not a destination: no Location.
					http.Error(w, "410 Gone", http.StatusGone)
					return
				}
				http.Redirect(w, r, dest, rule.status)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// publishRedirects re-reads <output>/_redirects and makes it the live table.
func publishRedirects(outputDir string, warn func(string, ...any)) {
	table := parseRedirectsFile(readOutputRules(outputDir, "_redirects"), warn)
	liveRedirects.Store(&table)
}

// readOutputRules reads one of the generated rule files. A file that is not
// there yet is empty, not an error: a server can be built before a build has
// written anything.
func readOutputRules(outputDir, name string) string {
	b, err := os.ReadFile(filepath.Join(outputDir, name)) // #nosec G304 -- the server reads its own output tree
	if err != nil {
		return ""
	}
	return string(b)
}

// republishOutputRules re-reads both generated rule files. Called where the
// server is built and again after every rebuild — unlike the endpoint table
// these files change on every build, not only when the config is edited.
func republishOutputRules(cfg *config.Config) {
	warn := func(format string, a ...any) {
		if !cfg.Quiet {
			errf(format+"\n", a...)
		}
	}
	publishRedirects(cfg.OutputDir, warn)
	publishHeaders(cfg.OutputDir, warn)
}
