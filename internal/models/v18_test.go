package models

import (
	"strings"
	"testing"
	"time"
)

// TestComputeReadingStats covers BLOG-006 word count / reading time.
func TestComputeReadingStats(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantWords int
		wantMins  int
	}{
		{"empty", "", 0, 0},
		{"few words", "one two three", 3, 1},
		{"strips html", "<p>one two</p> <b>three</b>", 3, 1},
		{"strips shortcodes", "one {{banner}} two [toc] three", 3, 1},
		{"rounds up", strings.Repeat("word ", 250), 250, 2},
		{"exact boundary", strings.Repeat("word ", 200), 200, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Page{Content: tt.content}
			p.ComputeReadingStats()
			if p.WordCount != tt.wantWords {
				t.Errorf("WordCount = %d, want %d", p.WordCount, tt.wantWords)
			}
			if p.ReadingTime != tt.wantMins {
				t.Errorf("ReadingTime = %d, want %d", p.ReadingTime, tt.wantMins)
			}
		})
	}
}

// TestPermalinkPathOverridesURL covers SEO-001 at the model layer.
func TestPermalinkPathOverridesURL(t *testing.T) {
	p := Page{
		Type:          "post",
		Slug:          "hello",
		Date:          time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		PermalinkPath: "2026/07/hello",
	}
	if got := p.GetURL(); got != "/2026/07/hello/" {
		t.Errorf("GetURL = %q, want /2026/07/hello/", got)
	}
	if got := p.GetOutputPath(); got != "2026/07/hello" {
		t.Errorf("GetOutputPath = %q, want 2026/07/hello", got)
	}
}

// TestLinkBeatsPermalink verifies the frontmatter Link keeps priority over a
// configured permalink (SEO-001).
func TestLinkBeatsPermalink(t *testing.T) {
	p := Page{Link: "/custom/", PermalinkPath: "ignored/path", Slug: "s"}
	if got := p.GetOutputPath(); got != "custom" {
		t.Errorf("GetOutputPath = %q, want custom", got)
	}
}

// TestLangPrefix covers PLAT-005 language-prefixed URLs/paths.
func TestLangPrefix(t *testing.T) {
	p := Page{Type: "page", Slug: "about", LangPrefix: "en"}
	if got := p.GetURL(); got != "/en/about/" {
		t.Errorf("GetURL = %q, want /en/about/", got)
	}
	if got := p.GetOutputPath(); got != "en/about" {
		t.Errorf("GetOutputPath = %q, want en/about", got)
	}
	// Explicit Link is never language-prefixed.
	pl := Page{Link: "/x/", LangPrefix: "en"}
	if got := pl.GetOutputPath(); got != "x" {
		t.Errorf("GetOutputPath with Link = %q, want x", got)
	}
}

// TestLangPrefixWithPermalink combines PLAT-005 + SEO-001.
func TestLangPrefixWithPermalink(t *testing.T) {
	p := Page{Type: "post", Slug: "hi", PermalinkPath: "blog/hi", LangPrefix: "pl"}
	if got := p.GetURL(); got != "/pl/blog/hi/" {
		t.Errorf("GetURL = %q, want /pl/blog/hi/", got)
	}
}
