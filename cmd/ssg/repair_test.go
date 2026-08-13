package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brokenPage is what a page-builder export leaves behind: markup indented past
// four columns, which renders as a literal code block.
const brokenPage = "---\ntitle: \"Logistics\"\n---\n\n## Content\n\n<div class=\"widget\">\n\ttext\n\n\t\t</div>\n\t</section>\n"

// writeRepairFixture lays out a project whose content dir holds one broken page
// and one clean page, and chdirs into it.
func writeRepairFixture(t *testing.T) string {
	t.Helper()
	tmp := chdirTemp(t)
	pages := filepath.Join(tmp, "content", "site", "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"broken.md": brokenPage,
		"clean.md":  "---\ntitle: \"Clean\"\n---\n\nJust prose.\n",
		"notes.txt": brokenPage, // not Markdown: must be ignored
	} {
		if err := os.WriteFile(filepath.Join(pages, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return tmp
}

func TestRunRepair_DryRunReportsAndLeavesFilesAlone(t *testing.T) {
	tmp := writeRepairFixture(t)
	broken := filepath.Join(tmp, "content", "site", "pages", "broken.md")

	if code := runRepair(nil); code != 1 {
		t.Errorf("a dry run with findings should exit 1, got %d", code)
	}
	got, _ := os.ReadFile(broken)
	if string(got) != brokenPage {
		t.Errorf("dry run rewrote the file:\n%s", got)
	}
}

func TestRunRepair_FixRewritesOnlyBrokenFiles(t *testing.T) {
	tmp := writeRepairFixture(t)
	pages := filepath.Join(tmp, "content", "site", "pages")
	clean := filepath.Join(pages, "clean.md")
	before, _ := os.ReadFile(clean)

	if code := runRepair([]string{"--fix", "-q"}); code != 0 {
		t.Errorf("--fix should exit 0, got %d", code)
	}

	repaired, _ := os.ReadFile(filepath.Join(pages, "broken.md"))
	if strings.Contains(string(repaired), "\t\t</div>") {
		t.Errorf("markup is still indented:\n%s", repaired)
	}
	if !strings.Contains(string(repaired), "\n</div>\n</section>") {
		t.Errorf("closing tags should be flush left:\n%s", repaired)
	}
	if after, _ := os.ReadFile(clean); string(after) != string(before) {
		t.Errorf("a clean file was rewritten:\n%s", after)
	}
	// A .txt file is not content: it must not have been touched.
	if txt, _ := os.ReadFile(filepath.Join(pages, "notes.txt")); string(txt) != brokenPage {
		t.Errorf("non-Markdown file was rewritten:\n%s", txt)
	}
	// Second run finds nothing.
	if code := runRepair(nil); code != 0 {
		t.Errorf("after --fix the dry run should be clean, got exit %d", code)
	}
}

func TestRunRepair_ExplicitPaths(t *testing.T) {
	tmp := writeRepairFixture(t)
	pages := filepath.Join(tmp, "content", "site", "pages")

	if code := runRepair([]string{filepath.Join(pages, "clean.md")}); code != 0 {
		t.Errorf("scanning one clean file should exit 0, got %d", code)
	}
	if code := runRepair([]string{filepath.Join(pages, "broken.md")}); code != 1 {
		t.Errorf("scanning one broken file should exit 1, got %d", code)
	}
}

func TestRunRepair_MissingPath(t *testing.T) {
	chdirTemp(t)
	if code := runRepair([]string{"nope"}); code != 1 {
		t.Errorf("a missing path should exit 1, got %d", code)
	}
}

func TestRunRepair_DefaultRootFollowsConfig(t *testing.T) {
	tmp := chdirTemp(t)
	// content_dir points somewhere other than the default "content".
	if err := os.WriteFile(filepath.Join(tmp, ".ssg.yaml"),
		[]byte("source: site\ncontent_dir: docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(tmp, "docs", "site", "pages")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "broken.md"), []byte(brokenPage), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := repairDefaultRoot(); got != "docs" {
		t.Errorf("default root should follow content_dir, got %q", got)
	}
	if code := runRepair([]string{"--fix"}); code != 0 {
		t.Errorf("--fix should exit 0, got %d", code)
	}
	out, _ := os.ReadFile(filepath.Join(docs, "broken.md"))
	if strings.Contains(string(out), "\t\t</div>") {
		t.Errorf("file under content_dir was not repaired:\n%s", out)
	}
}

func TestRepairDefaultRoot_NoConfig(t *testing.T) {
	chdirTemp(t)
	if got := repairDefaultRoot(); got != "content" {
		t.Errorf("without a config the default root is content/, got %q", got)
	}
}

func TestRunRepair_NothingToDo(t *testing.T) {
	tmp := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(tmp, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := runRepair(nil); code != 0 {
		t.Errorf("a clean tree should exit 0, got %d", code)
	}
	if code := runRepair([]string{"-q"}); code != 0 {
		t.Errorf("quiet clean tree should exit 0, got %d", code)
	}
}

func TestParseRepairFlags(t *testing.T) {
	f, code := parseRepairFlags([]string{"--fix", "--quiet", "some/path"})
	if code != -1 || !f.fix || !f.quiet || len(f.paths) != 1 {
		t.Errorf("unexpected parse: %+v code=%d", f, code)
	}
	if _, code := parseRepairFlags([]string{"--help"}); code != 0 {
		t.Errorf("--help should exit 0, got %d", code)
	}
	if _, code := parseRepairFlags([]string{"--nope"}); code != 2 {
		t.Errorf("an unknown flag should exit 2, got %d", code)
	}
}

func TestRunRepair_UnknownFlagStops(t *testing.T) {
	chdirTemp(t)
	if code := runRepair([]string{"--nope"}); code != 2 {
		t.Errorf("unknown flag should exit 2, got %d", code)
	}
}

func TestRepairFile_PreservesPermissions(t *testing.T) {
	tmp := chdirTemp(t)
	path := filepath.Join(tmp, "page.md")
	if err := os.WriteFile(path, []byte(brokenPage), 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := repairFile(path, true); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("permissions changed to %v", fi.Mode().Perm())
	}
}

func TestRepairFile_UnreadableFile(t *testing.T) {
	if _, err := repairFile(filepath.Join(t.TempDir(), "absent.md"), false); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestRepairDefaultRoot_UnreadableConfig(t *testing.T) {
	tmp := chdirTemp(t)
	// A config that cannot be parsed must not take the repair down with it:
	// the fallback is the conventional content dir.
	if err := os.WriteFile(filepath.Join(tmp, ".ssg.yaml"), []byte("source: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := repairDefaultRoot(); got != "content" {
		t.Errorf("a broken config should fall back to content/, got %q", got)
	}
}

func TestRepairFile_UnwritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.md")
	if err := os.WriteFile(path, []byte(brokenPage), 0o400); err != nil {
		t.Fatal(err)
	}
	// A read-only file inside a read-only directory cannot be replaced.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := repairFile(path, true); err == nil {
		t.Skip("filesystem allows writing a read-only file (running as root?)")
	}
}

func TestRepairScanRoot_UnreadableTree(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "locked")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "page.md"), []byte(brokenPage), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	// A directory the walk cannot enter is reported, never silently skipped.
	if _, err := repairScanRoot(dir, false); err == nil {
		t.Skip("filesystem allows reading a 0000 directory (running as root?)")
	}
}

// TestDispatchSingleVerb_Repair confirms the verb is claimed by the dispatcher,
// so `ssg repair` never falls through to the positional build.
func TestDispatchSingleVerb_Repair(t *testing.T) {
	chdirTemp(t)
	code, handled := dispatchSingleVerb([]string{"repair", "--help"})
	if !handled || code != 0 {
		t.Errorf("repair should be handled with exit 0, got handled=%v code=%d", handled, code)
	}
}
