package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// publicWorker makes a worker directory whose public/ holds files.
func publicWorker(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		mustWrite(t, filepath.Join(dir, "public", filepath.FromSlash(rel)), body)
	}
	return dir
}

func publicGen(t *testing.T) *Generator {
	t.Helper()
	return &Generator{config: Config{OutputDir: t.TempDir(), StaticDir: filepath.Join(t.TempDir(), "static"), Quiet: true}}
}

func wantClash(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("a clash must stop the build")
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("message lacks %q:\n%v", p, err)
		}
	}
}

// TestWorkerPublicOverPage (#420): a site page at /core/ meets core's
// /core/index.html — both are named, the page survives.
func TestWorkerPublicOverPage(t *testing.T) {
	g := publicGen(t)
	g.siteData = &models.SiteData{Pages: []models.Page{{Title: "Core", Link: "/core/", SourceFile: "content/pages/core.md"}}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "core", "index.html"), "<p>the site's page</p>")
	w := publicWorker(t, map[string]string{"core/index.html": "<p>the worker's page</p>"})
	err := g.copyWorkerPublic(w, `"core"`, map[string]string{})
	wantClash(t, err, "output collision at /core/index.html", `worker "core" (public/core/index.html)`, "content/pages/core.md", "silently replace")
	if got := mustRead(t, filepath.Join(g.config.OutputDir, "core", "index.html")); got != "<p>the site's page</p>" {
		t.Errorf("the page was overwritten: %q", got)
	}
	// The front page, a post, and a page elsewhere.
	g.siteData = &models.SiteData{Posts: []models.Page{{Title: "Elsewhere", Link: "/other/"}}, Pages: []models.Page{{Title: "Home", Link: "/", SourceFile: "content/index.md"}}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "index.html"), "home")
	err = g.copyWorkerPublic(publicWorker(t, map[string]string{"index.html": "worker"}), `"w"`, map[string]string{})
	wantClash(t, err, "/index.html", "content/index.md")
}

// TestWorkerPublicOverStaticAsset: a static file meets a worker's file.
func TestWorkerPublicOverStaticAsset(t *testing.T) {
	g := publicGen(t)
	mustWrite(t, filepath.Join(g.config.StaticDir, "js", "consent.js"), "site")
	mustWrite(t, filepath.Join(g.config.OutputDir, "js", "consent.js"), "site")
	err := g.copyWorkerPublic(publicWorker(t, map[string]string{"js/consent.js": "worker"}), "worker[0]", map[string]string{})
	wantClash(t, err, "/js/consent.js", "worker worker[0]", "the static file "+filepath.ToSlash(g.config.StaticDir)+"/js/consent.js")
}

// TestWorkerPublicOverGeneratedFile: a file the build generated (here a
// sitemap; nested, a feed) meets a worker's file.
func TestWorkerPublicOverGeneratedFile(t *testing.T) {
	g := publicGen(t)
	g.config.StaticDir = "" // the default static/ of the working directory, which has none of these
	g.siteData = &models.SiteData{Pages: []models.Page{{Title: "Elsewhere", Link: "/elsewhere/"}}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "sitemap.xml"), "<urlset/>")
	err := g.copyWorkerPublic(publicWorker(t, map[string]string{"sitemap.xml": "<x/>"}), `"w"`, map[string]string{})
	wantClash(t, err, "/sitemap.xml", "the file the build already wrote there")
	mustWrite(t, filepath.Join(g.config.OutputDir, "blog", "feeds", "deep", "atom.xml"), "<feed/>")
	err = g.copyWorkerPublic(publicWorker(t, map[string]string{"blog/feeds/deep/atom.xml": "<x/>"}), `"w"`, map[string]string{})
	wantClash(t, err, "/blog/feeds/deep/atom.xml")
	// A directory where the worker has a file.
	mustMkdir(t, filepath.Join(g.config.OutputDir, "panel"))
	err = g.copyWorkerPublic(publicWorker(t, map[string]string{"panel": "x"}), `"w"`, map[string]string{})
	wantClash(t, err, "the directory /panel/ this build wrote")
}

// TestWorkerPublicIdenticalIsNoClash: the same bytes lose nothing.
func TestWorkerPublicIdenticalIsNoClash(t *testing.T) {
	g := publicGen(t)
	g.siteData = &models.SiteData{Pages: []models.Page{{Title: "Same", Link: "/a/b/"}}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "a", "b", "index.html"), "same")
	w := publicWorker(t, map[string]string{"a/b/index.html": "same", "a/b/new.css": "new"})
	if err := g.copyWorkerPublic(w, `"w"`, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if mustRead(t, filepath.Join(g.config.OutputDir, "a", "b", "new.css")) != "new" {
		t.Error("the rest of public/ is copied")
	}
	if same(filepath.Join(w, "missing"), filepath.Join(w, "public", "a", "b", "new.css")) {
		t.Error("an unreadable file is never the same")
	}
}

// TestWorkerPublicTwoWorkers: the existing worker-vs-worker error stands.
func TestWorkerPublicTwoWorkers(t *testing.T) {
	g := publicGen(t)
	seen := map[string]string{}
	if err := g.copyWorkerPublic(publicWorker(t, map[string]string{"x/a.js": "1"}), `"one"`, seen); err != nil {
		t.Fatal(err)
	}
	err := g.copyWorkerPublic(publicWorker(t, map[string]string{"x/a.js": "1"}), `"two"`, seen)
	if err == nil || err.Error() != `worker "two" and "one" both provide public asset x/a.js` {
		t.Errorf("got %v", err)
	}
}

// TestWorkerPublicLeftoverFromEarlierBuild: without clean, the worker's own
// copy from the last build (an older template, or core's generated theme.css)
// is replaced as before; a file this build rewrote is not.
func TestWorkerPublicLeftoverFromEarlierBuild(t *testing.T) {
	g := publicGen(t)
	w := publicWorker(t, map[string]string{"core/theme.css": "", "core/nav.js": "new"})
	g.config.Workers = []WorkerConfig{{Name: "core", Dir: w}, {Name: "fetched", Source: "https://example.com/w.zip"}, {Name: "nothing"}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "core", "theme.css"), ":root { --primary: #123456; }")
	mustWrite(t, filepath.Join(g.config.OutputDir, "core", "nav.js"), "old")
	g.snapshotWorkerPublic()
	if len(g.publicBefore) != 2 {
		t.Fatalf("snapshot %v", g.publicBefore)
	}
	if err := g.copyWorkerPublic(w, `"core"`, map[string]string{}); err != nil {
		t.Fatalf("a leftover is not a clash: %v", err)
	}
	if mustRead(t, filepath.Join(g.config.OutputDir, "core", "nav.js")) != "new" {
		t.Error("the leftover is replaced")
	}
	// The same path written again during this build is a clash.
	g.snapshotWorkerPublic()
	path := filepath.Join(g.config.OutputDir, "core", "nav.js")
	mustWrite(t, path, "rewritten by this build, longer")
	err := g.copyWorkerPublic(w, `"core"`, map[string]string{})
	wantClash(t, err, "/core/nav.js")
	if workerPublicDir(WorkerConfig{}) != "" || workerPublicDir(WorkerConfig{Source: "x", Name: "n"}) != filepath.Join("workers", "n", "public") {
		t.Error("workerPublicDir")
	}
}

// TestWorkerPublicOverPageInFullBuild reproduces #420 end to end: a content
// page at /core/ and the shipped core worker.
func TestWorkerPublicOverPageInFullBuild(t *testing.T) {
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{"categories":[],"exported_at":"","media":[],"users":[]}`)
	mustWrite(t, filepath.Join(content, "pages", "core.md"), "---\ntitle: Core values\nslug: core\nstatus: publish\ntype: page\nlink: /core/\n---\n\nWhat we stand for.\n")
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: filepath.Join(tmp, "output"), Quiet: true,
		Workers: []WorkerConfig{{Name: "core", Dir: filepath.Join(templates, "core")}}})
	if err != nil {
		t.Fatal(err)
	}
	var genErr error
	_, _ = capture(t, func() error { genErr = gen.Generate(); return nil })
	wantClash(t, genErr, "output collision at /core/index.html", `worker "core"`, "core.md")
	if _, err := os.Stat(filepath.Join(tmp, "output", "core", "nav.js")); err == nil {
		t.Error("nothing of the worker's public files is copied after a clash")
	}
}
