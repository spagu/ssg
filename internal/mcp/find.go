package mcp

// Finding a place in the theme without reading it (#190).
//
// The toolset was list / read / write, so answering "where is the page
// background set?" meant listing the theme and reading files until the line
// turned up. Measured on a migrated site that was ~10k tokens for a one-line
// CSS change, most of it spent locating the line rather than changing it.
//
// A find turns that from O(files read) into O(answer): matching line ranges
// with a few lines of context, which is also exactly the anchor an edit needs
// (#187). Nothing is indexed and nothing is installed — it walks the same
// directories the section is already confined to.

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	// findContextLines is how much of the surrounding file comes back with each
	// hit — enough to recognise the place and to copy an anchor out of.
	findContextLines = 2
	// findDefaultLimit caps a reply that would otherwise be the whole file set.
	findDefaultLimit = 20
	// findMaxFileSize skips anything too large to be theme source. A minified
	// bundle or an inlined font matches everything and helps nobody.
	findMaxFileSize = 512 * 1024
)

// FindHit is one matching region of one file. Exported because a search backend
// lives outside this package and hands its answers back in this shape (#190).
type FindHit struct {
	Path     string
	From, To int // 1-based, inclusive
	Fragment string
	// Note is backend-supplied detail shown beside the range — a relevance
	// score, say. Empty for a local scan, which ranks nothing.
	Note string
	// Matches counts how many matching lines this region covers, so a merged
	// window does not read as a single hit.
	Matches int
	// FromCol and ToCol narrow the locus to a character range within a line,
	// 1-based and inclusive. Zero when the whole line is the answer. They are
	// what keeps a hit useful in a minified file, where the line is the file
	// (#204).
	FromCol, ToCol int
}

// FindFragmentLines is how many lines of a document a backend that has no line
// information should return, so its answer stays comparable to a local hit.
const FindFragmentLines = 2*findContextLines + 1

// searchQuery is a compiled query. A query that is valid regular-expression
// syntax is used as one; anything else is matched literally, so a model that
// pastes `background: #fff` (where `#` is fine but `(` would not be) still gets
// an answer instead of a syntax error.
type searchQuery struct {
	re      *regexp.Regexp
	literal bool
}

// compileQuery prepares a query for matching, case-insensitively.
func compileQuery(q string) (searchQuery, error) {
	if strings.TrimSpace(q) == "" {
		return searchQuery{}, fmt.Errorf("`query` is required")
	}
	if re, err := regexp.Compile("(?i)" + q); err == nil {
		return searchQuery{re: re}, nil
	}
	return searchQuery{re: regexp.MustCompile("(?i)" + regexp.QuoteMeta(q)), literal: true}, nil
}

// searchFiles returns the matching regions across the given project files.
func searchFiles(root string, rels []string, q searchQuery, limit int) []FindHit {
	var hits []FindHit
	for _, rel := range rels {
		if len(hits) >= limit {
			break
		}
		hits = append(hits, searchOneFile(root, rel, q)...)
	}
	// Trimmed after the loop as well as before it: one file can hold more
	// regions than the whole reply is allowed to carry.
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// searchOneFile returns the matching regions of one file, merging hits whose
// context windows overlap so a run of adjacent matches is one fragment rather
// than five nearly identical ones.
func searchOneFile(root, rel string, q searchQuery) []FindHit {
	abs := joinProject(root, rel)
	info, err := os.Stat(abs)
	if err != nil || info.Size() > findMaxFileSize {
		return nil
	}
	b, err := os.ReadFile(abs) // #nosec G304 -- confined to the section's directories by the caller
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")

	// A long line is reported as a window around the match rather than merged
	// with its neighbours: merging assumes a line is a small thing (#204).
	var matched []int
	var long []FindHit
	for i, line := range lines {
		loc := q.re.FindStringIndex(line)
		if loc == nil {
			continue
		}
		if isLongLine(line) {
			frag, fromCol, toCol, _ := charWindow(line, loc[0], loc[1])
			long = append(long, FindHit{
				Path: rel, From: i + 1, To: i + 1, Fragment: capFragment(frag),
				FromCol: fromCol, ToCol: toCol, Matches: 1,
			})
			continue
		}
		matched = append(matched, i+1)
	}
	if len(matched) == 0 {
		return long
	}
	return append(long, mergeHits(rel, lines, matched)...)
}

// mergeHits turns matching line numbers into context windows, coalescing only
// windows that genuinely overlap.
//
// Merging windows that merely touch looks equivalent and is not: in a file
// where the query matches every few lines, each new window abuts the last and
// the whole file collapses into one region — which then counts as one hit, so
// the limit stops limiting and the reply is the file. Overlap is the honest
// test: two matches share a region when they share a line.
func mergeHits(rel string, lines []string, matched []int) []FindHit {
	var out []FindHit
	for _, ln := range matched {
		from, to := max(1, ln-findContextLines), min(len(lines), ln+findContextLines)
		if n := len(out); n > 0 && from <= out[n-1].To {
			out[n-1].To = max(out[n-1].To, to)
			out[n-1].Matches++
			continue
		}
		out = append(out, FindHit{Path: rel, From: from, To: to, Matches: 1})
	}
	for i := range out {
		out[i].Fragment = capFragment(strings.Join(lines[out[i].From-1:out[i].To], "\n"))
	}
	return out
}

// renderHits formats the reply: path, line range, then the fragment. The line
// range is what makes a follow-up edit anchorable without a read.
func renderHits(hits []FindHit, q string, literal bool) string {
	if len(hits) == 0 {
		note := ""
		if !literal {
			note = " (the query was treated as a regular expression)"
		}
		return fmt.Sprintf("No match for %q%s.", q, note)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d match%s for %q:\n", len(hits), plural(len(hits)), q)
	for _, h := range hits {
		note := ""
		if h.Note != "" {
			note = "  (" + h.Note + ")"
		}
		fmt.Fprintf(&b, "\n%s:%s%s\n%s\n", h.Path, locusOf(h.From, h.To, h.FromCol, h.ToCol), note, h.Fragment)
	}
	return strings.TrimRight(b.String(), "\n")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// intArg returns a positive integer argument, or fallback when absent or unusable.
// JSON numbers arrive as float64, which is why this is not a type assertion to int.
func intArg(args map[string]any, key string, fallback int) int {
	v, ok := args[key]
	if !ok {
		return fallback
	}
	f, ok := v.(float64)
	if !ok || f < 1 {
		return fallback
	}
	return int(f)
}

// sortedUnique returns rels in a stable order, so two identical queries answer
// identically.
func sortedUnique(rels []string) []string {
	sort.Strings(rels)
	return rels
}
