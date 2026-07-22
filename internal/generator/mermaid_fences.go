// Mermaid fence rewriting (GO-073): top-level ```mermaid blocks become a raw
// <pre class="mermaid"> HTML block before markdown conversion, so goldmark
// passes the diagram source through verbatim (an HTML block is not escaped or
// re-parsed) and the mermaid.js runtime — injected only on pages that use one —
// turns it into a diagram in the browser. Fences nested inside other code
// blocks are left untouched, mirroring mathFencesToDisplay (GO-055).
package generator

import "strings"

// mermaidFenceRewriter turns top-level ```mermaid fences into
// <pre class="mermaid">…</pre> while leaving fences nested in other code blocks
// alone. Same three-state machine as mathFenceRewriter.
type mermaidFenceRewriter struct {
	mode int
	out  []string
}

func (r *mermaidFenceRewriter) step(line string) {
	t := strings.TrimSpace(line)
	switch r.mode {
	case fenceMath: // reused constant: "inside the block being rewritten"
		r.stepInMermaid(line, t)
	case fenceCode:
		r.out = append(r.out, line)
		if isFenceLine(t) {
			r.mode = fenceText
		}
	default:
		r.stepInText(line, t)
	}
}

func (r *mermaidFenceRewriter) stepInMermaid(line, trimmed string) {
	if trimmed == "```" {
		r.out = append(r.out, "</pre>", "")
		r.mode = fenceText
		return
	}
	r.out = append(r.out, line)
}

func (r *mermaidFenceRewriter) stepInText(line, trimmed string) {
	if trimmed == "```mermaid" {
		// A blank line before the HTML block keeps it a distinct block; the
		// opening <pre> starts a CommonMark type-1 HTML block that runs
		// verbatim (through blank lines) until the closing </pre>.
		r.out = append(r.out, "", `<pre class="mermaid">`)
		r.mode = fenceMath
		return
	}
	r.out = append(r.out, line)
	if isFenceLine(trimmed) {
		r.mode = fenceCode
	}
}

// mermaidFencesToHTML rewrites top-level ```mermaid fences into
// <pre class="mermaid"> blocks. An unclosed fence is still closed so the rest
// of the document is not swallowed into the diagram.
func mermaidFencesToHTML(s string) string {
	if !strings.Contains(s, "```mermaid") {
		return s
	}
	lines := strings.Split(s, "\n")
	r := &mermaidFenceRewriter{out: make([]string, 0, len(lines)+4)}
	for _, line := range lines {
		r.step(line)
	}
	if r.mode == fenceMath {
		r.out = append(r.out, "</pre>")
	}
	return strings.Join(r.out, "\n")
}

// containsMermaid reports whether raw content carries a mermaid fence.
func containsMermaid(content string) bool {
	return strings.Contains(content, "```mermaid")
}
