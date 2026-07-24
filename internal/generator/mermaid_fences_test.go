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
	g := &Generator{}
	page := `<html><head></head><body><pre class="mermaid">graph TD</pre></body></html>`
	out := g.mermaidHTMLString(page)
	if strings.Count(out, "mermaid@"+mermaidVersion) != 1 {
		t.Fatalf("runtime should be injected exactly once:\n%s", out)
	}
	// Idempotent: a second pass must not add it again.
	if g.mermaidHTMLString(out) != out {
		t.Fatal("second pass should be a no-op")
	}
}

func TestMermaidHTMLString_NoDiagramNoInject(t *testing.T) {
	g := &Generator{}
	page := `<html><body><p>no diagram</p></body></html>`
	if g.mermaidHTMLString(page) != page {
		t.Fatal("pages without a diagram must not load the runtime")
	}
}

func TestInjectMermaidAssets_BeforeBodyClose(t *testing.T) {
	out := injectMermaidAssets(`<body>x</body>`, "", "")
	if !strings.Contains(out, "</script>\n</body>") {
		t.Fatalf("runtime should sit just before </body>:\n%s", out)
	}
	if strings.Contains(out, "theme:") || strings.Contains(out, "<style>") {
		t.Fatalf("no theme/background configured, none should be emitted:\n%s", out)
	}
}

func TestInjectMermaidAssets_ThemeAndBackground(t *testing.T) {
	g := &Generator{config: Config{MermaidTheme: "neutral", MermaidBackground: "#fff"}}
	out := g.mermaidHTMLString(`<body><pre class="mermaid">graph TD</pre></body>`)
	if !strings.Contains(out, `theme:"neutral"`) {
		t.Fatalf("configured theme should reach initialize():\n%s", out)
	}
	if !strings.Contains(out, `pre.mermaid{background:#fff`) {
		t.Fatalf("configured background should box the diagram:\n%s", out)
	}
	// The <style> must precede the runtime <script>.
	if strings.Index(out, "<style>") > strings.Index(out, "<script") {
		t.Fatalf("style should sit before the runtime script:\n%s", out)
	}
}

func TestInjectMermaidAssets_ThemeEscaped(t *testing.T) {
	// A hostile theme value must not break out of the inline <script>.
	out := injectMermaidAssets(`<body>x</body>`, `</script><script>alert(1)`, "")
	if strings.Contains(out, "</script><script>alert(1)") {
		t.Fatalf("theme must be escaped, not injected raw:\n%s", out)
	}
}
