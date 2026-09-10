package mcp

// site_dependencies: what a page was built from, and what a file change
// rebuilds (GO-095 phase 3).

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/depgraph"
)

// depServer is a server whose project has a dependency graph from one build.
func depServer(t *testing.T, global bool) *Server {
	t.Helper()
	root := t.TempDir()
	g := depgraph.New()
	g.AddInput("content/site/posts/hello.md", depgraph.KindContent, "h1")
	g.AddInput("templates/theme/post.html", depgraph.KindTemplate, "h2")
	g.AddInput("data/authors.json", depgraph.KindData, "h3")
	g.AddInput(".ssg.yaml", depgraph.KindConfig, "h4")
	g.Depends("output/hello/index.html", "content/site/posts/hello.md")
	g.Depends("output/hello/index.html", "data/authors.json")
	g.Depends("output/other/index.html", "data/authors.json")
	if global {
		g.MarkGlobal("external sources can change without a file changing")
	}
	if err := g.Save(filepath.Join(root, ".ssg-cache", GraphDirName)); err != nil {
		t.Fatal(err)
	}
	return NewServer(Options{Root: root, Roles: map[string]bool{"content": true}})
}

func TestDependenciesWithoutAGraphSaysWhereItComesFrom(t *testing.T) {
	s := NewServer(Options{Root: t.TempDir()})
	r := s.siteDependencies(map[string]any{"url": "/hello/"})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "run a build first") {
		t.Errorf("result = %+v", r)
	}
}

func TestDependenciesOverviewSaysWhetherBuildsCanBeNarrowed(t *testing.T) {
	m := decode(t, depServer(t, false).siteDependencies(map[string]any{}))
	if m["inputs"].(float64) != 4 || m["outputs"].(float64) != 2 || m["edges"].(float64) != 3 {
		t.Errorf("overview = %v", m)
	}
	narrowable := m["narrowable"].(map[string]any)
	if narrowable["yes"] != true {
		t.Errorf("narrowable = %v", narrowable)
	}

	m = decode(t, depServer(t, true).siteDependencies(map[string]any{}))
	narrowable = m["narrowable"].(map[string]any)
	if narrowable["yes"] != false {
		t.Errorf("narrowable = %v", narrowable)
	}
	reasons := narrowable["reasons"].([]any)
	if len(reasons) != 1 || !strings.Contains(reasons[0].(string), "external sources") {
		t.Errorf("reasons = %v", reasons)
	}
}

func TestDependenciesForAPageNamesItsInputsByKind(t *testing.T) {
	m := decode(t, depServer(t, false).siteDependencies(map[string]any{"url": "/hello/"}))
	if m["output"] != "output/hello/index.html" {
		t.Errorf("output = %v", m["output"])
	}
	inputs := m["inputs"].(map[string]any)
	content := inputs["content"].([]any)
	if len(content) != 1 || content[0] != "content/site/posts/hello.md" {
		t.Errorf("content = %v", content)
	}
	data := inputs["data"].([]any)
	if len(data) != 1 || data[0] != "data/authors.json" {
		t.Errorf("data = %v", data)
	}
	if _, ok := inputs["template"]; ok {
		t.Error("a template with no edge to this page should not be claimed as one of its inputs")
	}
}

// TestDependenciesAcceptsTheURLShapesAPersonTypes.
func TestDependenciesAcceptsTheURLShapesAPersonTypes(t *testing.T) {
	s := depServer(t, false)
	for _, url := range []string{"/hello/", "hello", "/hello", "/hello/index.html"} {
		m := decode(t, s.siteDependencies(map[string]any{"url": url}))
		if m["output"] != "output/hello/index.html" {
			t.Errorf("%q → %v", url, m["output"])
		}
	}
}

func TestDependenciesForAnUnknownPagePointsSomewhereUseful(t *testing.T) {
	r := depServer(t, false).siteDependencies(map[string]any{"url": "/absent/"})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "site_pages") {
		t.Errorf("result = %+v", r)
	}
}

func TestDependenciesForAFileListsWhatItRebuilds(t *testing.T) {
	m := decode(t, depServer(t, false).siteDependencies(map[string]any{"path": "data/authors.json"}))
	if m["full"] != false || m["count"].(float64) != 2 {
		t.Errorf("plan = %v", m)
	}
	outputs := m["outputs"].([]any)
	if outputs[0] != "output/hello/index.html" || outputs[1] != "output/other/index.html" {
		t.Errorf("outputs = %v", outputs)
	}
}

// TestDependenciesForAFileExplainsAFullRebuild rather than listing every page.
func TestDependenciesForAFileExplainsAFullRebuild(t *testing.T) {
	s := depServer(t, false)
	for _, tc := range []struct{ path, want string }{
		{"templates/theme/post.html", "is a template"},
		{".ssg.yaml", "the configuration"},
		{"content/site/posts/new.md", "new to this build"},
	} {
		m := decode(t, s.siteDependencies(map[string]any{"path": tc.path}))
		if m["full"] != true {
			t.Errorf("%s: plan = %v", tc.path, m)
			continue
		}
		if !strings.Contains(m["reason"].(string), tc.want) {
			t.Errorf("%s: reason = %v", tc.path, m["reason"])
		}
		if _, ok := m["outputs"]; ok {
			t.Errorf("%s: a full rebuild should not list outputs", tc.path)
		}
	}
}

// TestDependenciesWindowsPathSeparators: a caller on Windows types backslashes,
// the graph stores forward slashes.
func TestDependenciesNormalisesPathSeparators(t *testing.T) {
	m := decode(t, depServer(t, false).siteDependencies(
		map[string]any{"path": filepath.FromSlash("data/authors.json")}))
	if m["full"] != false {
		t.Errorf("plan = %v", m)
	}
}

func TestDependenciesRefusesBothQuestionsAtOnce(t *testing.T) {
	r := depServer(t, false).siteDependencies(map[string]any{"url": "/hello/", "path": "data/authors.json"})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "not both") {
		t.Errorf("result = %+v", r)
	}
}

// TestDependenciesReadsAnExplicitCacheDir, for a project whose cache is not
// beside its content.
func TestDependenciesReadsAnExplicitCacheDir(t *testing.T) {
	cache := t.TempDir()
	g := depgraph.New()
	g.AddInput("content/a.md", depgraph.KindContent, "h")
	g.Depends("out/a/index.html", "content/a.md")
	if err := g.Save(filepath.Join(cache, GraphDirName)); err != nil {
		t.Fatal(err)
	}
	s := NewServer(Options{Root: t.TempDir(), CacheDir: cache})
	m := decode(t, s.siteDependencies(map[string]any{"url": "/a/"}))
	if m["output"] != "out/a/index.html" {
		t.Errorf("output = %v", m["output"])
	}
}

// TestDependenciesOnAGlobalBuildCarriesTheCaveat: the edges recorded are fewer
// than the real ones, and an agent has to be told.
func TestDependenciesOnAGlobalBuildCarriesTheCaveat(t *testing.T) {
	m := decode(t, depServer(t, true).siteDependencies(map[string]any{"url": "/hello/"}))
	global := m["global"].([]any)
	if len(global) != 1 {
		t.Errorf("global = %v", global)
	}
	if !strings.Contains(m["caveat"].(string), "fewer") {
		t.Errorf("caveat = %v", m["caveat"])
	}
}

func TestDependencyToolIsDescribedForAnAgent(t *testing.T) {
	tools := depServer(t, false).dependencyTools()
	if len(tools) != 1 || tools[0].name != "site_dependencies" {
		t.Fatalf("tools = %+v", tools)
	}
	if !strings.Contains(tools[0].description, "url") || !strings.Contains(tools[0].description, "path") {
		t.Errorf("description = %q", tools[0].description)
	}
	props := tools[0].schema.(map[string]any)["properties"].(map[string]any)
	if _, ok := props["url"]; !ok {
		t.Error("url is not in the schema")
	}
	if _, ok := props["path"]; !ok {
		t.Error("path is not in the schema")
	}
}
