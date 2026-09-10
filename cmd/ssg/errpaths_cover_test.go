package main

// What the commands do when the filesystem says no.
//
// These are the paths nobody exercises by hand and everybody hits eventually:
// a project directory that is read-only, a cache namespace that is a file where
// a directory should be, a config the operator cannot write back. Each of them
// has to report and stop, because the alternative — carrying on and printing a
// success — is how a failed write becomes a mystery an hour later.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ecovReadOnlyDir makes a directory nothing can be written into, and puts it
// back at the end so the temp dir can be removed.
func ecovReadOnlyDir(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, info.Mode()) })
}

// TestInitReportsAScaffoldItCannotWrite rather than claiming it created files.
func TestInitReportsAScaffoldItCannotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ecovReadOnlyDir(t, dir)
	out := captureStderr(t, func() {
		if code := runInit(nil); code != 1 {
			t.Errorf("code = %d", code)
		}
	})
	if out == "" {
		t.Error("a failed scaffold must say so")
	}
}

// TestInitRejectsAFlagItDoesNotKnow, with the usage line, because a mistyped
// --domian would otherwise scaffold a site with the wrong one.
func TestInitRejectsAFlagItDoesNotKnow(t *testing.T) {
	t.Chdir(t.TempDir())
	out := captureStderr(t, func() {
		if code := runInit([]string{"--domian=example.com"}); code == 0 {
			t.Error("an unknown flag must not scaffold anything")
		}
	})
	if !strings.Contains(out, "usage: ssg init") {
		t.Errorf("stderr = %q", out)
	}
	if _, err := os.Stat(".ssg.yaml"); err == nil {
		t.Error("nothing should have been written")
	}
}

// TestInitTakesADomainInEitherSpelling: `--domain x` and `--domain=x` are the
// same request, and a scaffold with the wrong canonical host is a site that
// has to be rebuilt.
func TestInitTakesADomainInEitherSpelling(t *testing.T) {
	for _, args := range [][]string{
		{"blog", "--domain", "example.com"},
		{"blog", "--domain=example.com"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Chdir(t.TempDir())
			if code := runInit(args); code != 0 {
				t.Fatalf("code = %d", code)
			}
			body, err := os.ReadFile(".ssg.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "example.com") {
				t.Errorf("config = %q", body)
			}
		})
	}
}

// TestCacheStatsReportsANamespaceItCannotRead and keeps going: one broken
// namespace must not hide the sizes of the others.
func TestCacheStatsReportsANamespaceItCannotRead(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// A file where the images cache directory should be.
	if err := os.MkdirAll(".ssg-cache", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".ssg-cache", "images"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runCache([]string{"stats"}); code != 0 {
		t.Errorf("code = %d — one unreadable namespace must not fail the command", code)
	}
}

// TestCacheCleanReportsWhatItCouldNotRemove: "cleaned" printed over a failed
// removal is worse than the failure.
func TestCacheCleanReportsWhatItCouldNotRemove(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	nested := filepath.Join(".ssg-cache", "images", "aa")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "entry"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ecovReadOnlyDir(t, filepath.Join(".ssg-cache", "images"))
	out := captureStderr(t, func() {
		if code := runCache([]string{"clean", "--namespace=images"}); code != 1 {
			t.Errorf("code = %d", code)
		}
	})
	if !strings.Contains(out, "images") {
		t.Errorf("stderr = %q", out)
	}
}

// TestConfigSetReportsAFileItCannotWriteBack.
//
// The edit is validated in a sibling file before the original is touched, so a
// failure here is the write itself — and the config on disk must still be the
// one that was there.
func TestConfigSetReportsAFileItCannotWriteBack(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, ".ssg.yaml")
	original := "domain: example.com\ntemplate: simple\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	ecovReadOnlyDir(t, dir)
	out := captureStderr(t, func() {
		if code := runConfig([]string{"set", "template", "krowy"}); code != 1 {
			t.Errorf("code = %d", code)
		}
	})
	if out == "" {
		t.Error("a failed write must say so")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != original {
		t.Errorf("the config was changed despite the failure:\n%s", body)
	}
}
