package main

// `ssg import redirects` converts a Next.js redirects() rule set into the
// `redirects:` YAML block SSG consumes (GO-067). Two inputs: a JSON dump of the
// redirects array (the reliable path) or a next.config.(js|ts|mjs) file parsed
// heuristically. Output goes to stdout — nothing is written or clobbered.

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// dispatchSubcommand routes `ssg <verb> <noun> ...` to a handler. Returns
// (exitCode, true) when it handled the args, (0, false) to fall through to the
// normal positional build. Only known verb+noun pairs are claimed.
func dispatchSubcommand(args []string) (int, bool) {
	if len(args) < 2 {
		return 0, false
	}
	switch {
	case args[0] == "import" && args[1] == "redirects":
		return runImportRedirects(args[2:]), true
	case args[0] == "new" && args[1] == "worker":
		return runNewWorker(args[2:]), true
	}
	return 0, false
}

// dispatchSingleVerb routes single-verb subcommands like `ssg init`. Kept
// separate from dispatchSubcommand (which needs a verb+noun pair) because
// `init` takes an optional source-name argument, not a fixed noun.
func dispatchSingleVerb(args []string) (int, bool) {
	if len(args) >= 1 && args[0] == "init" {
		return runInit(args[1:]), true
	}
	return 0, false
}

// nextRedirect mirrors the shape of a Next.js redirects() entry.
type nextRedirect struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Permanent   *bool  `json:"permanent"`
	StatusCode  int    `json:"statusCode"`
	Has         any    `json:"has"`
	Missing     any    `json:"missing"`
}

// runImportRedirects parses the input and prints a redirects: YAML block.
func runImportRedirects(args []string) int {
	var jsonPath, configPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from-json":
			if i+1 < len(args) {
				jsonPath = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				configPath = args[i]
			}
		}
	}
	if jsonPath == "" && configPath == "" {
		fmt.Fprintln(os.Stderr, "usage: ssg import redirects <next.config.ts> | --from-json <redirects.json>")
		return 2
	}

	var rules []importedRule
	var warnings []string
	var err error
	if jsonPath != "" {
		rules, warnings, err = importRedirectsFromJSON(jsonPath)
	} else {
		rules, warnings, err = importRedirectsFromConfig(configPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return 1
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "⚠️  %s\n", w)
	}
	fmt.Print(renderRedirectsYAML(rules))
	return 0
}

// importedRule is a resolved rule ready to render as YAML.
type importedRule struct {
	From   string
	To     string
	Status int
}

// importRedirectsFromJSON reads a JSON array of Next.js redirect objects.
func importRedirectsFromJSON(path string) ([]importedRule, []string, error) {
	data, err := os.ReadFile(path) // #nosec G304,G703 -- path is an explicit CLI argument, not attacker-controlled
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var entries []nextRedirect
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, nil, fmt.Errorf("parsing %s as a JSON redirects array: %w", path, err)
	}
	return convertNextRedirects(entries)
}

// redirectsCallRe isolates the body of the redirects() function so array
// scanning does not pick up unrelated object literals (rewrites, headers).
var redirectsCallRe = regexp.MustCompile(`(?s)redirects\s*\([^)]*\)\s*\{(.*)`)

// importRedirectsFromConfig heuristically extracts redirect literals from a
// next.config.(js|ts|mjs) file. Non-literal entries are reported, never dropped.
func importRedirectsFromConfig(path string) ([]importedRule, []string, error) {
	data, err := os.ReadFile(path) // #nosec G304,G703 -- path is an explicit CLI argument, not attacker-controlled
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	body := string(data)
	if m := redirectsCallRe.FindStringSubmatch(body); m != nil {
		body = m[1]
	}
	entries, warnings := parseNextRedirects(body)
	rules, convWarn, err := convertNextRedirects(entries)
	warnings = append(warnings, convWarn...)
	// Reconcile: every `source:` in the body should be accounted for by a
	// parsed rule or an explicit warning. A shortfall means an entry with a
	// shape the flat parser could not read (nested has/missing arrays,
	// multi-line objects) was dropped — surface it instead of hiding it.
	if got, want := len(rules)+len(warnings), countSources(body); want > got {
		warnings = append(warnings, fmt.Sprintf("%d redirect source(s) in %s could not be parsed automatically — dump the array with --from-json for a complete import", want-got, path))
	}
	return rules, warnings, err
}

// sourceKeyRe counts source: keys, the marker of a redirect entry.
var sourceKeyRe = regexp.MustCompile(`source\s*:`)

// countSources counts redirect entries by their source: key.
func countSources(body string) int {
	return len(sourceKeyRe.FindAllString(body, -1))
}

// objectLiteralRe matches a single { ... } object (no nested braces — Next.js
// redirect entries are flat except for has/missing arrays handled separately).
var (
	objectLiteralRe = regexp.MustCompile(`\{[^{}]*\}`)
	sourceRe        = regexp.MustCompile(`source\s*:\s*(['"` + "`" + `])((?:\\.|[^\\])*?)['"` + "`" + `]`)
	destinationRe   = regexp.MustCompile(`destination\s*:\s*(['"` + "`" + `])((?:\\.|[^\\])*?)['"` + "`" + `]`)
	permanentRe     = regexp.MustCompile(`permanent\s*:\s*(true|false)`)
	statusCodeRe    = regexp.MustCompile(`statusCode\s*:\s*(\d+)`)
)

// parseNextRedirects scans object literals for source/destination pairs. Any
// literal that has a source/destination but also a has:/missing: condition, a
// template-literal path (`...${...}`) or a regex-constrained param is flagged.
func parseNextRedirects(body string) ([]nextRedirect, []string) {
	var out []nextRedirect
	var warnings []string
	for _, obj := range objectLiteralRe.FindAllString(body, -1) {
		src := sourceRe.FindStringSubmatch(obj)
		dst := destinationRe.FindStringSubmatch(obj)
		if src == nil || dst == nil {
			continue
		}
		if src[1] == "`" || dst[1] == "`" || strings.Contains(src[2], "${") || strings.Contains(dst[2], "${") {
			warnings = append(warnings, fmt.Sprintf("skipped redirect with a template-literal path (source %q) — add it manually", src[2]))
			continue
		}
		if strings.Contains(obj, "has:") || strings.Contains(obj, "missing:") {
			warnings = append(warnings, fmt.Sprintf("skipped conditional redirect (source %q) — has/missing is not expressible in _redirects", src[2]))
			continue
		}
		entry := nextRedirect{Source: src[2], Destination: dst[2]}
		if p := permanentRe.FindStringSubmatch(obj); p != nil {
			b := p[1] == "true"
			entry.Permanent = &b
		}
		if s := statusCodeRe.FindStringSubmatch(obj); s != nil {
			entry.StatusCode, _ = strconv.Atoi(s[1])
		}
		out = append(out, entry)
	}
	return out, warnings
}

// convertNextRedirects turns Next.js entries into importedRules, converting the
// path syntax and resolving the status code.
func convertNextRedirects(entries []nextRedirect) ([]importedRule, []string, error) {
	var out []importedRule
	var warnings []string
	for _, e := range entries {
		if e.Has != nil || e.Missing != nil {
			warnings = append(warnings, fmt.Sprintf("skipped conditional redirect (source %q) — has/missing is not expressible in _redirects", e.Source))
			continue
		}
		from, to, err := nextPathToRedirect(e.Source, e.Destination)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped redirect (source %q): %v", e.Source, err))
			continue
		}
		out = append(out, importedRule{From: from, To: to, Status: nextStatus(e)})
	}
	return out, warnings, nil
}

// nextStatus resolves a Next.js entry's status: explicit statusCode wins, else
// permanent:true is 308-equivalent but SSG defaults to 301 (the SEO-canonical
// permanent redirect), and non-permanent is 302.
func nextStatus(e nextRedirect) int {
	if e.StatusCode != 0 {
		return e.StatusCode
	}
	if e.Permanent != nil && !*e.Permanent {
		return 302
	}
	return 301
}

// nextParamRe matches Next.js path params: :slug, :slug*, :slug+, and flags
// regex-constrained forms like :id(\d+) as unsupported.
var (
	nextCatchAllRe    = regexp.MustCompile(`:([A-Za-z0-9_]+)\*`)
	nextConstrainedRe = regexp.MustCompile(`:[A-Za-z0-9_]+\(`)
)

// nextPathToRedirect converts Next.js path syntax to _redirects syntax:
// `/:slug*` catch-all becomes `/*` (source) / `:splat` (destination); a plain
// `:slug` stays as a placeholder (shared syntax). Regex-constrained params are
// rejected — _redirects cannot express them.
func nextPathToRedirect(source, destination string) (string, string, error) {
	if nextConstrainedRe.MatchString(source) || nextConstrainedRe.MatchString(destination) {
		return "", "", fmt.Errorf("regex-constrained param is not supported by _redirects")
	}
	from := nextCatchAllRe.ReplaceAllString(source, "*")
	from = strings.ReplaceAll(from, ":splat", "")
	to := nextCatchAllRe.ReplaceAllString(destination, ":splat")
	return from, to, nil
}

// renderRedirectsYAML prints a ready-to-paste redirects: block.
func renderRedirectsYAML(rules []importedRule) string {
	var b strings.Builder
	b.WriteString("# Generated by `ssg import redirects` — review before committing.\n")
	b.WriteString("# Cloudflare Pages caps: 2000 static + 100 dynamic (wildcard) rules.\n")
	b.WriteString("redirects:\n")
	if len(rules) == 0 {
		b.WriteString("  []\n")
		return b.String()
	}
	for _, r := range rules {
		fmt.Fprintf(&b, "  - from: %q\n    to: %q\n    status: %d\n", r.From, r.To, r.Status)
	}
	return b.String()
}
