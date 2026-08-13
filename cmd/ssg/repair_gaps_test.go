package main

// Failure paths of `ssg repair` and of the migration's config completion
// (1.8.31). Both write to files the user owns, so a refusal must be reported
// or silent-but-harmless — never a partial rewrite.

import (
	"os"
	"path/filepath"
	"testing"
)

// brokenMarkdown is what a page-builder export leaves behind: markup indented
// far enough that CommonMark prints it to the visitor as a code block.
const brokenMarkdown = "---\ntitle: T\n---\n\n\t<div class=\"x\">\n\t\t<p>hi</p>\n\t</div>\n"

// TestRepairFileUnwritable: a file that cannot be rewritten fails loudly
// instead of reporting a repair that never landed.
func TestRepairFileUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "page.md")
	if err := os.WriteFile(path, []byte(brokenMarkdown), 0o644); err != nil {
		t.Fatal(err)
	}
	// The rewrite truncates the file in place, so the FILE has to be read-only
	// (a read-only directory still allows writing an existing file).
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	if _, err := repairFile(path, true); err == nil {
		t.Fatal("an unwritable target must be reported, not swallowed")
	}
	// The dry run over the same file still works: it only reads.
	res, err := repairFile(path, false)
	if err != nil || len(res.findings) == 0 {
		t.Fatalf("dry run must still report: %v %+v", err, res)
	}
}

// TestRepairScanRootPropagatesFileErrors: a scan that hits an unreadable file
// stops with that error rather than reporting a clean tree.
func TestRepairScanRootPropagatesFileErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "page.md")
	if err := os.WriteFile(path, []byte(brokenMarkdown), 0o200); err != nil { // write-only
		t.Fatal(err)
	}
	if _, err := repairScanRoot(dir, false); err == nil {
		t.Fatal("an unreadable file must fail the scan")
	}
}

// TestApplyMigratedIdentityUnwritableConfig: when the config cannot be written
// the migration reports nothing applied — it must not claim keys it failed to
// set, and it must not fail the migration that already succeeded.
func TestApplyMigratedIdentityUnwritableConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".ssg.yaml", []byte("source: s\ntemplate: simple\ndomain: e.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"title":"Magna Valor","description":"Advisory","marketing":{"colors":{"primary":"#123456"}}}`
	if err := os.MkdirAll(filepath.Join("content", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("content", "s", "metadata.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(".ssg.yaml", 0o444); err != nil { // config itself read-only
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, ".ssg.yaml"), 0o644) })

	if applied := applyMigratedIdentity(".ssg.yaml", filepath.Join("content", "s")); applied != nil {
		t.Fatalf("a failed write must report nothing applied, got %v", applied)
	}
}
