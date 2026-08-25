package generator

// Posts sitting directly in posts/ (#211).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// postsSite writes a project with one post flat in posts/ and one in a folder.
func postsSite(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"media":[],"users":[]}`)
	post := func(title, slug string) string {
		return "---\ntitle: " + title + "\nslug: " + slug +
			"\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [1]\n---\n\nBody.\n"
	}
	mustWrite(t, filepath.Join(contentDir, "posts", "flat.md"), post("Flat", "flat"))
	mustWrite(t, filepath.Join(contentDir, "posts", "news", "nested.md"), post("Nested", "nested"))
	return tmp
}

// buildPosts generates and returns the loaded posts.
func buildPosts(t *testing.T, tmp string, flat bool) []string {
	t.Helper()
	gen, err := New(Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true, FlatPosts: flat,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var slugs []string
	for _, p := range gen.siteData.Posts {
		slugs = append(slugs, p.Slug)
	}
	return slugs
}

// TestFlatPostsAreSkippedByDefault. They always have been, and loading them
// unasked would publish a page nobody asked to publish — worse than the bug.
func TestFlatPostsAreSkippedByDefault(t *testing.T) {
	got := buildPosts(t, postsSite(t), false)
	if len(got) != 1 || got[0] != "nested" {
		t.Errorf("posts = %v, want only the nested one", got)
	}
}

// TestFlatPostsLoadWhenAsked — the reported case, with the switch on.
func TestFlatPostsLoadWhenAsked(t *testing.T) {
	got := buildPosts(t, postsSite(t), true)
	if len(got) != 2 {
		t.Fatalf("posts = %v, want both", got)
	}
	var seen = map[string]bool{}
	for _, s := range got {
		if seen[s] {
			t.Fatalf("%q loaded twice — the flat read must not descend into the folders "+
				"loadPostsDir already walks", s)
		}
		seen[s] = true
	}
	if !seen["flat"] || !seen["nested"] {
		t.Errorf("posts = %v", got)
	}
}

// TestTheSkipIsAnnouncedByName: pages at the top level of pages/ load fine, so
// a skipped post reads as a frontmatter problem. The reporter bisected id,
// link, date quoting and category shapes before finding it was folder depth.
func TestTheSkipIsAnnouncedByName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "flat.md"), "---\ntitle: T\n---\n")
	mustWrite(t, filepath.Join(dir, "notes", "deep.md"), "---\ntitle: D\n---\n")

	out := captureGeneratorStdout(t, func() { warnSkippedFlatPosts(dir, false) })
	if !strings.Contains(out, "flat.md") {
		t.Errorf("the file must be named: %q", out)
	}
	if strings.Contains(out, "deep.md") {
		t.Errorf("a file in a folder is loaded and must not be reported: %q", out)
	}
	// Both remedies, because either is a reasonable choice.
	if !strings.Contains(out, "flat_posts: true") || !strings.Contains(out, "folder under posts/") {
		t.Errorf("the warning must name both remedies: %q", out)
	}
	// Silent under --quiet, like every other line of a build (#194).
	if out := captureGeneratorStdout(t, func() { warnSkippedFlatPosts(dir, true) }); out != "" {
		t.Errorf("--quiet must print nothing, got %q", out)
	}
}

// TestNothingIsSaidWhenThereIsNothingToSay: a site whose posts are all in
// folders must not learn about a rule it already follows.
func TestNothingIsSaidWhenThereIsNothingToSay(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "news", "a.md"), "---\ntitle: A\n---\n")
	mustWrite(t, filepath.Join(dir, "README.txt"), "not markdown")

	if out := captureGeneratorStdout(t, func() { warnSkippedFlatPosts(dir, false) }); out != "" {
		t.Errorf("nothing to warn about, got %q", out)
	}
	// A posts/ that does not exist at all is an ordinary site, not a problem.
	if out := captureGeneratorStdout(t, func() {
		warnSkippedFlatPosts(filepath.Join(dir, "absent"), false)
	}); out != "" {
		t.Errorf("a missing directory must be silent, got %q", out)
	}
}

// TestAFlatDraftIsStillNotPublished: the switch changes where the loader looks,
// not what it publishes. This is what makes turning it on safe — the only files
// that appear are ones that already said `status: publish`.
func TestAFlatDraftIsStillNotPublished(t *testing.T) {
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "posts", "draft.md"),
		"---\ntitle: Draft\nslug: draft\nstatus: draft\ntype: post\ndate: 2024-01-02\n---\n\nBody.\n")

	if got := buildPosts(t, tmp, true); len(got) != 0 {
		t.Errorf("a draft must stay unpublished wherever it sits: %v", got)
	}
}

// TestFlatPostFilesReadsOnlyMarkdownAtTheTop.
func TestFlatPostFilesReadsOnlyMarkdownAtTheTop(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.md", "a.md", "notes.txt", "img.png"} {
		mustWrite(t, filepath.Join(dir, name), "x")
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "c.md"), "x")

	got := flatPostFiles(dir)
	if len(got) != 2 || got[0] != "a.md" || got[1] != "b.md" {
		t.Errorf("files = %v, want a.md and b.md, sorted", got)
	}
	if flatPostFiles(filepath.Join(dir, "absent")) != nil {
		t.Error("a missing directory has no files")
	}
}

// TestAMissingPostsDirectoryIsAnOrdinarySite, not an error: a site with only
// pages has no posts/ at all.
func TestAMissingPostsDirectoryIsAnOrdinarySite(t *testing.T) {
	g := &Generator{config: Config{FlatPosts: true, Quiet: true}}
	if pages := g.loadMarkdownFiles(t.TempDir(), nil); len(pages) != 0 {
		t.Errorf("pages = %v", pages)
	}
	posts, err := g.loadPostsDir(filepath.Join(t.TempDir(), "absent"))
	if err != nil || len(posts) != 0 {
		t.Errorf("posts=%v err=%v", posts, err)
	}
}

// TestAnUnreadablePostsDirectoryIsReported rather than silently empty — the
// silence is what #211 was about, and a permissions problem must not inherit it.
func TestAnUnreadablePostsDirectoryIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	dir := filepath.Join(t.TempDir(), "posts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "a.md"), "---\ntitle: A\nstatus: publish\n---\n")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	g := &Generator{config: Config{FlatPosts: true, Quiet: true}}
	if _, err := g.loadPostsDir(dir); err == nil {
		t.Error("an unreadable posts/ must be an error, not a silent zero")
	}
}
