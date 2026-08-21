package generator

// One timestamp per build, pinnable for reproducibility (#186).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// fixedNow returns a clock stuck at one instant.
func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

// TestTheClockIsReadWhenNothingPinsIt: the ordinary build. Without this the
// copyright year in a footer is whatever it was when someone last edited it.
func TestTheClockIsReadWhenNothingPinsIt(t *testing.T) {
	t.Setenv(sourceDateEpoch, "")
	// Setenv with an empty value still defines the variable, so clear it the
	// only way that removes it for this test.
	t.Setenv(sourceDateEpoch, "not set")
	want := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	if got := resolveBuildTime(fixedNow(want)); !got.Equal(want) {
		t.Errorf("build time = %v, want the clock %v", got, want)
	}
}

// TestAPinnedEpochWins: the reproducible-builds contract. A CI job that pins
// SOURCE_DATE_EPOCH must get the same bytes out of the same sources.
func TestAPinnedEpochWins(t *testing.T) {
	t.Setenv(sourceDateEpoch, "1700000000")
	got := resolveBuildTime(fixedNow(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)))

	want := time.Unix(1700000000, 0).UTC()
	if !got.Equal(want) {
		t.Errorf("build time = %v, want the pinned %v", got, want)
	}
	// UTC, or the rendered date depends on which machine ran the build — which
	// is the whole thing a pinned epoch exists to prevent.
	if name, _ := got.Zone(); name != "UTC" {
		t.Errorf("zone = %q, want UTC", name)
	}
}

// TestAMalformedEpochIsIgnoredRatherThanFatal: the variable is usually set by a
// surrounding toolchain the site owner does not control, so someone else's typo
// must not break their deploy. The build stays internally consistent either way.
func TestAMalformedEpochIsIgnoredRatherThanFatal(t *testing.T) {
	want := time.Date(2029, 12, 31, 23, 59, 0, 0, time.UTC)
	for _, bad := range []string{"", "yesterday", "17e9", "1.5", " 1700000000 "} {
		t.Setenv(sourceDateEpoch, bad)
		if got := resolveBuildTime(fixedNow(want)); !got.Equal(want) {
			t.Errorf("%q: build time = %v, want the clock to be used", bad, got)
		}
	}
}

// TestZeroAndNegativeEpochsAreHonoured: 0 is a legitimate pin (the Unix epoch),
// and rejecting it as "empty" is the classic bug in code that parses this.
func TestZeroAndNegativeEpochsAreHonoured(t *testing.T) {
	t.Setenv(sourceDateEpoch, "0")
	if got := resolveBuildTime(fixedNow(time.Now())); got.Unix() != 0 {
		t.Errorf("epoch 0 = %v, want 1970-01-01", got)
	}
	t.Setenv(sourceDateEpoch, "-1")
	if got := resolveBuildTime(fixedNow(time.Now())); got.Unix() != -1 {
		t.Errorf("negative epoch = %v", got)
	}
}

// TestEveryPageOfOneBuildAgrees: the reason this is a field and not a template
// function. A footer in a base template renders on posts and on archives, and
// the two must not straddle midnight.
func TestEveryPageOfOneBuildAgrees(t *testing.T) {
	t.Setenv(sourceDateEpoch, "1700000000")
	g, err := New(Config{Domain: "example.com", DefaultLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if g.buildTime.IsZero() {
		t.Fatal("a generator must know when its build started")
	}

	page := g.pageToTemplateData(models.Page{Slug: "hello", Title: "Hello"}, false)
	archive := g.archiveData("category", "news", models.Category{}, nil, Pager{}, "en")

	pageTime, ok := page["BuildTime"].(time.Time)
	if !ok {
		t.Fatalf("page BuildTime = %T, want time.Time", page["BuildTime"])
	}
	archiveTime, ok := archive["BuildTime"].(time.Time)
	if !ok {
		t.Fatalf("archive BuildTime = %T, want time.Time", archive["BuildTime"])
	}
	if !pageTime.Equal(archiveTime) || !pageTime.Equal(g.buildTime) {
		t.Errorf("page %v and archive %v must both be the build's %v",
			pageTime, archiveTime, g.buildTime)
	}
	if pageTime.Year() != 2023 {
		t.Errorf("Year() = %d, want the pinned 2023", pageTime.Year())
	}
}

// TestBuildTimeReachesEveryTemplateContext is the assertion the map-level tests
// could not make. Pages and archives render from a map, but the front page and
// a taxonomy index render from anonymous structs of their own — a field absent
// there is not a missing value, it is `can't evaluate field BuildTime`, a hard
// template error on the most linked document the site has. That is exactly how
// this first shipped, and only an end-to-end build found it.
func TestBuildTimeReachesEveryTemplateContext(t *testing.T) {
	t.Setenv(sourceDateEpoch, "1700000000")
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")

	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "posts", "news", "one.md"),
		"---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\ntags: [go]\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n")

	// Every template reads .BuildTime, which is what a shared footer partial
	// amounts to. Any context that lacks the field fails the build.
	tmplDir := filepath.Join(tmp, "templates", "simple")
	for _, name := range []string{"base.html", "index.html", "post.html", "page.html",
		"category.html", "tag.html", "taxonomy.html"} {
		mustWrite(t, filepath.Join(tmplDir, name),
			`{{define "`+name+`"}}<html><body>YEAR={{.BuildTime.Year}}</body></html>{{end}}`)
	}

	cfg := Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The front page is the one that failed, so it is named explicitly rather
	// than swept up by a walk.
	front := mustRead(t, filepath.Join(tmp, "output", indexHTMLName))
	if !strings.Contains(front, "YEAR=2023") {
		t.Errorf("front page = %q, want the pinned year", front)
	}

	// And every other rendered document agrees — no page of one build may show
	// a different year from another.
	var checked int
	err = filepath.WalkDir(filepath.Join(tmp, "output"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return nil //nolint:nilerr // non-HTML output is not this test's business
		}
		body := mustRead(t, p)
		if !strings.Contains(body, "YEAR=") {
			return nil // a template this fixture did not define
		}
		checked++
		if !strings.Contains(body, "YEAR=2023") {
			t.Errorf("%s disagrees about the build year: %q", p, body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 3 {
		t.Fatalf("only %d document(s) rendered .BuildTime — the fixture proves too little", checked)
	}
}
