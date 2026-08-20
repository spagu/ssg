package generator

// Markdown the build never read (#168).
//
// A build reads pages/ and posts/ and nothing else, so a products/ directory an
// export left behind is not skipped so much as never looked at. The silence is
// what cost the reporter an afternoon.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sourceTree builds a source directory with the given files, each an empty
// Markdown document unless the name says otherwise.
func sourceTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("---\ntitle: x\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestUnreadDirectoriesAreFound: the reported shape — documents in a directory
// no step reads.
func TestUnreadDirectoriesAreFound(t *testing.T) {
	g := newTestGen(t, "")
	src := sourceTree(t,
		"pages/a.md", "posts/blog/b.md",
		"products/one.md", "products/two.md",
		"drafts/c.md",
		"media/photo.jpg", // not Markdown: nothing to read there
	)

	dirs := g.unreadContentDirs(src)
	got := map[string]int{}
	for _, d := range dirs {
		got[d.Path] = d.Files
	}
	if got["products"] != 2 {
		t.Errorf("products = %d, want 2", got["products"])
	}
	if got["drafts"] != 1 {
		t.Errorf("drafts = %d, want 1", got["drafts"])
	}
	for _, read := range []string{"pages", "posts", "media"} {
		if _, reported := got[read]; reported {
			t.Errorf("%s is read (or holds no Markdown) and must not be reported", read)
		}
	}
	// Sorted, so two builds report the same way.
	if len(dirs) == 2 && dirs[0].Path > dirs[1].Path {
		t.Errorf("the report must be ordered: %+v", dirs)
	}
}

// TestConfiguredContentSourcesAreNotUnread: naming a directory in the config is
// the fix this report suggests, so it must silence the report.
func TestConfiguredContentSourcesAreNotUnread(t *testing.T) {
	g := newTestGen(t, "")
	src := sourceTree(t, "pages/a.md", "products/one.md")
	g.config.ContentSources = []ContentSource{{Path: filepath.Join(src, "products"), Type: "page"}}

	if dirs := g.unreadContentDirs(src); len(dirs) != 0 {
		t.Errorf("a configured source is read: %+v", dirs)
	}
	// A source pointing deeper still accounts for its top-level directory.
	g.config.ContentSources = []ContentSource{{Path: filepath.Join(src, "products", "sub"), Type: "page"}}
	if dirs := g.unreadContentDirs(src); len(dirs) != 0 {
		t.Errorf("a deeper source still accounts for the directory: %+v", dirs)
	}
	// One pointing outside the source tree accounts for nothing here.
	g.config.ContentSources = []ContentSource{{Path: t.TempDir(), Type: "page"}}
	if dirs := g.unreadContentDirs(src); len(dirs) != 1 {
		t.Errorf("an unrelated source must not silence the report: %+v", dirs)
	}
}

// TestTheReportNamesTheCauseAndTheFix: the reporter concluded that `type:` had
// excluded the documents, which is wrong and expensive to act on — editing 282
// files fixes nothing. The report says so.
func TestTheReportNamesTheCauseAndTheFix(t *testing.T) {
	g := newTestGen(t, "")
	src := sourceTree(t, "pages/a.md", "products/one.md")

	out := captureBuildOutput(t, func() { g.reportUnreadContent(src) })
	for _, want := range []string{"products/", "content_sources:", "1 file(s)", "`type:` is not what excluded it"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report must contain %q:\n%s", want, out)
		}
	}
	// A quiet build says nothing, and neither does a clean tree.
	g.config.Quiet = true
	if out := captureBuildOutput(t, func() { g.reportUnreadContent(src) }); out != "" {
		t.Errorf("a quiet build printed %q", out)
	}
	g.config.Quiet = false
	clean := sourceTree(t, "pages/a.md")
	if out := captureBuildOutput(t, func() { g.reportUnreadContent(clean) }); out != "" {
		t.Errorf("a clean tree printed %q", out)
	}
	// No source at all is not a report either.
	if out := captureBuildOutput(t, func() { g.reportUnreadContent("") }); out != "" {
		t.Errorf("an empty source printed %q", out)
	}
}

// TestCountMarkdownCountsRecursively, and counts only Markdown.
func TestCountMarkdown(t *testing.T) {
	root := sourceTree(t, "a.md", "deep/b.md", "deep/deeper/c.md")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := countMarkdown(root); got != 3 {
		t.Errorf("countMarkdown = %d, want 3", got)
	}
	if got := countMarkdown(filepath.Join(root, "nope")); got != 0 {
		t.Errorf("a missing directory counts zero, got %d", got)
	}
}

// TestUnreadDirsOnAnUnreadableSource is not an error: a report must never be
// the thing that fails a build.
func TestUnreadDirsOnAnUnreadableSource(t *testing.T) {
	g := newTestGen(t, "")
	if dirs := g.unreadContentDirs(filepath.Join(t.TempDir(), "missing")); dirs != nil {
		t.Errorf("dirs = %+v, want nothing", dirs)
	}
}

// TestLoadedContentRootsHonoursCustomPaths, since pages_path/posts_path can be
// renamed and the report must follow.
func TestLoadedContentRoots(t *testing.T) {
	g := newTestGen(t, "")
	g.config.PagesPath = "strony"
	g.config.PostsPath = "wpisy"
	roots := g.loadedContentRoots(t.TempDir())

	for _, want := range []string{"strony", "wpisy", "media"} {
		if !roots[want] {
			t.Errorf("%q must count as read", want)
		}
	}
	if roots["pages"] {
		t.Error("the default name is not read when the config renamed it")
	}
}
