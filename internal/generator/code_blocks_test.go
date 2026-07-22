package generator

import (
	"strings"
	"testing"
)

// TestHighlightLineNumbers renders a code block with and without line numbers
// and checks the line-number markup appears only when enabled (GO-074). Chroma
// renders line numbers as a non-selectable inline-styled prefix span.
func TestHighlightLineNumbers(t *testing.T) {
	src := "```go\nfmt.Println(\"x\")\n```\n"

	off := &Generator{config: Config{Highlight: true, HighlightStyle: "github"}}
	off.md = buildMarkdown(off.config)
	if h := off.convertMarkdownToHTML(src); strings.Contains(h, "user-select:none") {
		t.Fatalf("line numbers must be off by default:\n%s", h)
	}

	on := &Generator{config: Config{Highlight: true, HighlightLineNumbers: true, HighlightStyle: "github"}}
	on.md = buildMarkdown(on.config)
	h := on.convertMarkdownToHTML(src)
	if !strings.Contains(h, "user-select:none") {
		t.Fatalf("line numbers should be rendered when enabled:\n%s", h)
	}
	// The code is still highlighted (inline colour styles present).
	if !strings.Contains(h, "color:") {
		t.Fatalf("highlighting missing:\n%s", h)
	}
}

// TestMermaidConversionEndToEnd confirms a ```mermaid fence becomes a raw
// <pre class="mermaid"> block through the full conversion, with the diagram
// source unescaped so mermaid.js can read it (GO-073).
func TestMermaidConversionEndToEnd(t *testing.T) {
	g := &Generator{config: Config{Mermaid: true}}
	g.md = buildMarkdown(g.config)
	html := g.convertMarkdownToHTML("```mermaid\ngraph TD\n  A --> B\n```\n")
	if !strings.Contains(html, `<pre class="mermaid">`) {
		t.Fatalf("mermaid container missing:\n%s", html)
	}
	// The arrow must be raw (-->), never HTML-escaped (--&gt;), or the diagram
	// fails to parse in the browser — the exact bug this fixes.
	if strings.Contains(html, "--&gt;") || !strings.Contains(html, "A --> B") {
		t.Fatalf("diagram source was escaped:\n%s", html)
	}
}

// TestMermaidDisabledStaysCodeBlock confirms that without mermaid: true a
// mermaid fence renders as an ordinary (escaped) code block.
func TestMermaidDisabledStaysCodeBlock(t *testing.T) {
	g := &Generator{config: Config{}}
	g.md = buildMarkdown(g.config)
	html := g.convertMarkdownToHTML("```mermaid\ngraph TD\n```\n")
	if strings.Contains(html, `<pre class="mermaid">`) {
		t.Fatalf("mermaid should be inert when disabled:\n%s", html)
	}
}
