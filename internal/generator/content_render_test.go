package generator

import (
	"html/template"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// bareRenderGen returns a minimal Generator with the content pipeline wired, as
// buildTemplateFuncs does in a real build.
func bareRenderGen() *Generator {
	g := &Generator{
		config:   Config{},
		siteData: &models.SiteData{Media: map[int]models.MediaItem{}},
	}
	g.md = buildMarkdown(g.config)
	g.renderContentFn = g.tmplSafeHTML(map[string]string{}, map[string]map[string]string{})
	return g
}

// TestContentContextValue_Renders is the #118 fix: root .Content is the rendered
// HTML, so a theme printing {{ .Content }} gets markup, not raw Markdown.
func TestContentContextValue_Renders(t *testing.T) {
	g := bareRenderGen()
	v := g.contentContextValue("## Sub\n\n**bold**")
	h, ok := v.(template.HTML)
	if !ok {
		t.Fatalf("root .Content should be template.HTML, got %T", v)
	}
	if !strings.Contains(string(h), "<h2") || !strings.Contains(string(h), "<strong>") {
		t.Fatalf("root .Content not rendered: %q", h)
	}
}

// TestSafeHTMLValue_AcceptsStringAndHTML is the #118 fix: safeHTML renders a raw
// string and passes an already-rendered template.HTML through, so both
// {{ .Post.Content | safeHTML }} and {{ .Content | safeHTML }} work.
func TestSafeHTMLValue_AcceptsStringAndHTML(t *testing.T) {
	g := bareRenderGen()
	if got := g.safeHTMLValue("# Hi"); !strings.Contains(string(got), "<h1") {
		t.Fatalf("string should be rendered: %q", got)
	}
	if got := g.safeHTMLValue(template.HTML("<b>x</b>")); string(got) != "<b>x</b>" {
		t.Fatalf("template.HTML should pass through: %q", got)
	}
	// An unknown type is escaped, never trusted as markup.
	if got := g.safeHTMLValue(7); strings.Contains(string(got), "<") {
		t.Fatalf("unexpected markup from int: %q", got)
	}
}

// TestContentContextValue_SanitizerFallback keeps SEC-014 in the bare fallback:
// before the funcs are wired, a sanitizer build hands back a plain (auto-escaped)
// string rather than raw template.HTML.
func TestContentContextValue_SanitizerFallback(t *testing.T) {
	g := &Generator{config: Config{}}
	if _, ok := g.contentContextValue("x").(template.HTML); !ok {
		t.Errorf("no sanitizer, no funcs → template.HTML")
	}
	g.sanitizer = newSanitizer(true)
	if _, ok := g.contentContextValue("x").(string); !ok {
		t.Errorf("sanitizer + no funcs → plain string (SEC-014)")
	}
}
