package generator

import (
	"strings"
	"testing"
)

// TestTransformHTMLPageMermaid: the page pass runs the Mermaid step when on.
func TestTransformHTMLPageMermaid(t *testing.T) {
	g := mermaidPageGen(t)
	out := g.transformHTMLPage("<html><head></head><body><pre class=\"mermaid\">graph TD; A-->B</pre></body></html>", nil, false)
	if !strings.Contains(out, "mermaid") {
		t.Errorf("mermaid page: %s", out)
	}
}

func mermaidPageGen(t *testing.T) *Generator {
	t.Helper()
	g, err := New(Config{Domain: "example.com", Mermaid: true})
	if err != nil {
		t.Fatal(err)
	}
	return g
}
