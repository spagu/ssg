package depgraph

// The graph's whole job is to be wrong in one direction only: when it is not
// sure, it says "rebuild everything". These tests are mostly about that.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// smallGraph is one content file, one data file, one template, one config and
// two outputs.
func smallGraph() *Graph {
	g := New()
	g.AddInput("content/site/posts/a.md", KindContent, "hash-a")
	g.AddInput("content/site/posts/b.md", KindContent, "hash-b")
	g.AddInput("data/authors.json", KindData, "hash-d")
	g.AddInput("templates/theme/post.html", KindTemplate, "hash-t")
	g.AddInput(".ssg.yaml", KindConfig, "hash-c")
	g.Depends("output/a/index.html", "content/site/posts/a.md")
	g.Depends("output/b/index.html", "content/site/posts/b.md")
	g.Depends("output/a/index.html", "data/authors.json")
	return g
}

func TestPlanNarrowsToWhatAChangedFileReaches(t *testing.T) {
	plan := smallGraph().PlanFor([]string{"content/site/posts/a.md"})
	if plan.Full {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Outputs) != 1 || plan.Outputs[0] != "output/a/index.html" {
		t.Errorf("outputs = %v", plan.Outputs)
	}
}

func TestPlanUnionsSeveralChanges(t *testing.T) {
	plan := smallGraph().PlanFor([]string{"content/site/posts/a.md", "data/authors.json"})
	if plan.Full || len(plan.Outputs) != 1 {
		t.Fatalf("plan = %+v", plan) // both reach the same output, counted once
	}
	plan = smallGraph().PlanFor([]string{"content/site/posts/a.md", "content/site/posts/b.md"})
	if plan.Full || len(plan.Outputs) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
}

// TestPlanIsFullWheneverItIsNotCertain walks every case the design commits to
// failing towards a full build.
func TestPlanIsFullWheneverItIsNotCertain(t *testing.T) {
	global := smallGraph()
	global.MarkGlobal("content comes from MDDB")

	wrongSchema := smallGraph()
	wrongSchema.Schema = Schema + 1

	cases := []struct {
		name    string
		graph   *Graph
		changed []string
		want    string
	}{
		{"no graph at all", nil, []string{"x"}, "no usable graph"},
		{"an empty graph", New(), []string{"x"}, "no usable graph"},
		{"a graph from another schema", wrongSchema, []string{"x"}, "no usable graph"},
		{"a build that cannot be narrowed", global, []string{"content/site/posts/a.md"}, "cannot be narrowed"},
		{"a file the graph never saw", smallGraph(), []string{"content/site/posts/new.md"}, "new to this build"},
		{"a template", smallGraph(), []string{"templates/theme/post.html"}, "is a template"},
		{"the configuration", smallGraph(), []string{".ssg.yaml"}, "the configuration"},
		{"an output offered as an input", smallGraph(), []string{"output/a/index.html"}, "not an input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := tc.graph.PlanFor(tc.changed)
			if !plan.Full {
				t.Fatalf("plan = %+v", plan)
			}
			if !strings.Contains(plan.Reason, tc.want) {
				t.Errorf("reason = %q, want it to mention %q", plan.Reason, tc.want)
			}
		})
	}
}

func TestNothingChangedIsNotAFullBuild(t *testing.T) {
	plan := smallGraph().PlanFor(nil)
	if plan.Full || len(plan.Outputs) != 0 || plan.Reason != "nothing changed" {
		t.Errorf("plan = %+v", plan)
	}
}

func TestChangedSinceSeesEditsAndDisappearances(t *testing.T) {
	g := smallGraph()
	hashes := map[string]string{
		"content/site/posts/a.md":   "hash-a",
		"content/site/posts/b.md":   "EDITED",
		"data/authors.json":         "hash-d",
		"templates/theme/post.html": "hash-t",
		// .ssg.yaml is absent: a file the graph recorded and that is now gone.
	}
	changed := g.ChangedSince(func(p string) (string, bool) {
		h, ok := hashes[p]
		return h, ok
	})
	want := map[string]bool{"content/site/posts/b.md": true, ".ssg.yaml": true}
	if len(changed) != len(want) {
		t.Fatalf("changed = %v", changed)
	}
	for _, c := range changed {
		if !want[c] {
			t.Errorf("changed = %v, %s should not be in it", changed, c)
		}
	}
	// Outputs are never compared: they are written, not read.
	for _, c := range changed {
		if strings.HasPrefix(c, "output/") {
			t.Errorf("an output was compared as if it were an input: %s", c)
		}
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := smallGraph()
	original.MarkGlobal("a reason")
	if err := original.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded := Load(dir)
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.Stats() != original.Stats() {
		t.Errorf("stats %+v != %+v", loaded.Stats(), original.Stats())
	}
	if !loaded.IsGlobal() || loaded.Reasons()[0] != "a reason" {
		t.Errorf("reasons = %v", loaded.Reasons())
	}
	if !loaded.HasOutput("output/a/index.html") || loaded.HasOutput("content/site/posts/a.md") {
		t.Error("HasOutput does not distinguish an output from an input")
	}
	if !loaded.Knows("content/site/posts/a.md") || loaded.Knows("content/site/posts/nope.md") {
		t.Error("Knows is wrong")
	}
}

// TestLoadRefusesRatherThanGuesses: every unreadable graph is a full build, not
// an error and not a partial answer.
func TestLoadRefusesRatherThanGuesses(t *testing.T) {
	dir := t.TempDir()
	if g := Load(dir); g != nil {
		t.Error("a missing graph should load as nil")
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if g := Load(dir); g != nil {
		t.Error("a corrupt graph should load as nil")
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"schema":999,"nodes":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if g := Load(dir); g != nil {
		t.Error("a graph from another schema should load as nil")
	}
}

func TestSaveReportsAnUnwritableDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := smallGraph().Save(filepath.Join(file, "graph")); err == nil {
		t.Error("Save should report a directory it cannot create")
	}
}

// TestNilGraphIsUsable: every method has to tolerate the nil a missing graph
// loads as, because the caller's alternative is a nil check at every call.
func TestNilGraphIsUsable(t *testing.T) {
	var g *Graph
	g.AddInput("a", KindContent, "h")
	g.AddOutput("b")
	g.Depends("b", "a")
	g.MarkGlobal("r")
	if !g.IsGlobal() {
		t.Error("a graph that does not exist cannot promise a narrow build")
	}
	if g.Reasons() != nil || g.InputIDs() != nil || g.ChangedSince(HashFile) != nil {
		t.Error("nil accessors should be empty")
	}
	if g.HasOutput("b") || g.Knows("a") {
		t.Error("a nil graph knows nothing")
	}
	if (g.Stats() != Stats{}) {
		t.Error("stats of nothing should be zero")
	}
	if err := g.Save(t.TempDir()); err != nil {
		t.Errorf("saving nothing = %v", err)
	}
	if !strings.Contains(g.DOT(), "digraph ssg") {
		t.Error("DOT of nothing should still be a valid graph")
	}
}

// TestEmptyIDsAreIgnored: a page with no source file must not create an edge
// from "".
func TestEmptyIDsAreIgnored(t *testing.T) {
	g := New()
	g.AddInput("", KindContent, "h")
	g.AddOutput("")
	g.Depends("out", "")
	g.Depends("", "in")
	g.MarkGlobal("")
	if len(g.Nodes) != 0 || len(g.Edges) != 0 || g.IsGlobal() {
		t.Errorf("nodes = %v edges = %v global = %v", g.Nodes, g.Edges, g.Global)
	}
}

func TestDependsAndMarkGlobalDoNotDuplicate(t *testing.T) {
	g := New()
	g.Depends("out", "in")
	g.Depends("out", "in")
	g.MarkGlobal("same")
	g.MarkGlobal("same")
	if len(g.Edges["in"]) != 1 || len(g.Global) != 1 {
		t.Errorf("edges = %v global = %v", g.Edges, g.Global)
	}
}

// TestAddOutputDoesNotOverwriteAnInput: a file read and then written keeps the
// hash that lets it be compared.
func TestAddOutputDoesNotOverwriteAnInput(t *testing.T) {
	g := New()
	g.AddInput("shared", KindContent, "h")
	g.AddOutput("shared")
	if n := g.Nodes["shared"]; n.Kind != KindContent || n.Hash != "h" {
		t.Errorf("node = %+v", n)
	}
}

func TestDOTListsEveryEdge(t *testing.T) {
	dot := smallGraph().DOT()
	for _, want := range []string{
		`"content/site/posts/a.md" -> "output/a/index.html"`,
		`"data/authors.json" -> "output/a/index.html"`,
		`"content/site/posts/b.md" -> "output/b/index.html"`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT is missing %s:\n%s", want, dot)
		}
	}
}

func TestStatsCountsInputsOutputsAndEdges(t *testing.T) {
	if got := (smallGraph().Stats()); got != (Stats{Inputs: 5, Outputs: 2, Edges: 3}) {
		t.Errorf("stats = %+v", got)
	}
}

func TestExplainIsThePlanForOneFile(t *testing.T) {
	g := smallGraph()
	if plan := g.Explain("content/site/posts/a.md"); plan.Full || len(plan.Outputs) != 1 {
		t.Errorf("plan = %+v", plan)
	}
	if plan := g.Explain("templates/theme/post.html"); !plan.Full {
		t.Errorf("plan = %+v", plan)
	}
}

func TestHashFileHashesContentNotNames(t *testing.T) {
	dir := t.TempDir()
	one, two := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	for _, p := range []string{one, two} {
		if err := os.WriteFile(p, []byte("same bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h1, ok1 := HashFile(one)
	h2, ok2 := HashFile(two)
	if !ok1 || !ok2 || h1 != h2 {
		t.Errorf("%q != %q", h1, h2)
	}
	if h1 != HashBytes([]byte("same bytes")) {
		t.Error("HashFile and HashBytes disagree")
	}
	if _, ok := HashFile(filepath.Join(dir, "absent")); ok {
		t.Error("a missing file cannot be hashed")
	}
}

func TestInputIDsExcludesOutputs(t *testing.T) {
	ids := smallGraph().InputIDs()
	if len(ids) != 5 {
		t.Fatalf("ids = %v", ids)
	}
	for _, id := range ids {
		if strings.HasPrefix(id, "output/") {
			t.Errorf("an output appeared among the inputs: %s", id)
		}
	}
}
