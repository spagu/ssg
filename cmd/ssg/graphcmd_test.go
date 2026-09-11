package main

// `ssg graph` answers two questions a build log cannot: what a change rebuilds,
// and why a build cannot be narrowed (GO-094).

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/generator"
)

// seedGraph writes a graph where the current directory's build would keep one,
// and returns nothing: the tests read it back through runGraph.
func seedGraph(t *testing.T, global bool) {
	t.Helper()
	g := depgraph.New()
	g.AddInput("content/site/posts/a.md", depgraph.KindContent, "hash-a")
	g.AddInput("templates/theme/post.html", depgraph.KindTemplate, "hash-t")
	g.Depends("output/a/index.html", "content/site/posts/a.md")
	g.Depends("output/index.html", "content/site/posts/a.md")
	if global {
		g.MarkGlobal("external sources can change without a file changing")
	}
	if err := g.Save(".ssg-cache/" + generator.GraphDirName); err != nil {
		t.Fatal(err)
	}
}

// captureGraphOutput collects what a command printed.
func captureGraphOutput(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	code := fn()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out, code
}

func TestGraphWithoutABuildSaysSo(t *testing.T) {
	t.Chdir(t.TempDir())
	if code := runGraph(nil); code != 1 {
		t.Errorf("code = %d", code)
	}
}

func TestGraphSummarisesTheLastBuild(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	out, code := captureGraphOutput(t, func() int { return runGraph(nil) })
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out, "2 inputs") || !strings.Contains(out, "2 outputs") || !strings.Contains(out, "2 edges") {
		t.Errorf("summary = %q", out)
	}
	if !strings.Contains(out, "ssg graph content/site/posts/hello.md") {
		t.Errorf("the summary should say how to ask about one file: %q", out)
	}
}

func TestGraphExplainsWhyABuildCannotBeNarrowed(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, true)
	out, _ := captureGraphOutput(t, func() int { return runGraph(nil) })
	if !strings.Contains(out, "cannot be narrowed") || !strings.Contains(out, "external sources") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "That is correct, not a bug") {
		t.Errorf("the explanation should say a global build is not a failure: %q", out)
	}
}

func TestGraphAnswersForOneFile(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	out, _ := captureGraphOutput(t, func() int { return runGraph([]string{"content/site/posts/a.md"}) })
	if !strings.Contains(out, "2 output(s)") || !strings.Contains(out, "output/a/index.html") {
		t.Errorf("output = %q", out)
	}
	out, _ = captureGraphOutput(t, func() int { return runGraph([]string{"templates/theme/post.html"}) })
	if !strings.Contains(out, "a full rebuild") || !strings.Contains(out, "is a template") {
		t.Errorf("output = %q", out)
	}
}

func TestGraphSaysNothingWhenNothingDependsOnAFile(t *testing.T) {
	t.Chdir(t.TempDir())
	g := depgraph.New()
	g.AddInput("data/unused.json", depgraph.KindData, "h")
	g.AddOutput("output/index.html")
	if err := g.Save(".ssg-cache/" + generator.GraphDirName); err != nil {
		t.Fatal(err)
	}
	out, _ := captureGraphOutput(t, func() int { return runGraph([]string{"data/unused.json"}) })
	if !strings.Contains(out, "→ nothing") {
		t.Errorf("output = %q", out)
	}
}

// TestGraphTruncatesALongList, and says how to get the rest.
func TestGraphTruncatesALongList(t *testing.T) {
	t.Chdir(t.TempDir())
	g := depgraph.New()
	g.AddInput("data/authors.json", depgraph.KindData, "h")
	for i := 0; i < 45; i++ {
		g.Depends("output/"+string(rune('a'+i%26))+string(rune('a'+i/26))+"/index.html", "data/authors.json")
	}
	if err := g.Save(".ssg-cache/" + generator.GraphDirName); err != nil {
		t.Fatal(err)
	}
	out, _ := captureGraphOutput(t, func() int { return runGraph([]string{"data/authors.json"}) })
	if !strings.Contains(out, "and 5 more") || !strings.Contains(out, "--json") {
		t.Errorf("output = %q", out)
	}
}

func TestGraphJSONIsMachineReadable(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)

	out, _ := captureGraphOutput(t, func() int { return runGraph([]string{"--json"}) })
	var summary struct {
		Inputs, Outputs, Edges int
		Global                 []string
	}
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("%v in %q", err, out)
	}
	if summary.Inputs != 2 || summary.Outputs != 2 || summary.Edges != 2 || len(summary.Global) != 0 {
		t.Errorf("summary = %+v", summary)
	}

	out, _ = captureGraphOutput(t, func() int {
		return runGraph([]string{"--json", "content/site/posts/a.md"})
	})
	var one struct {
		Path    string
		Full    bool
		Reason  string
		Outputs []string
	}
	if err := json.Unmarshal([]byte(out), &one); err != nil {
		t.Fatalf("%v in %q", err, out)
	}
	if one.Path != "content/site/posts/a.md" || one.Full || len(one.Outputs) != 2 {
		t.Errorf("answer = %+v", one)
	}
	if one.Outputs[0] > one.Outputs[1] {
		t.Errorf("outputs should be sorted: %v", one.Outputs)
	}
}

func TestGraphDOTRenders(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	out, code := captureGraphOutput(t, func() int { return runGraph([]string{"--dot"}) })
	if code != 0 || !strings.Contains(out, "digraph ssg") ||
		!strings.Contains(out, `"content/site/posts/a.md" -> "output/a/index.html"`) {
		t.Errorf("output = %q", out)
	}
}

func TestGraphRejectsAnUnknownOption(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	if code := runGraph([]string{"--nope"}); code != 2 {
		t.Errorf("code = %d", code)
	}
}

// TestGraphIsClaimedUnlessItLooksLikeABuild: `ssg graph` takes an optional
// path, but `ssg graph simple example.com` is a site whose source is named
// "graph".
func TestGraphIsClaimedUnlessItLooksLikeABuild(t *testing.T) {
	for _, args := range [][]string{
		{}, {"content/a.md"}, {"--json"}, {"--dot"}, {"--json", "content/a.md"},
	} {
		if !isGraphInvocation(args) {
			t.Errorf("graph %v should be the graph command", args)
		}
	}
	for _, args := range [][]string{
		{"simple", "example.com"}, {"simple", "example.com", "--clean"},
	} {
		if isGraphInvocation(args) {
			t.Errorf("graph %v is a positional build", args)
		}
	}
}

// TestGraphIsDispatchedWithNoArgument, which no verb+noun rule would catch.
func TestGraphIsDispatchedWithNoArgument(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	out, code := captureGraphOutput(t, func() int {
		c, handled := dispatchSubcommand([]string{"graph"})
		if !handled {
			t.Error("`ssg graph` fell through to a build")
		}
		return c
	})
	if code != 0 || !strings.Contains(out, "Dependency graph:") {
		t.Errorf("code = %d, output = %q", code, out)
	}

	out, _ = captureGraphOutput(t, func() int {
		c, handled := dispatchSubcommand([]string{"graph", "--json", "content/site/posts/a.md"})
		if !handled {
			t.Error("`ssg graph --json path` fell through to a build")
		}
		return c
	})
	if !strings.Contains(out, `"full": false`) {
		t.Errorf("output = %q", out)
	}

	if _, handled := dispatchSubcommand([]string{"graph", "simple", "example.com"}); handled {
		t.Error("a site whose source is named graph must still build")
	}
}

// TestProfilePageNamesWhatBuiltIt: `ssg profile page` used to say the
// dependency tree needed GO-094. Now it has one (GO-094).
func TestProfilePageNamesWhatBuiltIt(t *testing.T) {
	t.Chdir(t.TempDir())
	g := depgraph.New()
	g.AddInput("content/site/posts/a.md", depgraph.KindContent, "h")
	g.AddInput("data/authors.json", depgraph.KindData, "h")
	g.Depends("out/hello/index.html", "content/site/posts/a.md")
	g.Depends("out/hello/index.html", "data/authors.json")
	if err := g.Save(".ssg-cache/" + generator.GraphDirName); err != nil {
		t.Fatal(err)
	}
	out, _ := captureGraphOutput(t, func() int { printPageInputs("/hello/"); return 0 })
	if !strings.Contains(out, "content/site/posts/a.md") || !strings.Contains(out, "data/authors.json") {
		t.Errorf("output = %q", out)
	}
}

func TestProfilePageWithoutAGraphSaysWhereToGetOne(t *testing.T) {
	t.Chdir(t.TempDir())
	out, _ := captureGraphOutput(t, func() int { printPageInputs("/hello/"); return 0 })
	if !strings.Contains(out, "no dependency graph") {
		t.Errorf("output = %q", out)
	}
}

func TestProfilePageOnAGlobalBuildSaysEveryInput(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, true)
	out, _ := captureGraphOutput(t, func() int { printPageInputs("/hello/"); return 0 })
	if !strings.Contains(out, "every input") || !strings.Contains(out, "external sources") {
		t.Errorf("output = %q", out)
	}
}

func TestProfilePageForAnUnknownPageSaysSo(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	out, _ := captureGraphOutput(t, func() int { printPageInputs("/absent/"); return 0 })
	if !strings.Contains(out, "not in the last build's graph") {
		t.Errorf("output = %q", out)
	}
}

// TestGraphSaysAggregatesAreNotCounted: the number of outputs is not the number
// of files a build rewrites, and reading it that way would be wrong.
func TestGraphSaysAggregatesAreNotCounted(t *testing.T) {
	t.Chdir(t.TempDir())
	seedGraph(t, false)
	out, _ := captureGraphOutput(t, func() int { return runGraph([]string{"content/site/posts/a.md"}) })
	if !strings.Contains(out, "rebuilt on every build and are not counted here") {
		t.Errorf("output = %q", out)
	}
}
