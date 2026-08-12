package generator

// Gaps in the [ai …] shortcode plumbing (#1.8.16), the SSG_* env export and
// the hook runner's blank-command guard, plus the heading-id dedup for linked
// headings (issue #26).

import (
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/ai"
	"github.com/spagu/ssg/internal/models"
)

// TestResolveAIContentFallbacks: pages (not only posts) are resolved; content
// without a shortcode is untouched; an unparseable ifs and an unknown model
// both fall back rather than shipping the raw shortcode or failing the build.
func TestResolveAIContentFallbacks(t *testing.T) {
	g := newTestGen(t, "")
	g.config.AI = ai.New(map[string]ai.Model{"m": {URL: "http://127.0.0.1:1", Model: "x"}},
		nil, "m", "", t.TempDir(), 0)
	g.siteData.Pages = []models.Page{
		{Slug: "plain", Content: "no shortcode here"},
		{Slug: "badifs", Content: `[ai question="q?" ifs="not-a-condition" fallback="IFS-FB"]`},
		{Slug: "badmodel", Content: `[ai question="q?" model="missing" fallback="MODEL-FB"]`},
	}
	g.resolveAIContent()
	if got := g.siteData.Pages[0].Content; got != "no shortcode here" {
		t.Errorf("shortcode-free page changed: %q", got)
	}
	if got := g.siteData.Pages[1].Content; got != "IFS-FB" {
		t.Errorf("unparseable ifs must fall back, got %q", got)
	}
	if got := g.siteData.Pages[2].Content; got != "MODEL-FB" {
		t.Errorf("unknown model must fall back, got %q", got)
	}
}

// TestAIVarsPrecedence: page fields and frontmatter extras are visible to the
// ifs guard, and a site variable never shadows a page field of the same name.
func TestAIVarsPrecedence(t *testing.T) {
	page := models.Page{Lang: "en", Slug: "s", Tags: []string{"a", "b"},
		Extra: map[string]interface{}{"level": 3}}
	vars := aiVars(page, map[string]interface{}{"lang": "SITE", "owner": "acme"})
	if vars["lang"] != "en" {
		t.Errorf("page field must win over site variable, got %q", vars["lang"])
	}
	if vars["level"] != "3" || vars["owner"] != "acme" || vars["tags"] != "a,b" {
		t.Errorf("vars = %v", vars)
	}
}

// TestExportVariablesToEnvSanitizesKeys: variable names carrying separators
// become valid env identifiers, so `my-key` is reachable as SSGGAP_MY_KEY.
func TestExportVariablesToEnvSanitizesKeys(t *testing.T) {
	t.Setenv("SSGGAP_MY_KEY", "") // registers cleanup for the export below
	exportVariablesToEnv(map[string]interface{}{"my-key": "v1"}, "SSGGAP")
	if got := os.Getenv("SSGGAP_MY_KEY"); got != "v1" {
		t.Fatalf("SSGGAP_MY_KEY = %q, want v1", got)
	}
}

// TestRunHooksSkipsBlankCommands: whitespace-only hook lines are skipped, not
// executed as empty argv (which would abort every build using a YAML "").
func TestRunHooksSkipsBlankCommands(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Hooks = map[string][]string{"pre_build": {"   ", ""}}
	if err := g.runHooks("pre_build", nil); err != nil {
		t.Fatalf("blank hooks must be no-ops: %v", err)
	}
}

// TestHeadingIDDedupWithLinks: two identical linked headings get distinct ids
// — anchors must stay unique per document (issue #26).
func TestHeadingIDDedupWithLinks(t *testing.T) {
	g, err := New(Config{Domain: "example.com", Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	html := g.convertMarkdownToHTML("## [Ian](/authors/ian/)\n\ntext\n\n## [Ian](/authors/ian/)\n")
	if !strings.Contains(html, `id="ian"`) || !strings.Contains(html, `id="ian-1"`) {
		t.Fatalf("duplicate linked headings must dedupe ids, got:\n%s", html)
	}
}
