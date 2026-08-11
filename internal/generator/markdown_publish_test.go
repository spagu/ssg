package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestPageMarkdown_PrependsTitle(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true}}
	p := models.Page{Title: "Configuration", Content: "Body text here."}
	md := g.pageMarkdown(p)
	if !strings.HasPrefix(md, "# Configuration\n\nBody text here.") {
		t.Fatalf("unexpected markdown:\n%s", md)
	}
}

func TestPageMarkdown_KeepsExistingH1(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true}}
	p := models.Page{Title: "Config", Content: "# Real Heading\n\nBody."}
	md := g.pageMarkdown(p)
	if strings.Count(md, "# ") != 1 || !strings.HasPrefix(md, "# Real Heading") {
		t.Fatalf("should not double the H1:\n%s", md)
	}
}

func TestPageMarkdown_CleanChars(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true, CleanSpecialChars: true}}
	p := models.Page{Title: "T", Content: "“smart” — dash"}
	md := g.pageMarkdown(p)
	if strings.ContainsAny(md, "“”—") {
		t.Fatalf("special chars not cleaned:\n%s", md)
	}
}

func TestStartsWithH1(t *testing.T) {
	if !startsWithH1("\n\n# Title\ntext") {
		t.Fatal("should detect a leading H1 after blank lines")
	}
	if startsWithH1("## Sub") || startsWithH1("text") || startsWithH1("#notheading") {
		t.Fatal("false positive H1")
	}
}

func TestInjectMarkdownAlternate(t *testing.T) {
	out := injectMarkdownAlternate(`<head><title>x</title></head><body></body>`)
	if !strings.Contains(out, `<link rel="alternate" type="text/markdown" href="index.md">`) {
		t.Fatalf("alternate not injected: %s", out)
	}
	// Idempotent.
	if injectMarkdownAlternate(out) != out {
		t.Fatal("second injection should be a no-op")
	}
}

func TestEffectiveHomeLimit(t *testing.T) {
	cases := []struct{ cfg, total, want int }{
		{0, 20, 6},   // default 6
		{0, 3, 3},    // fewer than 6 → all
		{-1, 20, 20}, // negative → no limit
		{4, 20, 4},   // explicit
		{50, 20, 20}, // capped at total
	}
	for _, c := range cases {
		if got := effectiveHomeLimit(c.cfg, c.total); got != c.want {
			t.Errorf("effectiveHomeLimit(%d,%d) = %d, want %d", c.cfg, c.total, got, c.want)
		}
	}
}

func TestMarkdownURLFor(t *testing.T) {
	p := models.Page{Link: "/configuration/"}
	if got := markdownURLFor(p, "example.com"); got != "https://example.com/configuration/index.md" {
		t.Fatalf("md url = %q", got)
	}
	// A non-directory URL (flat page) has no index.md counterpart.
	flat := models.Page{Link: "/configuration.html"}
	if got := markdownURLFor(flat, "example.com"); got != "" {
		t.Fatalf("flat page should have no md url, got %q", got)
	}
}
