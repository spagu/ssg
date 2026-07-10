package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return path
}

// TestParseNoFrontmatter covers GO-009: a file without opening "---" is treated
// as published content instead of being silently discarded.
func TestParseNoFrontmatter(t *testing.T) {
	path := writeTemp(t, "plain.md", "Just some body text\nwith two lines")
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if page.Status != "publish" {
		t.Errorf("Status = %q, want publish", page.Status)
	}
	if page.Content != "Just some body text\nwith two lines" {
		t.Errorf("Content = %q", page.Content)
	}
}

// TestParseLeadingBlankThenFrontmatter ensures leading blank lines before "---"
// are still tolerated (no false no-frontmatter detection).
func TestParseLeadingBlankThenFrontmatter(t *testing.T) {
	path := writeTemp(t, "fm.md", "\n\n---\ntitle: Hi\nstatus: publish\n---\nBody")
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if page.Title != "Hi" {
		t.Errorf("Title = %q, want Hi", page.Title)
	}
	if page.Content != "Body" {
		t.Errorf("Content = %q, want Body", page.Content)
	}
}

// TestParseAliasesAndSeries covers SEO-002 aliases and AX-005 series frontmatter.
func TestParseAliasesAndSeries(t *testing.T) {
	fm := "---\ntitle: T\nstatus: publish\nseries: Learn Go\naliases:\n  - /old/\n  - /older/\n---\nBody"
	path := writeTemp(t, "a.md", fm)
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if page.Series != "Learn Go" {
		t.Errorf("Series = %q, want Learn Go", page.Series)
	}
	if len(page.Aliases) != 2 || page.Aliases[0] != "/old/" || page.Aliases[1] != "/older/" {
		t.Errorf("Aliases = %v", page.Aliases)
	}
	// aliases/series must not leak into Extra
	if _, ok := page.Extra["aliases"]; ok {
		t.Errorf("aliases leaked into Extra")
	}
	if _, ok := page.Extra["series"]; ok {
		t.Errorf("series leaked into Extra")
	}
}
