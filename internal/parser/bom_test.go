package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A UTF-8 byte order mark before the opening "---" must not hide the
// frontmatter: the title comes from it and the YAML never reaches the body.
func TestParseMarkdownFileWithByteOrderMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bom.md")
	src := "\ufeff---\ntitle: \"From frontmatter\"\nslug: \"bom\"\n---\n\n# From heading\n\nBody text.\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "From frontmatter" {
		t.Errorf("title = %q, want the frontmatter's", page.Title)
	}
	if strings.Contains(page.Content, "title:") || strings.Contains(page.Content, "\ufeff") {
		t.Errorf("frontmatter or BOM leaked into the body: %q", page.Content)
	}
}

// Only a mark at the very start is removed; one later in the text is content.
func TestParseMarkdownFileKeepsALaterByteOrderMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "later.md")
	src := "---\ntitle: \"T\"\n---\n\nbefore\ufeffafter\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Content, "before\ufeffafter") {
		t.Errorf("a mark inside the text was removed: %q", page.Content)
	}
}
