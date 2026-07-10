package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckLinksIfRequested(t *testing.T) {
	g := newTestGen(t, "")
	// off → no-op
	if err := g.checkLinksIfRequested(); err != nil {
		t.Fatalf("off should be a no-op: %v", err)
	}
	out := g.config.OutputDir
	_ = os.WriteFile(filepath.Join(out, "index.html"), []byte(`<a href="/missing/">x</a>`), 0644)

	g.config.CheckLinks = "warn"
	if err := g.checkLinksIfRequested(); err != nil {
		t.Errorf("warn must not fail the build: %v", err)
	}
	g.config.CheckLinks = "strict"
	if err := g.checkLinksIfRequested(); err == nil {
		t.Error("strict must fail on a broken internal link")
	}
	// A clean tree passes even in strict mode.
	_ = os.WriteFile(filepath.Join(out, "index.html"), []byte(`<a href="#top">x</a>`), 0644)
	if err := g.checkLinksIfRequested(); err != nil {
		t.Errorf("clean strict should pass: %v", err)
	}
}

func TestBuildMarkdownHighlight(t *testing.T) {
	g := &Generator{config: Config{Highlight: true, HighlightStyle: "monokai"}}
	g.md = buildMarkdown(g.config)
	out := g.convertMarkdownToHTML("```go\nvar x = 1\n```")
	if !strings.Contains(out, "<pre") {
		t.Errorf("highlighting not applied: %s", out)
	}
	// empty style falls back to a default without panicking.
	g2 := &Generator{config: Config{Highlight: true}}
	g2.md = buildMarkdown(g2.config)
	if out := g2.convertMarkdownToHTML("```\nx\n```"); !strings.Contains(out, "<pre") {
		t.Errorf("default-style highlight failed: %s", out)
	}
}

func TestReplaceTOCMarker(t *testing.T) {
	g := &Generator{config: Config{TOCDepth: 3}}
	g.md = buildMarkdown(g.config)
	out := g.replaceTOCMarker("[toc]\n## Sec\ntext")
	if !strings.Contains(out, `class="toc"`) {
		t.Errorf("[toc] not expanded: %s", out)
	}
	if g.replaceTOCMarker("no marker here") != "no marker here" {
		t.Error("content without [toc] should be unchanged")
	}
}

func TestFingerprintIfRequestedEnabled(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Fingerprint = true
	_ = os.WriteFile(filepath.Join(g.config.OutputDir, "s.css"), []byte("a{color:red}"), 0644)
	if err := g.fingerprintIfRequested(); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "assets-manifest.json")); err != nil {
		t.Errorf("manifest not written: %v", err)
	}
}
