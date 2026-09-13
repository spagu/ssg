package generator

// Markdown read but not published: no status line (#274), frontmatter that is
// not YAML (#279), and a source with no metadata.json (#277).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skippedFixture is a site with one published page, one deliberate draft, two
// files with no status line and one whose description carries a bare colon.
func skippedFixture(t *testing.T) Config {
	t.Helper()
	return newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/home.md":    "---\ntitle: Home\nstatus: publish\n---\n\nHi.\n",
		"pages/draft.md":   "---\ntitle: Draft\nstatus: draft\n---\n\nLater.\n",
		"pages/about.md":   "---\ntitle: About\n---\n\nNo status.\n",
		"pages/terms.md":   "---\ntitle: Terms\n---\n\nNo status.\n",
		"pages/shop.md":    "---\ntitle: Shop\ndescription: Sensors that feed verdicts: humidity.\nstatus: publish\n---\n\nBad.\n",
		"pages/plain.md":   "No frontmatter at all is published (GO-009), so it is not reported.\n",
		"posts/news/p1.md": "---\ntitle: P1\ntype: post\ndate: 2026-01-02\n---\n\nNo status either.\n",
	}, nil)
}

// buildSkipped builds the fixture and returns its output and error.
func buildSkipped(t *testing.T, cfg Config) (string, error) {
	t.Helper()
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var buildErr error
	out := captureGeneratorStdout(t, func() { buildErr = gen.Generate() })
	return out, buildErr
}

// TestFilesWithNoStatusAreNamed — #274. The build said "Loaded 1 pages" beside
// five files on disk and nothing else. It still publishes exactly what it did;
// it now names the files dropped for a line nobody chose to leave out.
func TestFilesWithNoStatusAreNamed(t *testing.T) {
	cfg := skippedFixture(t)
	cfg.Quiet = false
	out, err := buildSkipped(t, cfg)
	if err != nil {
		t.Fatalf("a missing status must not fail a build: %v", err)
	}
	for _, want := range []string{
		"2 Markdown file(s) in " + filepath.ToSlash(filepath.Join(cfg.ContentDir, "site", "pages")),
		"about.md, terms.md",
		"p1.md",
		"`status: publish`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	// A status deliberately set to something else was a decision.
	if strings.Contains(out, "draft.md") {
		t.Errorf("a deliberate draft must stay quiet:\n%s", out)
	}
	if strings.Contains(out, "plain.md") {
		t.Errorf("a file without frontmatter is published and must not be reported:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about")); err == nil {
		t.Error("a file with no status must still not be published")
	}
}

// TestUnparseableFrontmatterIsNamedWithTheCause — #279. YAML's "mapping values
// are not allowed in this context" is not an author's phrasing; the likely
// cause and the fix are.
func TestUnparseableFrontmatterIsNamedWithTheCause(t *testing.T) {
	cfg := skippedFixture(t)
	cfg.Quiet = false
	out, err := buildSkipped(t, cfg)
	if err != nil {
		t.Fatalf("outside strict an unparseable file must not fail the build: %v", err)
	}
	for _, want := range []string{
		"1 Markdown file(s) were not published — their frontmatter could not be parsed",
		"shop.md: yaml:",
		`an unquoted ":" inside a value? Wrap the whole value in double quotes.`,
		"Set `strict: true`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

// TestStrictFailsOnUnparseableFrontmatter: with exit status 0 a site missing a
// whole section ships, as soon as nothing happens to link to it.
func TestStrictFailsOnUnparseableFrontmatter(t *testing.T) {
	cfg := skippedFixture(t)
	cfg.Strict = true
	_, err := buildSkipped(t, cfg)
	if err == nil || !strings.Contains(err.Error(), "shop.md") || !strings.Contains(err.Error(), "strict") {
		t.Fatalf("strict must fail naming the file, got %v", err)
	}
}

// TestSkippedContentIsQuietUnderQuiet (#194), and a rebuild does not repeat the
// previous build's list.
func TestSkippedContentIsQuietUnderQuiet(t *testing.T) {
	cfg := skippedFixture(t)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := captureGeneratorStdout(t, func() {
		if err := gen.Generate(); err != nil {
			t.Errorf("Generate: %v", err)
		}
	})
	if strings.Contains(out, "not published") {
		t.Errorf("--quiet must print nothing about skipped files:\n%s", out)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	if n := len(gen.skipped.unparsed); n != 1 {
		t.Errorf("a rebuild must start its own list, got %d unparsed entries", n)
	}
}

// TestFrontmatterHint pins the causes named, and silence for the rest.
func TestFrontmatterHint(t *testing.T) {
	cases := map[string]string{
		"yaml: line 2: mapping values are not allowed in this context":    `unquoted ":"`,
		"yaml: line 1: found character that cannot start any token":       "reserved character",
		"yaml: unmarshal errors: cannot unmarshal !!seq into string":      "",
		"yaml: line 3: did not find expected key while parsing a mapping": "",
	}
	for msg, want := range cases {
		got := frontmatterHint(errors.New(msg))
		if want == "" && got != "" || !strings.Contains(got, want) {
			t.Errorf("%q: hint %q, want one containing %q", msg, got, want)
		}
	}
}

// TestAMissingMetadataFileBuildsAndSaysSo — #277. A hand-made site has no
// taxonomy metadata; its first build failed with a raw open error.
func TestAMissingMetadataFileBuildsAndSaysSo(t *testing.T) {
	cfg := newSiteFixture(t, `{}`, map[string]string{
		"pages/home.md": "---\ntitle: Home\nstatus: publish\n---\n\nHi.\n",
	}, nil)
	if err := os.Remove(filepath.Join(cfg.ContentDir, "site", "metadata.json")); err != nil {
		t.Fatal(err)
	}
	cfg.Quiet = false
	out, err := buildSkipped(t, cfg)
	if err != nil {
		t.Fatalf("a site without metadata.json must build: %v", err)
	}
	for _, want := range []string{"metadata.json not found", `{"categories":[],"users":[],"tags":[],"media":[]}`, "docs/CONTENT.md#metadatajson"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

// TestSourceDirError: a mistyped source must still fail, naming the key — an
// empty site built from nothing would be the silence #277 set out to end.
func TestSourceDirError(t *testing.T) {
	dir := t.TempDir()
	if err := sourceDirError(dir); err != nil {
		t.Errorf("an existing directory: %v", err)
	}
	if err := sourceDirError(filepath.Join(dir, "absent")); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("absent: %v", err)
	}
	file := filepath.Join(dir, "file")
	mustWrite(t, file, "x")
	if err := sourceDirError(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("a file: %v", err)
	}
}

// TestReadMetadataReportsOtherOpenErrors: only an absent file is an empty set.
// A path that cannot be opened for another reason is still an error.
func TestReadMetadataReportsOtherOpenErrors(t *testing.T) {
	g := newTestGen(t, "")
	notADir := filepath.Join(t.TempDir(), "file")
	mustWrite(t, notADir, "x")
	if _, err := g.readMetadata(filepath.Join(notADir, "metadata.json")); err == nil {
		t.Error("a path through a regular file must be reported, not treated as absent")
	}
}
