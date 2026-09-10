package generator

// One page, several representations (GO-092).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// outputSite builds a site with a page and a post, and whatever output
// configuration the test wants.
func outputSite(t *testing.T, apply func(cfg *Config)) Config {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md": "---\ntitle: About us\nslug: about\nstatus: publish\ntype: page\n---\n\n" +
			"We build **things**. Read [more](/x/).\n",
		"posts/news/hello.md": "---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\ndate: 2024-01-02\n---\n\nA post.\n",
	}, nil)
	if apply != nil {
		apply(&cfg)
	}
	buildSiteFixture(t, cfg)
	return cfg
}

// TestOutputsPerContentType: the thing a flat list could never say.
func TestOutputsPerContentType(t *testing.T) {
	cfg := outputSite(t, func(cfg *Config) {
		cfg.OutputsPerType = map[string][]string{
			"page": {"html", "json", "txt"},
			"post": {"html", "markdown"},
		}
	})
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(cfg.OutputDir, filepath.FromSlash(rel)))
		return err == nil
	}
	for _, rel := range []string{"about/index.json", "about/index.txt", "2024/01/02/hello/index.md"} {
		if !exists(rel) {
			t.Errorf("%s was not written", rel)
		}
	}
	for _, rel := range []string{"about/index.md", "2024/01/02/hello/index.json", "2024/01/02/hello/index.txt"} {
		if exists(rel) {
			t.Errorf("%s should not have been written", rel)
		}
	}
	// The Markdown output keeps the flat sibling GO-085 established.
	if !exists("2024/01/02/hello.md") {
		t.Error("the flat Markdown sibling is missing")
	}
}

// TestFlatListStillMeansEveryType: the form every existing config writes.
func TestFlatListStillMeansEveryType(t *testing.T) {
	cfg := outputSite(t, func(cfg *Config) { cfg.Outputs = []string{"html", "json"} })
	for _, rel := range []string{"about/index.json", "2024/01/02/hello/index.json"} {
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
}

// TestMarkdownPublishIsAnAlias: the switch that predates the registry keeps
// working, including its llms.txt and its <head> link.
func TestMarkdownPublishIsAnAlias(t *testing.T) {
	cfg := outputSite(t, func(cfg *Config) { cfg.MarkdownPublish = true })
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about", "index.md")); err != nil {
		t.Errorf("index.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about.md")); err != nil {
		t.Errorf("the flat sibling: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "llms.txt")); err != nil {
		t.Errorf("llms.txt: %v", err)
	}
	got := mustRead(t, filepath.Join(cfg.OutputDir, "about", "index.html"))
	if strings.Count(got, `type="text/markdown"`) != 1 {
		t.Errorf("exactly one markdown alternate, got:\n%s", got)
	}
}

// TestPageOutputsOverrideTheType (GO-096 meeting GO-092).
func TestPageOutputsOverrideTheType(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
		"pages/api.md":   "---\ntitle: API\nslug: api\nstatus: publish\ntype: page\noutputs: [html, txt]\n---\n\nAPI.\n",
	}, nil)
	cfg.OutputsPerType = map[string][]string{"page": {"html", "json"}}
	buildSiteFixture(t, cfg)
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about", "index.json")); err != nil {
		t.Errorf("the type's list should apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "api", "index.txt")); err != nil {
		t.Errorf("the page's own list should win: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "api", "index.json")); err == nil {
		t.Error("the page opted out of json")
	}
}

// TestTextOutputIsProse: title, then the body with the markup taken out.
func TestTextOutputIsProse(t *testing.T) {
	cfg := outputSite(t, func(cfg *Config) { cfg.Outputs = []string{"html", "txt"} })
	got := mustRead(t, filepath.Join(cfg.OutputDir, "about", "index.txt"))
	if !strings.HasPrefix(got, "About us\n\n") {
		t.Errorf("the title should lead:\n%q", got)
	}
	if strings.Contains(got, "<") || strings.Contains(got, "**") {
		t.Errorf("markup survived:\n%q", got)
	}
	if !strings.Contains(got, "We build things") {
		t.Errorf("the prose is missing:\n%q", got)
	}
}

// TestCustomFormatWritesWhatItsTemplateSays, without HTML escaping mangling it.
func TestCustomFormatWritesWhatItsTemplateSays(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "onix.xml")
	mustWrite(t, tmpl, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<record><title>{{ xmlEscape .Page.Title }}</title><url>https://{{ .Domain }}{{ .Page.GetURL }}</url></record>`)
	cfg := outputSite(t, func(cfg *Config) {
		cfg.Outputs = []string{"html", "onix"}
		cfg.OutputsCustom = []CustomOutput{{
			Name: "onix", Suffix: "index.xml", MIME: "application/xml", Template: tmpl,
		}}
	})
	got := mustRead(t, filepath.Join(cfg.OutputDir, "about", "index.xml"))
	if !strings.HasPrefix(got, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("the XML declaration was escaped:\n%s", got)
	}
	if !strings.Contains(got, "<title>About us</title>") || !strings.Contains(got, "https://example.com/about/") {
		t.Errorf("custom output:\n%s", got)
	}
	// It is announced in the page's head.
	page := mustRead(t, filepath.Join(cfg.OutputDir, "about", "index.html"))
	if !strings.Contains(page, `<link rel="alternate" type="application/xml" href="/about/index.xml">`) {
		t.Errorf("no alternate for the custom format:\n%s", page)
	}
}

// TestXMLEscapeHelper: the one helper a custom format is most likely to need.
func TestXMLEscapeHelper(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "f.xml")
	mustWrite(t, tmpl, `<t>{{ xmlEscape .Page.Title }}</t>`)
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: \"Tom & Jerry <b>\"\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
	}, nil)
	cfg.Outputs = []string{"html", "f"}
	cfg.OutputsCustom = []CustomOutput{{Name: "f", Suffix: "index.xml", Template: tmpl}}
	buildSiteFixture(t, cfg)
	got := mustRead(t, filepath.Join(cfg.OutputDir, "a", "index.xml"))
	if strings.Contains(got, "<b>") || !strings.Contains(got, "&amp;") {
		t.Errorf("xmlEscape did not escape:\n%s", got)
	}
}

// TestOutputConfigErrors: a format named and not defined, and a custom format
// that cannot be built, both fail at load.
func TestOutputConfigErrors(t *testing.T) {
	base := func(apply func(cfg *Config)) error {
		cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
			"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
		}, nil)
		apply(&cfg)
		return mustBuild(t, cfg)
	}
	if err := base(func(cfg *Config) { cfg.Outputs = []string{"html", "pdf"} }); err == nil ||
		!strings.Contains(err.Error(), "pdf") {
		t.Errorf("an unknown format: %v", err)
	}
	if err := base(func(cfg *Config) {
		cfg.OutputsPerType = map[string][]string{"post": {"html", "epub"}}
	}); err == nil || !strings.Contains(err.Error(), "outputs.post") {
		t.Errorf("an unknown format per type should name the type: %v", err)
	}
	if err := base(func(cfg *Config) {
		cfg.OutputsCustom = []CustomOutput{{Name: "json", Template: "x"}}
	}); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Errorf("shadowing a built-in: %v", err)
	}
	if err := base(func(cfg *Config) {
		cfg.OutputsCustom = []CustomOutput{{Name: "x"}}
	}); err == nil || !strings.Contains(err.Error(), "template is required") {
		t.Errorf("a format with no template: %v", err)
	}
	if err := base(func(cfg *Config) {
		cfg.OutputsCustom = []CustomOutput{{Name: "", Template: "x"}}
	}); err == nil || !strings.Contains(err.Error(), "name of its own") {
		t.Errorf("a nameless format: %v", err)
	}
	if err := base(func(cfg *Config) {
		cfg.OutputsCustom = []CustomOutput{{Name: "x", Template: filepath.Join(t.TempDir(), "gone.tmpl")}}
	}); err == nil || !strings.Contains(err.Error(), "outputs_custom.x") {
		t.Errorf("a missing template: %v", err)
	}
	broken := filepath.Join(t.TempDir(), "broken.tmpl")
	mustWrite(t, broken, "{{ .Page.Title")
	if err := base(func(cfg *Config) {
		cfg.OutputsCustom = []CustomOutput{{Name: "x", Template: broken}}
	}); err == nil || !strings.Contains(err.Error(), "outputs_custom.x") {
		t.Errorf("an unparsable template: %v", err)
	}
}

// TestOutputURLsAndPaths: how a representation is addressed and where it lands,
// for both URL shapes.
func TestOutputURLsAndPaths(t *testing.T) {
	json := builtinFormats()[FormatJSON]
	// Link is the page's own URL when it carries one, which is how a fixture
	// says "this page is served here".
	dir := models.Page{Link: "/about/"}
	flat := models.Page{Link: "/about.html"}
	if got := outputURLFor(dir, json); got != "/about/index.json" {
		t.Errorf("directory URL = %q", got)
	}
	if got := outputURLFor(flat, json); got != "/about.json" {
		t.Errorf("flat URL = %q", got)
	}
	if got := outputFilePath("/out/about/index.html", json); got != "/out/about/index.json" {
		t.Errorf("directory path = %q", got)
	}
	if got := outputFilePath("/out/about.html", json); got != "/out/about.json" {
		t.Errorf("flat path = %q", got)
	}
	// A page with no slug is the site root, and its representation lives at the
	// root — not behind a doubled slash.
	if got := outputURLFor(models.Page{Link: ""}, json); got != "/index.json" {
		t.Errorf("the root's representation = %q", got)
	}
}

// TestCollapseBlankLines: stripping tags leaves runs of empty lines behind.
func TestCollapseBlankLines(t *testing.T) {
	got := collapseBlankLines("a\n\n\n\nb   \n\n\nc")
	if got != "a\n\nb\n\nc" {
		t.Errorf("got %q", got)
	}
}

// TestNoOutputsWritesOnlyHTML, which is what every site that configured
// nothing has always got.
func TestNoOutputsWritesOnlyHTML(t *testing.T) {
	cfg := outputSite(t, nil)
	entries, err := os.ReadDir(filepath.Join(cfg.OutputDir, "about"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "index.html" {
			t.Errorf("unexpected representation: %s", e.Name())
		}
	}
}

// TestOutputWriteFailuresAreReported rather than leaving a page announcing a
// representation nobody wrote.
func TestOutputWriteFailuresAreReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	cfg := outputSite(t, func(cfg *Config) { cfg.Outputs = []string{"html", "json"} })
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.loadOutputs(nil); err != nil {
		t.Fatal(err)
	}
	locked := t.TempDir()
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	page := models.Page{Title: "T", Slug: "s", Type: "page"}
	// A write that cannot happen warns once and moves on.
	gen.writePageOutputs(page, filepath.Join(locked, "s", "index.html"))
	gen.writePageOutputs(page, filepath.Join(locked, "s", "index.html"))

	// A path that escapes the output root is refused before anything is
	// written, whatever the filesystem would have allowed.
	gen.config.OutputDir = t.TempDir()
	gen.writePageOutputs(page, filepath.Join(t.TempDir(), "elsewhere", "index.html"))

	// A file that is not HTML has no representations to write.
	gen.writePageOutputs(page, filepath.Join(gen.config.OutputDir, "feed.xml"))
	if _, err := os.Stat(filepath.Join(gen.config.OutputDir, "feed.json")); err == nil {
		t.Error("a non-HTML file should produce nothing")
	}
}

// TestCustomFormatThatFailsAtRenderTime is reported, and the other formats on
// the page still get written.
func TestCustomFormatThatFailsAtRenderTime(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "boom.tmpl")
	mustWrite(t, tmpl, "{{ len .Page.Missing }}")
	cfg := outputSite(t, func(cfg *Config) {
		cfg.Outputs = []string{"html", "boom", "json"}
		cfg.OutputsCustom = []CustomOutput{{Name: "boom", Suffix: "index.boom", Template: tmpl}}
	})
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about", "index.json")); err != nil {
		t.Errorf("a failing format must not take the others with it: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about", "index.boom")); err == nil {
		t.Error("a format that failed should write nothing")
	}
}

// TestFormatWithoutMIMEIsNotAdvertised: a format can exist without a discovery
// link, which is what a private representation wants.
func TestFormatWithoutMIMEIsNotAdvertised(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "q.tmpl")
	mustWrite(t, tmpl, "{{ .Page.Title }}")
	cfg := outputSite(t, func(cfg *Config) {
		cfg.Outputs = []string{"html", "quiet"}
		cfg.OutputsCustom = []CustomOutput{{Name: "quiet", Template: tmpl}}
	})
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about", "index.quiet")); err != nil {
		t.Errorf("the default suffix should be index.<name>: %v", err)
	}
	if got := mustRead(t, filepath.Join(cfg.OutputDir, "about", "index.html")); strings.Contains(got, "index.quiet") {
		t.Errorf("a format with no MIME should not be advertised:\n%s", got)
	}
}

// TestMarkdownAndTypeListsCombine: `markdown_publish` adds markdown to every
// list rather than replacing them.
func TestMarkdownAndTypeListsCombine(t *testing.T) {
	cfg := outputSite(t, func(cfg *Config) {
		cfg.MarkdownPublish = true
		cfg.OutputsPerType = map[string][]string{"page": {"html", "json"}}
	})
	for _, rel := range []string{"about/index.json", "about/index.md"} {
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if got := appendUnique([]string{"markdown"}, "markdown"); len(got) != 1 {
		t.Errorf("a format should be added once: %v", got)
	}
}

// TestPageTextOfAnEmptyPage: a page with only a title, and one with only a
// body, both come out as themselves.
func TestPageTextOfAnEmptyPage(t *testing.T) {
	g := &Generator{config: Config{}}
	if got := g.pageText(models.Page{Title: "Only a title"}); got != "Only a title\n" {
		t.Errorf("got %q", got)
	}
	if got := g.pageText(models.Page{Content: "Only a body."}); got != "Only a body.\n" {
		t.Errorf("got %q", got)
	}
}
