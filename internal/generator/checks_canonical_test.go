package generator

// An empty canonical is wrong for every site and was reported by nothing (#247).

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNoteEmptyCanonicalRecognisesTheBrokenTags: what a theme actually emits
// when it names a value its context does not carry.
func TestNoteEmptyCanonicalRecognisesTheBrokenTags(t *testing.T) {
	broken := map[string]string{
		`empty href`:          `<head><link rel="canonical" href=""/></head>`,
		`no href at all`:      `<head><link rel="canonical"></head>`,
		`single quotes`:       `<head><link rel='canonical' href=''></head>`,
		`unquoted rel`:        `<head><link rel=canonical href=""></head>`,
		`href before rel`:     `<head><link href="" rel="canonical"></head>`,
		`whitespace only`:     `<head><link rel="canonical" href="   "></head>`,
		`empty og:url`:        `<head><meta property="og:url" content=""></head>`,
		`empty twitter:url`:   `<head><meta name="twitter:url" content=""></head>`,
		`no closing head tag`: `<link rel="canonical" href="">`,
	}
	for label, doc := range broken {
		g := newTestGen(t, "")
		g.noteEmptyCanonical(filepath.Join(g.config.OutputDir, "page", indexHTMLName), doc)
		if len(g.emptyCanonicals) != 1 {
			t.Errorf("%s: not reported", label)
		}
	}
}

// TestNoteEmptyCanonicalLeavesGoodPagesAlone: a check that cries wolf gets
// filtered out, so the negatives matter as much as the positives.
func TestNoteEmptyCanonicalLeavesGoodPagesAlone(t *testing.T) {
	fine := map[string]string{
		`a real canonical`: `<head><link rel="canonical" href="https://example.com/a/"></head>`,
		`no canonical`:     `<head><title>T</title></head><body><p>x</p></body>`,
		`relative`:         `<head><link rel="canonical" href="/a/"></head>`,
		`another rel`:      `<head><link rel="stylesheet" href=""></head>`,
		`filled self URLs`: `<head><meta property="og:url" content="https://example.com/a/"></head>`,
		`only in the body`: `<head><title>T</title></head><body><code>&lt;link rel="canonical" href=""&gt;</code></body>`,
	}
	for label, doc := range fine {
		g := newTestGen(t, "")
		g.noteEmptyCanonical(filepath.Join(g.config.OutputDir, "page", indexHTMLName), doc)
		if len(g.emptyCanonicals) != 0 {
			t.Errorf("%s: reported %v", label, g.emptyCanonicals)
		}
	}
}

// TestNoteEmptyCanonicalNamesEveryEmptyTag: they are written from one value, so
// they fail together and are worth reporting together.
func TestNoteEmptyCanonicalNamesEveryEmptyTag(t *testing.T) {
	g := newTestGen(t, "")
	g.noteEmptyCanonical(filepath.Join(g.config.OutputDir, "tag", "go", indexHTMLName),
		`<head><link rel="canonical" href=""><meta property="og:url" content="">`+
			`<meta name="twitter:url" content=""></head>`)

	detail, ok := g.emptyCanonicals["tag/go/"+indexHTMLName]
	if !ok {
		t.Fatalf("recorded under the wrong key: %v", g.emptyCanonicals)
	}
	for _, want := range []string{`<link rel="canonical">`, "og:url", "twitter:url"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not name %s", detail, want)
		}
	}
}

// TestReportEmptyCanonicalsCapsTheList: the failure is usually site-wide, and a
// warning that scrolls the build out of the terminal teaches people to ignore
// warnings.
func TestReportEmptyCanonicalsCapsTheList(t *testing.T) {
	g := newTestGen(t, "")
	g.emptyCanonicals = map[string]string{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		g.emptyCanonicals[name+"/"+indexHTMLName] = `<link rel="canonical">`
	}
	out := captureBuildOutput(t, g.reportEmptyCanonicals)

	if !strings.Contains(out, "7 page(s)") {
		t.Errorf("the count is missing:\n%s", out)
	}
	if !strings.Contains(out, "…and 2 more") {
		t.Errorf("the list is not capped:\n%s", out)
	}
	if strings.Contains(out, "g/"+indexHTMLName) {
		t.Errorf("more than the cap was listed:\n%s", out)
	}
	// Sorted, so two builds of the same site read the same way.
	if !strings.Contains(out, "a/"+indexHTMLName) {
		t.Errorf("the first file is missing:\n%s", out)
	}
}

// TestReportEmptyCanonicalsSaysNothingWhenClean: silence is the normal case.
func TestReportEmptyCanonicalsSaysNothingWhenClean(t *testing.T) {
	g := newTestGen(t, "")
	if out := captureBuildOutput(t, g.reportEmptyCanonicals); out != "" {
		t.Errorf("a clean build printed %q", out)
	}
	// Quiet keeps it quiet, like every other build report.
	g.config.Quiet = true
	g.emptyCanonicals = map[string]string{"a/" + indexHTMLName: "og:url"}
	if out := captureBuildOutput(t, g.reportEmptyCanonicals); out != "" {
		t.Errorf("--quiet printed %q", out)
	}
}

// TestBuildReportsAnEmptyCanonical: end to end, through a real theme making the
// real mistake — reading a key the archive context does not carry.
func TestBuildReportsAnEmptyCanonical(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[{"id":2,"name":"News","slug":"news"}],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\n---\n\nBody.\n",
		}, func(name string) string {
			if name == "category.html" {
				// The #245 mistake: a name the context does not carry.
				return `<html><head><link rel="canonical" href="{{.CanonicalUrl}}"/></head><body><p>x</p></body></html>`
			}
			return `<html><head><title>T</title></head><body><p>x</p></body></html>`
		})
	cfg.Quiet = false

	out := captureBuildOutput(t, func() { buildSiteFixture(t, cfg) })

	if !strings.Contains(out, "name their own URL with an empty value") {
		t.Errorf("the build did not report the empty canonical:\n%s", out)
	}
	if !strings.Contains(out, "category/news/"+indexHTMLName) {
		t.Errorf("the report does not name the file:\n%s", out)
	}
	// And it stays a warning: the site is publishable.
	if _, err := filepath.Glob(filepath.Join(cfg.OutputDir, "category", "news", indexHTMLName)); err != nil {
		t.Fatal(err)
	}
}

// TestBuildIsQuietOnAGoodTheme: the same build with the right key says nothing,
// which is what makes the warning above worth reading.
func TestBuildIsQuietOnAGoodTheme(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[{"id":2,"name":"News","slug":"news"}],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\n---\n\nBody.\n",
		}, func(name string) string {
			if name == "category.html" {
				return `<html><head><link rel="canonical" href="{{.CanonicalURL}}"/></head><body><p>x</p></body></html>`
			}
			return `<html><head><title>T</title></head><body><p>x</p></body></html>`
		})
	cfg.Quiet = false

	out := captureBuildOutput(t, func() { buildSiteFixture(t, cfg) })

	if strings.Contains(out, "name their own URL with an empty value") {
		t.Errorf("a correct theme was reported:\n%s", out)
	}
}
