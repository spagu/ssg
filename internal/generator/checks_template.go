package generator

// A `<title>` inside a `<script>` block (#152).
//
// Go's html/template escapes by CONTEXT, not by expression. A `<script …>`
// start tag opens a JavaScript context that runs until the matching
// `</script>`, and everything interpolated inside it is escaped as a JS value —
// which for text means a "quoted" string. A scraped theme whose inline
// analytics or JSON-LD block lost its end tag therefore renders
//
//	<title>"Artisans of Taste" - "Bringing people…"</title>
//
// while the same expression in the body comes out clean. Every value is
// affected — the site title, the page title, a term's name — because the
// escaper is looking at where the value lands, not at where it came from.
//
// The escaping is correct: in a real script context those quotes are what keep
// a value from breaking out of its string. What was missing is anyone saying
// so. Finding it cost the reporter the better part of a day, with byte-clean
// inputs and a template that looks right in isolation.

import (
	"fmt"
	"regexp"
	"strings"
)

// scriptTagRe matches script start and end tags. A start tag is captured with
// its `/` absent; `</script>` with it present.
var scriptTagRe = regexp.MustCompile(`(?is)<(/?)script\b[^>]*>`)

// titleTagRe matches a `<title>` start tag.
var titleTagRe = regexp.MustCompile(`(?i)<title\b`)

// templateFinding is one defect found in a theme file, with the lines that
// explain it.
type templateFinding struct {
	File       string
	TitleLine  int
	ScriptLine int
	CloseLine  int // 0 when the script is never closed
}

// Message is the line a build prints.
func (f templateFinding) Message() string {
	closed := "never closed"
	if f.CloseLine > 0 {
		closed = fmt.Sprintf("closed line %d", f.CloseLine)
	}
	return fmt.Sprintf("%s: <title> on line %d is inside a <script> block (opened line %d, %s) — "+
		"html/template escapes values there as JavaScript, so they render quoted. "+
		"Close the script before the title.", f.File, f.TitleLine, f.ScriptLine, closed)
}

// checkTemplateScriptContext reports a `<title>` that sits inside a script
// block in one theme file. Only a title carrying an interpolation is reported:
// a static one in a script is odd but renders nothing surprising, and a warning
// nobody can act on is worse than none.
func checkTemplateScriptContext(file, src string) []templateFinding {
	var findings []templateFinding
	for _, span := range scriptSpans(src) {
		region := src[span.start:span.end]
		loc := titleTagRe.FindStringIndex(region)
		if loc == nil {
			continue
		}
		// The interpolation is what the escaper changes; a static title in a
		// script block is not worth a line in the log.
		titleEnd := strings.Index(strings.ToLower(region[loc[0]:]), "</title>")
		if titleEnd < 0 {
			titleEnd = len(region) - loc[0]
		}
		if !strings.Contains(region[loc[0]:loc[0]+titleEnd], "{{") {
			continue
		}
		f := templateFinding{
			File:       file,
			TitleLine:  lineOf(src, span.start+loc[0]),
			ScriptLine: lineOf(src, span.start),
		}
		if span.closed {
			f.CloseLine = lineOf(src, span.end)
		}
		findings = append(findings, f)
	}
	return findings
}

// scriptSpan is the region between a `<script …>` start tag and its end tag,
// or to the end of the file when there is none.
type scriptSpan struct {
	start, end int
	closed     bool
}

// scriptSpans finds every script region. Nested start tags cannot occur in
// HTML, so the first `</script>` closes the block — which is exactly how a
// browser and html/template both read it.
func scriptSpans(src string) []scriptSpan {
	var spans []scriptSpan
	tags := scriptTagRe.FindAllStringSubmatchIndex(src, -1)
	for i := 0; i < len(tags); i++ {
		if src[tags[i][2]:tags[i][3]] == "/" {
			continue // an end tag with no start before it: nothing to report
		}
		span := scriptSpan{start: tags[i][0], end: len(src)}
		for j := i + 1; j < len(tags); j++ {
			if src[tags[j][2]:tags[j][3]] == "/" {
				span.end, span.closed = tags[j][1], true
				i = j
				break
			}
		}
		spans = append(spans, span)
	}
	return spans
}

// lineOf returns the 1-based line a byte offset falls on.
func lineOf(src string, offset int) int {
	if offset > len(src) {
		offset = len(src)
	}
	return strings.Count(src[:offset], "\n") + 1
}
