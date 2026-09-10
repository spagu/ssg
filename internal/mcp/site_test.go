package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/sitegraph"
)

func siteServer(t *testing.T) (*Server, string) {
	t.Helper()
	out := t.TempDir()
	g := sitegraph.Graph{Schema: sitegraph.Schema, Domain: "example.com",
		Build: sitegraph.Build{Version: "1.8.60", Time: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Hash: "abc"},
		Pages: []sitegraph.Page{
			{URL: "/a/", Type: "post", Title: "A", Lang: "en"},
			{URL: "/b/", Type: "page", Title: "B", Lang: "en"},
			{URL: "/pl/c/", Type: "post", Title: "C", Lang: "pl"},
		},
		Links:      []sitegraph.Link{{From: "/a/", To: "/b/", Kind: "page"}, {From: "/b/", To: "/a/", Kind: "page"}, {From: "/a/", To: "https://x.org", Kind: "external"}},
		Taxonomies: []sitegraph.Taxonomy{{Name: "tag", Path: "tag", Terms: []sitegraph.Term{{Name: "Go", Slug: "go", URL: "/tag/go/", Count: 2}}}},
		Redirects:  []sitegraph.Redirect{{From: "/old/", To: "/a/", Status: 301}},
	}
	if _, err := sitegraph.Write(out, g, 0); err != nil {
		t.Fatal(err)
	}
	return NewServer(Options{Root: t.TempDir(), OutputDir: out, Roles: map[string]bool{"content": true}}), out
}

func decode(t *testing.T, r toolResult) map[string]any {
	t.Helper()
	if r.IsError {
		t.Fatalf("tool error: %s", r.Content[0].Text)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(r.Content[0].Text), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.Content[0].Text)
	}
	return m
}

// TestSiteToolsAreRegisteredWithAnOutputDir: the section appears for every
// role when the server knows where the build writes, and not otherwise.
func TestSiteToolsAreRegisteredWithAnOutputDir(t *testing.T) {
	s, _ := siteServer(t)
	names := map[string]bool{}
	for _, tl := range s.buildTools() {
		names[tl.name] = true
	}
	for _, want := range []string{"site_pages", "site_page", "site_links", "site_taxonomies", "site_redirects"} {
		if !names[want] {
			t.Errorf("%s not registered", want)
		}
	}
	bare := NewServer(Options{Root: t.TempDir(), Roles: map[string]bool{"content": true}})
	for _, tl := range bare.buildTools() {
		if strings.HasPrefix(tl.name, "site_") {
			t.Errorf("%s registered without an output dir", tl.name)
		}
	}
}

// TestSitePagesFiltersAndWindows: type/lang narrow, limit/offset page, and the
// build stamp rides along.
func TestSitePagesFiltersAndWindows(t *testing.T) {
	s, _ := siteServer(t)
	m := decode(t, s.sitePages(map[string]any{"type": "post"}))
	pages := m["pages"].(map[string]any)
	if pages["total"].(float64) != 2 {
		t.Errorf("post filter: %v", pages)
	}
	if m["build"].(map[string]any)["hash"] != "abc" {
		t.Errorf("build stamp missing: %v", m["build"])
	}
	m = decode(t, s.sitePages(map[string]any{"lang": "pl"}))
	if m["pages"].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("lang filter: %v", m["pages"])
	}
	m = decode(t, s.sitePages(map[string]any{"limit": float64(1), "offset": float64(1)}))
	items := m["pages"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["url"] != "/b/" {
		t.Errorf("window: %v", items)
	}
	// An offset past the end is an empty page, not a panic.
	m = decode(t, s.sitePages(map[string]any{"offset": float64(99)}))
	if len(m["pages"].(map[string]any)["items"].([]any)) != 0 {
		t.Error("offset past the end returned items")
	}
}

// TestSitePageWithLinks: one page, with what points at it and what it points to.
func TestSitePageWithLinks(t *testing.T) {
	s, _ := siteServer(t)
	m := decode(t, s.sitePage(map[string]any{"url": "/a/"}))
	page := m["page"].(map[string]any)
	if len(page["links_out"].([]any)) != 2 || len(page["links_in"].([]any)) != 1 {
		t.Errorf("links: %v", page)
	}
	if r := s.sitePage(map[string]any{"url": "/nope/"}); !r.IsError || !strings.Contains(r.Content[0].Text, "site_pages") {
		t.Errorf("unknown page should point at site_pages: %+v", r)
	}
}

// TestSiteLinksTaxonomiesRedirects: the remaining three, plus the filters.
func TestSiteLinksTaxonomiesRedirects(t *testing.T) {
	s, _ := siteServer(t)
	m := decode(t, s.siteLinks(map[string]any{"kind": "external"}))
	if m["links"].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("kind filter: %v", m["links"])
	}
	m = decode(t, s.siteLinks(map[string]any{"from": "/a/", "to": "/b/"}))
	if m["links"].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("from/to filter: %v", m["links"])
	}
	m = decode(t, s.siteTaxonomies(nil))
	if tax := m["taxonomies"].([]any); len(tax) != 1 || tax[0].(map[string]any)["name"] != "tag" {
		t.Errorf("taxonomies: %v", tax)
	}
	m = decode(t, s.siteRedirects(nil))
	if red := m["redirects"].([]any); len(red) != 1 || red[0].(map[string]any)["status"].(float64) != 301 {
		t.Errorf("redirects: %v", red)
	}
}

// TestSiteGraphMissingIsANamedError: no artifact means "build with
// site_graph: true", not an empty answer; no output dir means the server was
// started without one.
func TestSiteGraphMissingIsANamedError(t *testing.T) {
	s := NewServer(Options{Root: t.TempDir(), OutputDir: t.TempDir(), Roles: map[string]bool{"content": true}})
	if r := s.sitePages(nil); !r.IsError || !strings.Contains(r.Content[0].Text, "site_graph: true") {
		t.Errorf("missing graph: %+v", r)
	}
	bare := NewServer(Options{Root: t.TempDir(), Roles: map[string]bool{"content": true}})
	if r := bare.siteRedirects(nil); !r.IsError || !strings.Contains(r.Content[0].Text, "output directory") {
		t.Errorf("no output dir: %+v", r)
	}
}

// TestSiteGraphCacheFollowsTheFile: a rebuild that changes the artifact is
// picked up; an unchanged one is not re-read.
func TestSiteGraphCacheFollowsTheFile(t *testing.T) {
	s, out := siteServer(t)
	first, _ := s.loadGraph()
	if len(first.Pages) != 3 {
		t.Fatal("setup")
	}
	g := first
	g.Pages = g.Pages[:1]
	// Ensure a different mtime even on a coarse filesystem clock.
	time.Sleep(20 * time.Millisecond)
	if _, err := sitegraph.Write(out, g, 0); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(filepath.Join(out, sitegraph.FileName), future, future)
	second, _ := s.loadGraph()
	if len(second.Pages) != 1 {
		t.Errorf("a rebuilt graph was not picked up: %d pages", len(second.Pages))
	}
	third, _ := s.loadGraph()
	if len(third.Pages) != 1 {
		t.Error("cache returned something else")
	}
}

// TestSiteGraphUnreadableIsAnError: a corrupt artifact, or a directory sitting
// where the file should be, is reported rather than answered as empty.
func TestSiteGraphUnreadableIsAnError(t *testing.T) {
	out := t.TempDir()
	s := NewServer(Options{Root: t.TempDir(), OutputDir: out, Roles: map[string]bool{"content": true}})
	if err := os.WriteFile(filepath.Join(out, sitegraph.FileName), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := s.sitePages(nil); !r.IsError {
		t.Errorf("corrupt graph must be an error: %+v", r)
	}
	if err := os.Remove(filepath.Join(out, sitegraph.FileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(out, sitegraph.FileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := s.siteTaxonomies(nil); !r.IsError {
		t.Errorf("a directory in the file's place must be an error: %+v", r)
	}
}

// TestGraphResultRefusesTheUnencodable: the answer envelope reports an
// encoding failure as a tool error instead of an empty text.
func TestGraphResultRefusesTheUnencodable(t *testing.T) {
	if r := graphResult(sitegraph.Build{}, "x", make(chan int)); !r.IsError {
		t.Errorf("unencodable value: %+v", r)
	}
}

// TestSiteComponentsInvertsTheQuestion: the graph stores components per page,
// and the question a component's author has is the other way round.
func TestSiteComponentsInvertsTheQuestion(t *testing.T) {
	out := t.TempDir()
	g := sitegraph.Graph{Schema: sitegraph.Schema, Domain: "example.com",
		Build: sitegraph.Build{Version: "1.8.60", Hash: "abc"},
		Pages: []sitegraph.Page{
			{URL: "/a/", Type: "page", Components: []string{"youtube", "note"}},
			{URL: "/b/", Type: "page", Components: []string{"youtube"}},
			{URL: "/c/", Type: "page"},
		},
	}
	if _, err := sitegraph.Write(out, g, 0); err != nil {
		t.Fatal(err)
	}
	s := NewServer(Options{Root: t.TempDir(), OutputDir: out, Roles: map[string]bool{"content": true}})

	got := decode(t, s.siteComponents(nil))
	list, _ := got["components"].([]any)
	if len(list) != 2 {
		t.Fatalf("components = %v", got["components"])
	}
	first, _ := list[0].(map[string]any)
	if first["name"] != "note" || first["count"] != float64(1) {
		t.Errorf("first = %v (want them sorted by name)", first)
	}
	second, _ := list[1].(map[string]any)
	if second["name"] != "youtube" || second["count"] != float64(2) {
		t.Errorf("second = %v", second)
	}

	// One component's name narrows the answer.
	only := decode(t, s.siteComponents(map[string]any{"name": "youtube"}))
	if list, _ := only["components"].([]any); len(list) != 1 {
		t.Errorf("narrowed = %v", only["components"])
	}
	// And the answer names the build it came from.
	if _, ok := got["build"]; !ok {
		t.Error("every site answer carries its build")
	}
	// Without a graph it says what to turn on.
	bare := NewServer(Options{Root: t.TempDir(), OutputDir: t.TempDir(), Roles: map[string]bool{"content": true}})
	if r := bare.siteComponents(nil); !r.IsError {
		t.Error("no graph, no answer")
	}
}
