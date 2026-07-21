package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GO-057: a plain Markdown file (no frontmatter) had neither a title nor an
// excerpt, so it appeared untitled and blank in every listing, menu, feed and
// <title> element. The title now falls back to the document's own heading;
// the excerpt is derived only when auto_excerpt asks for it.

func parseString(t *testing.T, body string) (title, excerpt string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatalf("ParseMarkdownFile: %v", err)
	}
	return page.Title, page.Excerpt
}

func TestTitleFallsBackToHeading(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"atx heading", "# Content guide\n\nBody text.\n", "Content guide"},
		{"closed atx heading", "# Content guide #\n\nBody.\n", "Content guide"},
		{"first heading wins", "# First\n\ntext\n\n# Second\n", "First"},
		{"setext heading", "Content guide\n=============\n\nBody.\n", "Content guide"},
		{"frontmatter title still wins", "---\ntitle: From frontmatter\nstatus: publish\n---\n\n# Ignored\n", "From frontmatter"},
		{"no heading at all", "Just a paragraph.\n", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := parseString(t, tc.body)
			if got != tc.want {
				t.Errorf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

// The excerpt must stay empty without auto_excerpt — that is the behaviour
// existing sites' meta descriptions and feeds depend on.
func TestExcerptStaysEmptyByDefault(t *testing.T) {
	if _, excerpt := parseString(t, "# Title\n\nOpening paragraph.\n"); excerpt != "" {
		t.Errorf("excerpt = %q, want empty (derivation is opt-in)", excerpt)
	}
}

func TestDeriveExcerpt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"first paragraph only",
			"First paragraph here.\n\nSecond paragraph.\n",
			"First paragraph here.",
		},
		{
			"headings are skipped",
			"## Section\n\nReal prose starts here.\n",
			"Real prose starts here.",
		},
		{
			"fenced code is not prose",
			"```bash\nmake install\n```\n\nProse after the block.\n",
			"Prose after the block.",
		},
		{
			"liquid guards are skipped",
			"{% raw %}\nOne unified system feeds templates.\n",
			"One unified system feeds templates.",
		},
		{
			"lists, quotes, tables and images are skipped",
			"![badge](x.png)\n> quote\n| a | b |\n- item\n\nThe actual sentence.\n",
			"The actual sentence.",
		},
		{
			"inline markdown is stripped",
			"See the [configuration guide](CONFIGURATION.md) for `--flag` and **bold**.\n",
			"See the configuration guide for --flag and bold.",
		},
		{"nothing to derive", "```\ncode only\n```\n", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveExcerpt(tc.content); got != tc.want {
				t.Errorf("DeriveExcerpt() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeriveExcerptTruncates(t *testing.T) {
	long := strings.Repeat("word ", 100)
	got := DeriveExcerpt(long)
	if len([]rune(got)) > ExcerptMaxRunes+1 { // +1 for the ellipsis
		t.Errorf("derived excerpt is %d runes, want at most %d", len([]rune(got)), ExcerptMaxRunes+1)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated excerpt %q does not end with an ellipsis", got)
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Error("excerpt was cut mid-space")
	}
}
