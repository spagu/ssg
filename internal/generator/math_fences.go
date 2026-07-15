// Fenced-math rewriting (GO-055): ```math blocks become display math ($$…$$)
// before markdown conversion, so detection (containsMath), KaTeX injection
// (mathHTMLString requires a literal "$$") and browser-side auto-render all
// agree. Fences nested inside other code blocks are left untouched, mirroring
// the parser's fence tracking (GO-027). Inline \(...\) stays unsupported —
// CommonMark backslash-escaping would eat the delimiters.
package generator

import "strings"

// mathFencesToDisplay rewrites top-level ```math fences into $$ display-math
// paragraphs. An unclosed math fence still gets a closing "$$" so KaTeX sees
// a balanced pair instead of swallowing the rest of the document.
func mathFencesToDisplay(s string) string {
	if !strings.Contains(s, "```math") {
		return s
	}
	const (
		modeText = iota
		modeCode
		modeMath
	)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines)+4)
	mode := modeText
	for _, line := range lines {
		t := strings.TrimSpace(line)
		isFence := strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
		switch mode {
		case modeMath:
			if t == "```" {
				out = append(out, "$$", "")
				mode = modeText
				continue
			}
			out = append(out, line)
		case modeCode:
			out = append(out, line)
			if isFence {
				mode = modeText
			}
		default: // modeText
			if t == "```math" {
				out = append(out, "", "$$")
				mode = modeMath
				continue
			}
			out = append(out, line)
			if isFence {
				mode = modeCode
			}
		}
	}
	if mode == modeMath {
		out = append(out, "$$")
	}
	return strings.Join(out, "\n")
}
