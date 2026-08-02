package generator

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/ai"
	"github.com/spagu/ssg/internal/models"
)

// TestResolveAIContent covers #1.8.16: [ai …] shortcodes are replaced by the
// (cached) answer; the ifs guard gates on page context and falls back when
// false; a query error falls back; no ai config is a no-op.
func TestResolveAIContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"AI SAYS HI"}}]}`)
	}))
	defer srv.Close()

	g := newTestGen(t, "")
	g.config.AI = ai.New(
		map[string]ai.Model{"fast": {URL: srv.URL, Model: "m1"}},
		map[string]ai.Agent{"writer": {Model: "fast", Rules: []string{"be terse"}}},
		"fast", "", t.TempDir(), 0)
	g.siteData.Posts = []models.Page{
		{ // ifs true → answer injected
			Slug: "a", Type: "post", Lang: "en", Status: "publish",
			Content: `Intro. [ai model="fast" question="hi?" ifs="lang == en" fallback="(none)"] Outro.`,
		},
		{ // ifs false → fallback
			Slug: "b", Type: "post", Lang: "pl",
			Content: `[ai model="fast" question="hi?" ifs="lang == en" fallback="BRAK"]`,
		},
		{ // no question → fallback
			Slug: "c", Type: "post",
			Content: `[ai model="fast" fallback="NOQ"]`,
		},
		{ // invoked by agent name
			Slug: "d", Type: "post",
			Content: `[ai agent="writer" question="hi?" fallback="(none)"]`,
		},
	}
	g.resolveAIContent()

	if got := g.siteData.Posts[0].Content; !strings.Contains(got, "AI SAYS HI") || strings.Contains(got, "(none)") {
		t.Errorf("ifs-true post: %q", got)
	}
	if got := g.siteData.Posts[1].Content; got != "BRAK" {
		t.Errorf("ifs-false post = %q, want fallback BRAK", got)
	}
	if got := g.siteData.Posts[2].Content; got != "NOQ" {
		t.Errorf("no-question post = %q, want fallback NOQ", got)
	}
	if got := g.siteData.Posts[3].Content; !strings.Contains(got, "AI SAYS HI") {
		t.Errorf("agent-invoked post: %q", got)
	}
}

// TestResolveAINoConfig: without ai configured, content is untouched (no panic).
func TestResolveAINoConfig(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Posts = []models.Page{{Slug: "a", Content: `x [ai question="q" fallback="f"] y`}}
	g.resolveAIContent()
	if g.siteData.Posts[0].Content != `x [ai question="q" fallback="f"] y` {
		t.Errorf("no-ai-config must leave content untouched: %q", g.siteData.Posts[0].Content)
	}
}
