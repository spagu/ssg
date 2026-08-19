package generator

// Invisible characters that survive into published HTML (#176).
//
// Content reaches a build from a CMS export, a word processor, a chat window or
// a clipboard, and it carries characters that render as nothing and break
// things. A zero-width space splits a word for Ctrl+F, for the site's own search
// index and for a screen reader, so a visitor searching a page for a word that
// is plainly on it finds nothing. A bidi override makes text render in a
// different order than it is stored, which can make a link's visible text
// disagree with where it goes. None of it is authored on purpose, all of it
// survives a migration, and nothing reported it.
//
// The whole difficulty is restraint. A page documenting these characters needs
// them literally, and that page is exactly the one a careless pass would ruin.
// So:
//
//   - never inside <pre>, <code>, <script>, <style> or <textarea>;
//   - a leading BOM is a BOM, stripped only mid-document;
//   - U+00A0 is typography between a number and its unit, and residue only in
//     runs of two or more.

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// invisibleClass names a group of characters for the report, so an operator
// reads "bidi override" rather than a code point.
type invisibleClass struct {
	Name  string
	Chars []rune
}

// invisibleClasses are what is removed. Each is invisible in every renderer and
// changes how the page behaves; nothing here is a character an author types.
//
// Written as escapes on purpose: a source file carrying these literally is one
// nobody can review, which is the same problem this code exists to fix.
var invisibleClasses = []invisibleClass{
	{"zero-width space", []rune{'\u200B'}},
	{"zero-width joiner", []rune{'\u200C', '\u200D'}},
	{"word joiner", []rune{'\u2060'}},
	{"soft hyphen", []rune{'\u00AD'}},
	// Trojan Source: text that renders in a different order than it is stored,
	// so a link's visible text can disagree with where it goes.
	{"bidi override", []rune{
		'\u202A', '\u202B', '\u202C', '\u202D', '\u202E', // embedding and override
		'\u2066', '\u2067', '\u2068', '\u2069', // isolates
	}},
	{"byte order mark", []rune{'\uFEFF'}},
}

// exoticSpaces are real spaces of unusual width. They are normalised to an
// ordinary space rather than removed: the word break is meant, the width is
// word-processor residue that breaks line wrapping and copy-paste matching.
var exoticSpaces = []rune{
	'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006',
	'\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000',
}

// isTagChar reports whether r is in the Unicode tag block: invisible in every
// renderer, and a way to carry text only a machine reads.
func isTagChar(r rune) bool { return r >= 0xE0000 && r <= 0xE007F }

// sanitizeCounts is what one page's pass removed, by class name.
type sanitizeCounts map[string]int

// total is how many characters were removed in all.
func (c sanitizeCounts) total() int {
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}

// merge folds another page's counts in.
func (c sanitizeCounts) merge(other sanitizeCounts) {
	for k, v := range other {
		c[k] += v
	}
}

// summary renders the counts in a stable order, commonest first.
func (c sanitizeCounts) summary() string {
	names := make([]string, 0, len(c))
	for k := range c {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if c[names[i]] != c[names[j]] {
			return c[names[i]] > c[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s %d", n, c[n]))
	}
	return strings.Join(parts, " · ")
}

// classOf returns the report name for a character that must be removed, or ""
// for one that stays.
func classOf(r rune) string {
	for _, class := range invisibleClasses {
		for _, c := range class.Chars {
			if r == c {
				return class.Name
			}
		}
	}
	if isTagChar(r) {
		return "tag characters"
	}
	// Anything else in the format category is invisible by definition and was
	// not authored — this is what catches the next such character to be
	// standardised, rather than only the ones known today.
	if unicode.Is(unicode.Cf, r) {
		return "format character"
	}
	return ""
}

// sanitizeHTML removes invisible characters from generated HTML, returning the
// cleaned document and what was removed.
//
// Verbatim regions are skipped entirely: a page about these characters, or a
// code sample containing one, must survive unchanged.
//
// The walk is linear. An earlier version asked "is this offset inside any
// protected span?" per character, which is quadratic in the number of spans and
// hung a real build for twenty minutes on a documentation page with a few
// thousand inline <code> elements. The spans are sorted and the document is
// walked in order, so one cursor over them is enough.
func sanitizeHTML(s string) (string, sanitizeCounts) {
	counts := sanitizeCounts{}
	protected := verbatimSpans(s)

	var b strings.Builder
	b.Grow(len(s))
	next := 0     // the first span that may still cover the cursor
	spaceRun := 0 // consecutive non-breaking spaces

	for i, r := range s {
		// Advance past spans the cursor has already left. Both the document and
		// the span list are in order, so this pointer only moves forward.
		for next < len(protected) && protected[next].to <= i {
			next++
		}
		if next < len(protected) && i >= protected[next].from {
			b.WriteRune(r)
			spaceRun = 0
			continue
		}
		// A leading BOM is a BOM. Only a stray one mid-document is residue.
		if r == '\uFEFF' && i == 0 {
			b.WriteRune(r)
			continue
		}
		if name := classOf(r); name != "" {
			counts[name]++
			spaceRun = 0
			continue
		}
		if isExoticSpace(r) {
			counts["exotic space"]++
			b.WriteRune(' ')
			spaceRun = 0
			continue
		}
		// A non-breaking space between a number and its unit is typography. A
		// RUN of them is a word processor holding a line together, and it stops
		// the line wrapping where wrapping was wanted.
		if r == '\u00A0' {
			spaceRun++
			if spaceRun >= 2 {
				counts["non-breaking space run"]++
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(r)
			continue
		}
		spaceRun = 0
		b.WriteRune(r)
	}
	return b.String(), counts
}

// isExoticSpace reports whether r is a space of unusual width.
func isExoticSpace(r rune) bool {
	for _, c := range exoticSpaces {
		if r == c {
			return true
		}
	}
	return false
}

// span is a half-open BYTE range that must not be touched. Byte offsets rather
// than rune indices: the walk ranges over the string, which yields byte offsets,
// and converting between the two cost an allocation per document for nothing.
type span struct{ from, to int }

// verbatimTags are the elements whose content is shown or executed as written.
var verbatimTags = []string{"pre", "code", "script", "style", "textarea"}

// verbatimSpans locates every region that must survive unchanged, sorted and
// merged so the caller can walk them with a single forward cursor.
func verbatimSpans(s string) []span {
	lower := strings.ToLower(s)
	var out []span

	for _, tag := range verbatimTags {
		open, closing := "<"+tag, "</"+tag+">"
		for at := 0; ; {
			rel := strings.Index(lower[at:], open)
			if rel < 0 {
				break
			}
			start := at + rel
			// "<code" must not match "<codex": the next character has to end
			// the tag name.
			after := start + len(open)
			if after < len(lower) && !isTagBoundary(rune(lower[after])) {
				at = after
				continue
			}
			end := strings.Index(lower[start:], closing)
			if end < 0 {
				// Unclosed: protect the remainder rather than guess where it ends.
				out = append(out, span{start, len(s)})
				break
			}
			stop := start + end + len(closing)
			out = append(out, span{start, stop})
			at = stop
		}
	}
	return mergeSpans(out)
}

// mergeSpans sorts and coalesces overlapping regions — <pre><code> produces two
// that overlap, and a merged list is what makes the single-cursor walk correct.
func mergeSpans(in []span) []span {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(a, b int) bool { return in[a].from < in[b].from })
	out := in[:1]
	for _, sp := range in[1:] {
		last := &out[len(out)-1]
		if sp.from <= last.to {
			if sp.to > last.to {
				last.to = sp.to
			}
			continue
		}
		out = append(out, sp)
	}
	return out
}

// isTagBoundary reports whether c ends an HTML tag name.
func isTagBoundary(c rune) bool {
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// sanitizeMode resolves the configured behaviour. Unset means on: the failure
// this prevents is invisible, so a site that has not thought about it is the
// one that most needs the default.
func (g *Generator) sanitizeMode() string {
	switch strings.ToLower(strings.TrimSpace(g.config.SanitizeOutput)) {
	case "off", "false", "no":
		return "off"
	case "warn", "report":
		return "warn"
	default:
		return "on"
	}
}

// sanitizeHTMLString is the pipeline step: it cleans the page and records what
// it removed, so one line can report the whole build rather than one per page.
func (g *Generator) sanitizeHTMLString(s string) string {
	mode := g.sanitizeMode()
	if mode == "off" {
		return s
	}
	cleaned, counts := sanitizeHTML(s)
	if counts.total() == 0 {
		return s
	}
	g.recordSanitized(counts)
	if mode == "warn" {
		return s
	}
	return cleaned
}

// recordSanitized accumulates the build's totals. Rendering is concurrent, so
// this is the one place that touches them.
func (g *Generator) recordSanitized(counts sanitizeCounts) {
	g.sanitizeMu.Lock()
	defer g.sanitizeMu.Unlock()
	if g.sanitized == nil {
		g.sanitized = sanitizeCounts{}
	}
	g.sanitized.merge(counts)
	g.sanitizedPages++
}

// reportSanitized prints one line for the whole build. Silent when there was
// nothing to remove, which is every already-clean site.
func (g *Generator) reportSanitized() {
	g.sanitizeMu.Lock()
	defer g.sanitizeMu.Unlock()
	total := g.sanitized.total()
	if total == 0 || g.config.Quiet {
		return
	}
	verb := "Removed"
	if g.sanitizeMode() == "warn" {
		verb = "Found"
	}
	fmt.Printf("   🧹 %s %d invisible character(s) in %d page(s)\n", verb, total, g.sanitizedPages)
	fmt.Printf("      %s\n", g.sanitized.summary())
	if g.sanitizeMode() == "warn" {
		fmt.Println("      sanitize_output: warn — reporting only. Set it to `on` to remove them.")
	}
}

// reportStrippedImages says how many published images lost their metadata —
// which is how an operator learns their library carried locations at all.
func (g *Generator) reportStrippedImages() {
	g.sanitizeMu.Lock()
	defer g.sanitizeMu.Unlock()
	if g.strippedImages == 0 || g.config.Quiet {
		return
	}
	fmt.Printf("   🧼 Removed EXIF/IPTC metadata from %d published image(s)\n", g.strippedImages)
	fmt.Println("      GPS coordinates and camera serial numbers travel in it. `image_metadata: keep` to publish it.")
}
