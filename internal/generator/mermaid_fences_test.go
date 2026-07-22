package generator

import (
	"strings"
	"testing"
)

func TestMermaidFencesToHTML_TopLevel(t *testing.T) {
	in := "intro\n\n```mermaid\ngraph TD\n  A --> B\n```\n\nafter\n"
	out := mermaidFencesToHTML(in)
	if !strings.Contains(out, `<pre class="mermaid">`) || !strings.Contains(out, "</pre>") {
		t.Fatalf("fence not converted:\n%s", out)
	}
	// The diagram source must survive verbatim — the arrow is not escaped here.
	if !strings.Contains(out, "A --> B") {
		t.Fatalf("diagram body altered:\n%s", out)
	}
	// The ```mermaid fence marker is gone.
	if strings.Contains(out, "```mermaid") {
		t.Fatalf("fence marker left behind:\n%s", out)
	}
}

func TestMermaidFencesToHTML_LeavesNestedFences(t *testing.T) {
	// A ```mermaid nested inside a ```markdown code block must be left untouched.
	in := "```markdown\n```mermaid\ngraph TD\n```\n```\n"
	out := mermaidFencesToHTML(in)
	if strings.Contains(out, `class="mermaid"`) {
		t.Fatalf("nested fence should not be rewritten:\n%s", out)
	}
}

func TestMermaidFencesToHTML_UnclosedIsClosed(t *testing.T) {
	in := "```mermaid\ngraph TD\n  A --> B\n"
	out := mermaidFencesToHTML(in)
	if strings.Count(out, "</pre>") != 1 {
		t.Fatalf("unclosed fence should still be closed once:\n%s", out)
	}
}

func TestMermaidFencesToHTML_NoMermaidPassthrough(t *testing.T) {
	in := "just text\n```go\nfmt.Println()\n```\n"
	if got := mermaidFencesToHTML(in); got != in {
		t.Fatalf("content without mermaid must pass through unchanged")
	}
}

func TestContainsMermaid(t *testing.T) {
	if !containsMermaid("x\n```mermaid\ngraph TD\n```\n") {
		t.Fatal("should detect a mermaid fence")
	}
	if containsMermaid("```go\ncode\n```") {
		t.Fatal("should not detect mermaid in a go fence")
	}
}

func TestMermaidHTMLString_InjectsOnce(t *testing.T) {
	page := `<html><head></head><body><pre class="mermaid">graph TD</pre></body></html>`
	out := mermaidHTMLString(page)
	if strings.Count(out, "mermaid@"+mermaidVersion) != 1 {
		t.Fatalf("runtime should be injected exactly once:\n%s", out)
	}
	// Idempotent: a second pass must not add it again.
	if mermaidHTMLString(out) != out {
		t.Fatal("second pass should be a no-op")
	}
}

func TestMermaidHTMLString_NoDiagramNoInject(t *testing.T) {
	page := `<html><body><p>no diagram</p></body></html>`
	if mermaidHTMLString(page) != page {
		t.Fatal("pages without a diagram must not load the runtime")
	}
}

func TestInjectMermaidAssets_BeforeBodyClose(t *testing.T) {
	out := injectMermaidAssets(`<body>x</body>`)
	if !strings.Contains(out, "</script>\n</body>") {
		t.Fatalf("runtime should sit just before </body>:\n%s", out)
	}
}
