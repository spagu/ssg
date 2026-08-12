package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKeyerMatchesImagesFormula proves the Keyer reproduces the images cache's
// historical formula exactly: sha256(file bytes + ops JSON + "v"+version)[:10].
func TestKeyerMatchesImagesFormula(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "golden.png")
	content := []byte("golden-image-bytes-v1")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	opsJSON := []byte(`[{"op":"resize","width":320,"format":"webp","quality":70}]`)

	// Reference: the raw formula.
	h := sha256.New()
	h.Write(content)
	h.Write(opsJSON)
	h.Write([]byte("v1"))
	want := hex.EncodeToString(h.Sum(nil))[:10]

	k := NewKeyer("1", 10)
	if err := k.WriteFileContents(src); err != nil {
		t.Fatal(err)
	}
	k.Write(opsJSON)
	if got := k.Sum(); got != want {
		t.Fatalf("Keyer diverges from the historical formula: got %s want %s", got, want)
	}
}

func TestKeyerDelimPreventsCollisions(t *testing.T) {
	a := NewKeyer("", 0)
	a.WriteDelim("ab")
	a.WriteDelim("c")
	b := NewKeyer("", 0)
	b.WriteDelim("a")
	b.WriteDelim("bc")
	if a.Sum() == b.Sum() {
		t.Fatal("NUL-delimited parts must not collide")
	}
}

func TestKeyerVersionChangesKey(t *testing.T) {
	a := NewKeyer("1", 0)
	a.WriteString("x")
	b := NewKeyer("2", 0)
	b.WriteString("x")
	if a.Sum() == b.Sum() {
		t.Fatal("version must participate in the key")
	}
	c := NewKeyer("", 0)
	c.WriteString("x")
	if c.Sum() == a.Sum() {
		t.Fatal("empty version must omit the tag, not equal v1")
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ns")
	if err := WriteAtomicBytes(dir, "a.txt", 0o644, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("read back: %q %v", b, err)
	}
	// A failing fill leaves no file behind (atomicity).
	err = WriteAtomic(dir, "b.txt", 0o644, func(*os.File) error { return os.ErrInvalid })
	if err == nil {
		t.Fatal("fill error must propagate")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "a.txt" {
			t.Fatalf("unexpected leftover %q", e.Name())
		}
	}
	// Extension is preserved on the temp pattern (encoder tools sniff suffixes).
	if err := WriteAtomicBytes(dir, "img.webp", 0o644, []byte("x")); err != nil {
		t.Fatal(err)
	}
}

func TestDirStatsCleanGC(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ns")
	_ = WriteAtomicBytes(dir, "keep.txt", 0o644, []byte("12345"))
	_ = WriteAtomicBytes(dir, "drop.txt", 0o644, []byte("123"))
	_ = os.WriteFile(filepath.Join(dir, "tmp-stale.txt"), []byte("t"), 0o644)

	st, err := DirStats("ns", dir)
	if err != nil || st.Entries != 3 || st.Bytes != 9 {
		t.Fatalf("stats = %+v, %v", st, err)
	}
	// Missing dir → zero, no error.
	if st2, err := DirStats("x", filepath.Join(dir, "nope")); err != nil || st2.Entries != 0 {
		t.Fatalf("missing dir stats = %+v, %v", st2, err)
	}

	// GC keeps only keep.txt; tmp- always goes.
	files, bytes, err := GCKeep(dir, func(name string) bool { return name == "keep.txt" }, true)
	if err != nil || files != 2 || bytes != 4 {
		t.Fatalf("gc dry = %d files %d bytes %v", files, bytes, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drop.txt")); err != nil {
		t.Fatal("dry run must not delete")
	}
	if _, _, err := GCKeep(dir, func(name string) bool { return name == "keep.txt" }, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drop.txt")); err == nil {
		t.Fatal("gc should have removed drop.txt")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatal("gc must keep referenced entries")
	}
	// GC on a missing dir is a zero no-op.
	if f, b, err := GCKeep(filepath.Join(dir, "nope"), func(string) bool { return true }, false); err != nil || f != 0 || b != 0 {
		t.Fatalf("gc missing dir = %d %d %v", f, b, err)
	}

	if err := Clean(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("clean should remove the namespace dir")
	}
	if err := Clean(""); err == nil {
		t.Fatal("clean of empty dir must refuse")
	}
}

func TestKeyerFileError(t *testing.T) {
	k := NewKeyer("", 0)
	if err := k.WriteFileContents(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file must error")
	}
}

// fileAsParent returns a path whose PARENT is a regular file — the cheap way
// to force mkdir/readdir failures.
func fileAsParent(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return parent
}

func TestWriteAtomicMkdirError(t *testing.T) {
	parent := fileAsParent(t)
	if err := WriteAtomicBytes(filepath.Join(parent, "sub"), "a", 0o644, []byte("x")); err == nil {
		t.Fatal("mkdir over a file must error")
	}
}

func TestWriteAtomicRenameErrorLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "taken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomicBytes(dir, "taken", 0o644, []byte("x")); err == nil {
		t.Fatal("rename onto a directory must error")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "tmp-") {
			t.Fatalf("temp leftover after failed rename: %s", e.Name())
		}
	}
}

func TestWriteAtomicCreateTempError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o500); err != nil { // read+exec, no write
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := WriteAtomicBytes(dir, "a", 0o644, []byte("x")); err == nil {
		t.Fatal("CreateTemp in a read-only dir must error")
	}
}

func TestWriteAtomicNoExtAndPerm(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAtomicBytes(dir, "noext", 0o600, []byte("y")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "noext"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("perm not applied: %v %v", info, err)
	}
}

func TestWriteAtomicChmodError(t *testing.T) {
	dir := t.TempDir()
	// The fill callback deletes its own temp file, so the following Chmod fails.
	err := WriteAtomic(dir, "gone", 0o644, func(f *os.File) error {
		return os.Remove(f.Name())
	})
	if err == nil {
		t.Fatal("chmod on a removed temp must error")
	}
}

func TestGCSkipsEntriesWithoutInfo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "racy"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// keep() runs before Info(); a callback that removes the file simulates the
	// entry vanishing mid-GC — Info() fails and the entry is skipped, not fatal.
	files, bytes, err := GCKeep(dir, func(name string) bool {
		_ = os.Remove(filepath.Join(dir, name))
		return false
	}, true)
	if err != nil || files != 0 || bytes != 0 {
		t.Fatalf("vanished entry should be skipped: %d %d %v", files, bytes, err)
	}
}

func TestGCOverFilePathErrors(t *testing.T) {
	if _, _, err := GCKeep(fileAsParent(t), func(string) bool { return true }, false); err == nil {
		t.Fatal("GC over a file path must error")
	}
}

func TestDirStatsOverFilePath(t *testing.T) {
	st, err := DirStats("f", fileAsParent(t))
	if err != nil || st.Entries != 1 {
		t.Fatalf("stats over file = %+v, %v", st, err)
	}
}

func TestGCKeepsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := GCKeep(dir, func(string) bool { return false }, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sub); err != nil {
		t.Fatal("GC must not remove subdirectories")
	}
}

func TestDirAndHumanBytes(t *testing.T) {
	if got := Dir("", "images"); got != filepath.Join(".ssg-cache", "images") {
		t.Fatalf("Dir default root = %q", got)
	}
	if got := Dir("/tmp/root", "ai"); got != filepath.Join("/tmp/root", "ai") {
		t.Fatalf("Dir custom root = %q", got)
	}
	cases := map[int64]string{5: "5 B", 2048: "2.0 KiB", 3 * 1024 * 1024: "3.0 MiB"}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Fatalf("HumanBytes(%d) = %q want %q", n, got, want)
		}
	}
	if !strings.Contains(HumanBytes(5*1024*1024*1024), "GiB") {
		t.Fatal("GiB expected")
	}
}
