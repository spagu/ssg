// Package parser - tests for the GO-003 sitemap frontmatter propagation.
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseMarkdownSitemapField verifies GO-003: a file-based page carries its
// `sitemap:` frontmatter through to models.Page (previously only mddb set it),
// so `sitemap: no` can exclude the page from sitemap.xml.
func TestParseMarkdownSitemapField(t *testing.T) {
	tmpDir := t.TempDir()

	cases := map[string]string{
		"no.md":     "no",
		"yes.md":    "yes",
		"absent.md": "", // no sitemap field at all
	}
	bodies := map[string]string{
		"no.md":     "---\ntitle: No\nsitemap: no\n---\n\n## Content\n\nbody\n",
		"yes.md":    "---\ntitle: Yes\nsitemap: yes\n---\n\n## Content\n\nbody\n",
		"absent.md": "---\ntitle: Absent\n---\n\n## Content\n\nbody\n",
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte(bodies[name]), 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
			page, err := ParseMarkdownFile(path)
			if err != nil {
				t.Fatalf("ParseMarkdownFile: %v", err)
			}
			if page.Sitemap != want {
				t.Errorf("page.Sitemap = %q, want %q", page.Sitemap, want)
			}
			// "sitemap" must be a known field, never leaking into Extra.
			if _, leaked := page.Extra["sitemap"]; leaked {
				t.Errorf("sitemap leaked into Extra; it must be a known field")
			}
		})
	}
}
