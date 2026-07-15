// Fenced-math rewriting (GO-055): ```math blocks become display math ($$…$$)
// before markdown conversion, so detection (containsMath), KaTeX injection
// (mathHTMLString requires a literal "$$") and browser-side auto-render all
// agree. Fences nested inside other code blocks are left untouched, mirroring
// the parser's fence tracking (GO-027). Inline \(...\) stays unsupported —
// CommonMark backslash-escaping would eat the delimiters.
package generator

import "strings"

const (
	fenceText = iota // outside any fence
	fenceCode        // inside a non-math code fence (left untouched)
	fenceMath        // inside a ```math fence being rewritten
)

// mathFenceRewriter is a tiny state machine that turns top-level ```math fences
// into $$ display math while leaving fences nested in other code blocks alone.
type mathFenceRewriter struct {
	mode int
	out  []string
}

func isFenceLine(t string) bool {
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// step consumes one line, updating the state and the output buffer.
func (r *mathFenceRewriter) step(line string) {
	t := strings.TrimSpace(line)
	switch r.mode {
	case fenceMath:
		r.stepInMath(line, t)
	case fenceCode:
		r.out = append(r.out, line)
		if isFenceLine(t) {
			r.mode = fenceText
		}
	default:
		r.stepInText(line, t)
	}
}

func (r *mathFenceRewriter) stepInMath(line, trimmed string) {
	if trimmed == "```" {
		r.out = append(r.out, "$$", "")
		r.mode = fenceText
		return
	}
	r.out = append(r.out, line)
}

func (r *mathFenceRewriter) stepInText(line, trimmed string) {
	if trimmed == "```math" {
		r.out = append(r.out, "", "$$")
		r.mode = fenceMath
		return
	}
	r.out = append(r.out, line)
	if isFenceLine(trimmed) {
		r.mode = fenceCode
	}
}

// mathFencesToDisplay rewrites top-level ```math fences into $$ display-math
// paragraphs. An unclosed math fence still gets a closing "$$" so KaTeX sees
// a balanced pair instead of swallowing the rest of the document.
func mathFencesToDisplay(s string) string {
	if !strings.Contains(s, "```math") {
		return s
	}
	lines := strings.Split(s, "\n")
	r := &mathFenceRewriter{out: make([]string, 0, len(lines)+4)}
	for _, line := range lines {
		r.step(line)
	}
	if r.mode == fenceMath {
		r.out = append(r.out, "$$")
	}
	return strings.Join(r.out, "\n")
}
