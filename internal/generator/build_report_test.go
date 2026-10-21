package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// TestSSGVersionInEveryContext covers #338: every view that has .BuildTime —
// a post, a page, the front listing, an archive and a taxonomy index — also
// says which ssg built it.
func TestSSGVersionInEveryContext(t *testing.T) {
	tmp := t.TempDir()
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "one", "One",
		"technology: [Go]\ncategories: [1]\n")
	mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\nAbout.\n")
	writeTaxonomyMeta(t, tmp)
	dir := filepath.Join(tmp, "templates", "simple")
	for name, body := range map[string]string{
		"index.html":         `{{define "index.html"}}index ssg {{.SSG.Version}}{{end}}`,
		"post.html":          `{{define "post.html"}}post ssg {{.SSG.Version}}{{end}}`,
		"page.html":          `{{define "page.html"}}page ssg {{.SSG.Version}}{{end}}`,
		"category.html":      `{{define "category.html"}}archive ssg {{.SSG.Version}}{{end}}`,
		"taxonomy.html":      `{{define "taxonomy.html"}}taxindex ssg {{.SSG.Version}}{{end}}`,
		"taxonomy-term.html": `{{define "taxonomy-term.html"}}term ssg {{.SSG.Version}}{{end}}`,
	} {
		mustWrite(t, filepath.Join(dir, name), body)
	}
	cfg := taxonomyTestConfig(tmp)
	cfg.Version = "1.8.74"
	cfg.BuildReport = DefaultBuildReport
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := cfg.OutputDir
	for _, rel := range []string{"index.html", "about/index.html", "technology/index.html", "technology/go/index.html"} {
		if got := mustRead(t, filepath.Join(out, rel)); !strings.Contains(got, "ssg 1.8.74") {
			t.Errorf("%s does not name the ssg that built it: %q", rel, got)
		}
	}

	var report buildReport
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(out, "build.json"))), &report); err != nil {
		t.Fatal(err)
	}
	if report.Generator != "ssg" || report.Version != "1.8.74" || report.Pages == 0 || report.Files < report.Pages ||
		report.Content["post"] != 1 || report.Content["page"] != 1 || report.DurationSeconds < 0 || report.Started == "" {
		t.Errorf("report = %+v", report)
	}
}

// TestBuildReportCounts: what the report counts, against a fixed clock and a
// rebuild that finds its own previous copy.
func TestBuildReportCounts(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Version = "1.8.74"
	g.config.BuildReport = "/meta/build.json"
	g.buildTime = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	g.siteData.Posts = []models.Page{{Type: "post"}, {Type: "Recipe"}}
	g.siteData.Pages = []models.Page{{}}
	writeOut(t, g, "index.html", "<html></html>")
	writeOut(t, g, "a/index.HTML", "<html></html>")
	writeOut(t, g, "css/site.css", "body{}")
	writeOut(t, g, "meta/build.json", "{}")
	start := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if err := g.writeBuildReport(start, func() time.Time { return start.Add(2500 * time.Millisecond) }); err != nil {
		t.Fatal(err)
	}
	var r buildReport
	raw, err := os.ReadFile(filepath.Join(g.config.OutputDir, "meta", "build.json"))
	if err != nil || json.Unmarshal(raw, &r) != nil {
		t.Fatalf("report: %v", err)
	}
	if r.Pages != 2 || r.Files != 3 || r.DurationSeconds != 2.5 || r.BuildTime != "2026-10-21T12:00:00Z" ||
		r.Content["post"] != 1 || r.Content["recipe"] != 1 || r.Content["page"] != 1 {
		t.Errorf("report = %+v", r)
	}
}

func TestBuildReportOffAndOutside(t *testing.T) {
	g := newTestGen(t, "")
	if err := g.writeBuildReport(time.Now(), time.Now); err != nil {
		t.Errorf("no report configured must be a no-op: %v", err)
	}
	g.config.BuildReport = "../escape.json"
	if err := g.writeBuildReport(time.Now(), time.Now); err == nil {
		t.Error("a report outside the output directory must be refused")
	}
	g.config.BuildReport = "index.html/build.json"
	writeOut(t, g, "index.html", "x")
	if err := g.writeBuildReport(time.Now(), time.Now); err == nil {
		t.Error("an unwritable report path must be an error")
	}
	g2 := newTestGen(t, "")
	g2.config.OutputDir = filepath.Join(t.TempDir(), "missing")
	g2.config.BuildReport = "build.json"
	if err := g2.writeBuildReport(time.Now(), time.Now); err == nil {
		t.Error("a missing output directory must be an error")
	}
	g2.siteData = nil
	if len(g2.contentCounts()) != 0 {
		t.Error("no site data, no counts")
	}
}

// TestBuildReportIsWorldReadable covers #342: the report is published with
// the site, so a web server running as another user must be able to read it
// — and the directory it sits in — like the pages beside it.
func TestBuildReportIsWorldReadable(t *testing.T) {
	g := newTestGen(t, "")
	g.config.BuildReport = "meta/build.json"
	if err := g.writeBuildReport(time.Now(), time.Now); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(g.config.OutputDir, "meta"), filepath.Join(g.config.OutputDir, "meta", "build.json")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o004 == 0 {
			t.Errorf("%s is %v: others cannot read it", p, info.Mode().Perm())
		}
		if info.IsDir() && info.Mode().Perm()&0o001 == 0 {
			t.Errorf("%s is %v: others cannot enter it", p, info.Mode().Perm())
		}
	}
}
