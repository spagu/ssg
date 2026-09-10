package generator

// The dependency graph, and the one property that matters: an incremental
// build produces the same site as a full one (GO-094).

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/models"
)

// incrementalFixture builds a small site whose content can be edited between
// builds, in a working directory of its own so the graph lands beside it.
func incrementalFixture(t *testing.T, posts int) Config {
	t.Helper()
	files := map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout us.\n",
	}
	for i := 0; i < posts; i++ {
		name := string(rune('a' + i))
		files["posts/news/"+name+".md"] = "---\ntitle: Post " + name + "\nslug: post-" + name +
			"\nstatus: publish\ntype: post\ndate: 2024-01-0" + string(rune('1'+i%9)) + "\ntags: [go]\n---\n\nBody of " + name + ".\n"
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.Incremental = true
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Quiet = true
	return cfg
}

// contentRoot is where a fixture's Markdown lives.
func contentRoot(cfg Config) string {
	return filepath.Join(cfg.ContentDir, cfg.Source)
}

// treeOf reads an output tree into a map, for comparing two builds.
func treeOf(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(p) // #nosec G304 -- test fixture
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// compareTreesExactly fails with the first file that differs.
func compareTreesExactly(t *testing.T, want, got map[string][]byte, context string) {
	t.Helper()
	for rel, wantData := range want {
		gotData, ok := got[rel]
		if !ok {
			t.Errorf("%s: %s is missing from the incremental build", context, rel)
			continue
		}
		if !bytes.Equal(wantData, gotData) {
			t.Errorf("%s: %s differs between a full and an incremental build", context, rel)
		}
	}
	for rel := range got {
		if _, ok := want[rel]; !ok {
			t.Errorf("%s: %s exists only in the incremental build", context, rel)
		}
	}
}

// TestIncrementalEqualsFull is the king of this feature's acceptance criteria.
//
// A random sequence of edits, applied twice: once to a site rebuilt whole each
// time, once to a site rebuilt incrementally. The two trees must be identical
// at every step, byte for byte. A graph with a missing edge produces a stale
// page and a green build, which is the failure this test exists to catch.
func TestIncrementalEqualsFull(t *testing.T) {
	const posts = 6
	full := incrementalFixture(t, posts)
	full.Incremental = false
	inc := incrementalFixture(t, posts)

	// Mirror the content of one fixture onto the other so both start equal.
	mirror := func(rel, body string) {
		mustWrite(t, filepath.Join(contentRoot(full), filepath.FromSlash(rel)), body)
		mustWrite(t, filepath.Join(contentRoot(inc), filepath.FromSlash(rel)), body)
	}

	narrowed := 0
	buildBoth := func(step string) {
		t.Helper()
		buildSiteFixture(t, full)
		gen, err := New(inc)
		if err != nil {
			t.Fatal(err)
		}
		if err := gen.Generate(); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if !gen.plan.Full {
			narrowed++
		}
		compareTreesExactly(t, treeOf(t, full.OutputDir), treeOf(t, inc.OutputDir), step)
	}

	buildBoth("first build")

	rng := rand.New(rand.NewSource(1)) // #nosec G404 -- a test needs a repeatable sequence
	for step := 0; step < 6; step++ {
		name := string(rune('a' + rng.Intn(posts)))
		body := "---\ntitle: Post " + name + " v" + string(rune('1'+step)) +
			"\nslug: post-" + name + "\nstatus: publish\ntype: post\ndate: 2024-01-02\ntags: [go]\n---\n\n" +
			"Rewritten at step " + string(rune('1'+step)) + ".\n"
		mirror("posts/news/"+name+".md", body)
		buildBoth("after editing " + name)
	}

	// A page that did not exist when the graph was written: the case a hash
	// comparison alone cannot see.
	mirror("posts/news/late.md",
		"---\ntitle: Late\nslug: post-late\nstatus: publish\ntype: post\ndate: 2024-03-03\ntags: [go]\n---\n\nLate arrival.\n")
	buildBoth("after adding a page")
	mirror("pages/about.md", "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nRewritten.\n")
	buildBoth("after editing a page")

	// A run in which every build silently fell back to a full one would pass
	// every comparison above and prove nothing.
	if narrowed == 0 {
		t.Error("no build in this sequence was narrowed, so the comparison proved nothing")
	}
}

// TestIncrementalSkipsWhatCannotHaveChanged: the point of the feature. One
// post edited, and the other pages' files are not rewritten.
func TestIncrementalSkipsWhatCannotHaveChanged(t *testing.T) {
	cfg := incrementalFixture(t, 4)
	buildSiteFixture(t, cfg)

	untouched := filepath.Join(cfg.OutputDir, "2024", "01", "03", "post-c", "index.html")
	before, err := os.Stat(untouched)
	if err != nil {
		t.Fatal(err)
	}
	// Make the marker unmistakable: replace the file with something the build
	// would overwrite if it rendered the page again.
	if err := os.WriteFile(untouched, []byte("SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(contentRoot(cfg), "posts", "news", "a.md"),
		"---\ntitle: Post a\nslug: post-a\nstatus: publish\ntype: post\ndate: 2024-01-01\ntags: [go]\n---\n\nEdited.\n")
	buildSiteFixture(t, cfg)

	if got := mustRead(t, untouched); got != "SENTINEL" {
		t.Errorf("a page nothing could have changed was rewritten")
	}
	edited := mustRead(t, filepath.Join(cfg.OutputDir, "2024", "01", "01", "post-a", "index.html"))
	if !strings.Contains(edited, "Edited.") {
		t.Errorf("the edited page was not rewritten:\n%s", edited)
	}
	_ = before
}

// TestChangedTemplateForcesAFullBuild: the graph does not model which pages a
// partial reaches, so it refuses to guess.
func TestChangedTemplateForcesAFullBuild(t *testing.T) {
	cfg := incrementalFixture(t, 3)
	buildSiteFixture(t, cfg)

	sentinel := filepath.Join(cfg.OutputDir, "2024", "01", "02", "post-b", "index.html")
	if err := os.WriteFile(sentinel, []byte("SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmpl := filepath.Join(cfg.TemplatesDir, cfg.Template, "post.html")
	body := mustRead(t, tmpl)
	mustWrite(t, tmpl, strings.Replace(body, "<body>", "<body data-changed=\"1\">", 1))
	buildSiteFixture(t, cfg)

	if got := mustRead(t, sentinel); got == "SENTINEL" {
		t.Error("a changed template must rebuild every page")
	}
}

// TestNewContentForcesAFullBuild: a file the previous build never saw is a file
// whose effects are unknown.
func TestNewContentForcesAFullBuild(t *testing.T) {
	cfg := incrementalFixture(t, 2)
	buildSiteFixture(t, cfg)
	sentinel := filepath.Join(cfg.OutputDir, "2024", "01", "01", "post-a", "index.html")
	if err := os.WriteFile(sentinel, []byte("SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(contentRoot(cfg), "posts", "news", "z.md"),
		"---\ntitle: New\nslug: post-z\nstatus: publish\ntype: post\ndate: 2024-02-02\n---\n\nNew.\n")
	buildSiteFixture(t, cfg)
	if got := mustRead(t, sentinel); got == "SENTINEL" {
		t.Error("a new file must rebuild the site, because what depends on it is unknown")
	}
}

// TestGraphIsWrittenByEveryBuild, incremental or not: a graph that describes a
// build nobody ran is worse than none.
func TestGraphIsWrittenByEveryBuild(t *testing.T) {
	cfg := incrementalFixture(t, 2)
	cfg.Incremental = false
	buildSiteFixture(t, cfg)

	graph := depgraph.Load(filepath.Join(cfg.CacheDir, GraphDirName))
	if graph == nil {
		t.Fatal("a full build must still record the graph")
	}
	stats := graph.Stats()
	if stats.Inputs == 0 || stats.Outputs == 0 || stats.Edges == 0 {
		t.Errorf("stats = %+v", stats)
	}
	if graph.IsGlobal() {
		t.Errorf("a plain file-backed site should be narrowable: %v", graph.Reasons())
	}
}

// TestGraphExplainsAFanOut: a build that cannot be narrowed says why, because
// "incremental saves you nothing" is useless without the reason.
func TestGraphExplainsAFanOut(t *testing.T) {
	cfg := incrementalFixture(t, 2)
	cfg.ExternalSources.Enabled = true
	buildSiteFixture(t, cfg)
	graph := depgraph.Load(filepath.Join(cfg.CacheDir, GraphDirName))
	if graph == nil || !graph.IsGlobal() {
		t.Fatalf("graph = %+v", graph)
	}
	if reasons := graph.Reasons(); len(reasons) == 0 || !strings.Contains(reasons[0], "external sources") {
		t.Errorf("reasons = %v", reasons)
	}
	// And its plan is full whatever changed.
	if plan := graph.PlanFor([]string{"anything"}); !plan.Full {
		t.Errorf("plan = %+v", plan)
	}
}

// TestIncrementalWithoutAPreviousGraphIsFull.
func TestIncrementalWithoutAPreviousGraphIsFull(t *testing.T) {
	cfg := incrementalFixture(t, 2)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	if !gen.plan.Full || !strings.Contains(gen.plan.Reason, "no usable graph") {
		t.Errorf("plan = %+v", gen.plan)
	}
}

// TestCleanCancelsNarrowing: --clean empties the output directory, so there is
// nothing left to keep and every page has to be written again.
func TestCleanCancelsNarrowing(t *testing.T) {
	cfg := incrementalFixture(t, 2)
	buildSiteFixture(t, cfg)
	cfg.Clean = true
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	if !gen.plan.Full || !strings.Contains(gen.plan.Reason, "--clean") {
		t.Errorf("plan = %+v", gen.plan)
	}
	if gen.Graph() == nil {
		t.Error("Graph() should expose what the build recorded")
	}
}

// TestNarrowingIsAnnouncedInTheLog: a build that quietly changed shape is one
// nobody can reason about.
func TestNarrowingIsAnnouncedInTheLog(t *testing.T) {
	cfg := incrementalFixture(t, 2)
	cfg.Quiet = false
	out := captureBuildOutput(t, func() { buildSiteFixture(t, cfg) })
	if !strings.Contains(out, "Full build:") {
		t.Errorf("a first build is full and should say so:\n%s", out)
	}
	mustWrite(t, filepath.Join(contentRoot(cfg), "posts", "news", "a.md"),
		"---\ntitle: Post a\nslug: post-a\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\nEdited.\n")
	out = captureBuildOutput(t, func() { buildSiteFixture(t, cfg) })
	if !strings.Contains(out, "Incremental:") {
		t.Errorf("a narrowed build should say what it narrowed to:\n%s", out)
	}
}

// TestAnUnhashableInputCancelsNarrowing: a file that cannot be read now cannot
// be compared next time either.
func TestAnUnhashableInputCancelsNarrowing(t *testing.T) {
	cfg := incrementalFixture(t, 1)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	gen.startGraph()
	gen.recordInput(filepath.Join(t.TempDir(), "absent.md"), depgraph.KindContent)
	if !gen.graph.IsGlobal() {
		t.Fatal("an unreadable input should cancel narrowing")
	}
	if !strings.Contains(gen.graph.Reasons()[0], "could not be read for hashing") {
		t.Errorf("reasons = %v", gen.graph.Reasons())
	}
}

// TestGraphIDIsRelativeAndPortable, so a graph written on one machine reads on
// another.
func TestGraphIDIsRelativeAndPortable(t *testing.T) {
	gen := &Generator{}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := gen.graphID(filepath.Join(wd, "content", "site", "a.md")); got != "content/site/a.md" {
		t.Errorf("graphID = %q", got)
	}
	// A path outside the project keeps its own shape rather than growing "..".
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	if got := gen.graphID(outside); strings.HasPrefix(got, "..") {
		t.Errorf("graphID = %q", got)
	}
}

// TestPageSourcePathHandlesPagesWithoutAFile: a CMS import has none, and it
// must not produce an edge from "".
func TestPageSourcePathHandlesPagesWithoutAFile(t *testing.T) {
	if got := pageSourcePath(models.Page{}); got != "" {
		t.Errorf("pageSourcePath = %q", got)
	}
	if got := pageSourcePath(models.Page{SourceFile: "a.md"}); got != "a.md" {
		t.Errorf("pageSourcePath = %q", got)
	}
	if got := pageSourcePath(models.Page{SourceDir: "d", SourceFile: "a.md"}); got != filepath.Join("d", "a.md") {
		t.Errorf("pageSourcePath = %q", got)
	}
}

// TestGraphFailureIsReportedNotFatal: a cache that cannot be written should not
// fail a build whose output is correct.
func TestGraphFailureIsReportedNotFatal(t *testing.T) {
	cfg := incrementalFixture(t, 1)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.CacheDir = blocker // a file where a directory has to go
	cfg.Quiet = false
	out := captureBuildOutput(t, func() { buildSiteFixture(t, cfg) })
	if !strings.Contains(out, "dependency graph:") {
		t.Errorf("the failure should be reported:\n%s", out)
	}
}

// TestMddbContentCancelsNarrowing: content the graph cannot hash.
func TestMddbContentCancelsNarrowing(t *testing.T) {
	cfg := incrementalFixture(t, 1)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	gen.startGraph()
	gen.config.Mddb.Enabled = true
	gen.recordContentInputs()
	if !gen.graph.IsGlobal() || !strings.Contains(gen.graph.Reasons()[0], "MDDB") {
		t.Errorf("reasons = %v", gen.graph.Reasons())
	}
}
