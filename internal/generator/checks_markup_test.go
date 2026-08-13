package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// builderBody is a page-builder export: the blank line ends the HTML block and
// the tab-indented closing tags become an indented code block.
const builderBody = "<div class=\"widget\">\n\tcopy\n\n\t\t</div>\n\t</section>\n"

func TestCheckMarkupReportsSourceFileAndLine(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{
		{SourceFile: "logistics.md", Content: builderBody},
		{SourceFile: "clean.md", Content: "Just prose.\n"},
	}
	g.siteData.Posts = []models.Page{{SourceFile: "post.md", Content: builderBody}}
	g.config.CheckMarkup = "warn"

	out, err := capture(t, g.checkMarkupIfRequested)
	if err != nil {
		t.Fatalf("warn must not fail the build: %v", err)
	}
	for _, want := range []string{"logistics.md", "post.md", "ssg repair --fix"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the report:\n%s", want, out)
		}
	}
	if strings.Contains(out, "clean.md") {
		t.Errorf("a clean page must not be reported:\n%s", out)
	}
}

func TestCheckMarkupSilentWhenClean(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{{SourceFile: "clean.md", Content: "Prose only.\n"}}
	g.config.CheckMarkup = "warn"

	// It runs on every build, so a clean site must not pay a line of output.
	if out, err := capture(t, g.checkMarkupIfRequested); err != nil || out != "" {
		t.Errorf("clean site should be silent: %q, %v", out, err)
	}
}

func TestCheckMarkupModes(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{{SourceFile: "broken.md", Content: builderBody}}

	g.config.CheckMarkup = "strict"
	if _, err := capture(t, g.checkMarkupIfRequested); err == nil {
		t.Error("strict must fail the build")
	}

	// The global strict flag escalates it like every other check (#62).
	g.config.CheckMarkup, g.config.Strict = "warn", true
	if _, err := capture(t, g.checkMarkupIfRequested); err == nil {
		t.Error("global strict must escalate the check")
	}
	g.config.Strict = false

	for _, off := range []string{"", "off"} {
		g.config.CheckMarkup = off
		if out, err := capture(t, g.checkMarkupIfRequested); err != nil || out != "" {
			t.Errorf("check_markup=%q must be silent: %q, %v", off, out, err)
		}
	}
}

func TestCheckMarkupWithoutSiteData(t *testing.T) {
	g := &Generator{config: Config{CheckMarkup: "warn"}}
	if out, err := capture(t, g.checkMarkupIfRequested); err != nil || out != "" {
		t.Errorf("no site data should be a no-op: %q, %v", out, err)
	}
}

// TestMarkupSourceLabel: the report names something the author can open, and
// content without a file behind it (mddb, external sources) falls back to its
// URL and then its slug.
func TestMarkupSourceLabel(t *testing.T) {
	cases := []struct {
		page models.Page
		want string
	}{
		{models.Page{SourceFile: "a.md", Link: "/a/", Slug: "a"}, "a.md"},
		{models.Page{Link: "/a/", Slug: "a"}, "/a/"},
		{models.Page{Slug: "a"}, "a"},
	}
	for _, c := range cases {
		if got := markupSourceLabel(c.page); got != c.want {
			t.Errorf("markupSourceLabel(%+v) = %q, want %q", c.page, got, c.want)
		}
	}
}
