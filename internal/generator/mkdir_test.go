package generator

// Creating each output directory once.

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestEnsureDirCreatesAndRemembers: the directory is made, and the second call
// is a map hit rather than a walk of the path.
func TestEnsureDirCreatesAndRemembers(t *testing.T) {
	g := newTestGen(t, "")
	deep := filepath.Join(t.TempDir(), "a", "b", "c")

	if err := g.ensureDir(deep); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(deep); err != nil || !info.IsDir() {
		t.Fatalf("the directory must exist: %v", err)
	}
	if !g.dirs.has(deep) {
		t.Error("the directory must be remembered")
	}
	// Every ancestor was created by the same MkdirAll, so a sibling written
	// next needs no syscall either — that is where the saving comes from.
	if !g.dirs.has(filepath.Dir(deep)) || !g.dirs.has(filepath.Dir(filepath.Dir(deep))) {
		t.Error("ancestors must be recorded too")
	}
	// A second call is a no-op and still succeeds.
	if err := g.ensureDir(deep); err != nil {
		t.Errorf("second call = %v", err)
	}
}

// TestEnsureParentTakesTheFilesDirectory, which is the call the render path
// actually makes.
func TestEnsureParentTakesTheFilesDirectory(t *testing.T) {
	g := newTestGen(t, "")
	file := filepath.Join(t.TempDir(), "x", "y", "index.html")

	if err := g.ensureParent(file); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Dir(file)); err != nil || !info.IsDir() {
		t.Fatalf("the parent must exist: %v", err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("the file itself must not be created")
	}
}

// TestEnsureDirIgnoresNothing: an empty path is not a directory to make, and
// asking for one must not create something at the working directory.
func TestEnsureDirIgnoresNothing(t *testing.T) {
	g := newTestGen(t, "")
	for _, p := range []string{"", "."} {
		if err := g.ensureDir(p); err != nil {
			t.Errorf("ensureDir(%q) = %v", p, err)
		}
	}
}

// TestEnsureDirReportsRealFailures: a cache must not swallow an error, or a
// build reports success having written nothing.
func TestEnsureDirReportsRealFailures(t *testing.T) {
	g := newTestGen(t, "")
	// A file where a directory should be.
	file := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.ensureDir(filepath.Join(file, "child")); err == nil {
		t.Error("a path blocked by a file must be an error")
	}
	if g.dirs.has(filepath.Join(file, "child")) {
		t.Error("a directory that was not created must not be remembered")
	}
}

// TestEnsureDirIsSafeUnderTheRenderPool: pages are written concurrently, so the
// cache is touched from many goroutines at once.
func TestEnsureDirIsSafeUnderTheRenderPool(t *testing.T) {
	g := newTestGen(t, "")
	root := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// Half share a parent, half are distinct — the shared ones are what
			// the cache is for.
			dir := filepath.Join(root, "shared", "page")
			if n%2 == 0 {
				dir = filepath.Join(root, "own", string(rune('a'+n%26)))
			}
			if err := g.ensureDir(dir); err != nil {
				t.Errorf("ensureDir = %v", err)
			}
		}(i)
	}
	wg.Wait()

	if info, err := os.Stat(filepath.Join(root, "shared", "page")); err != nil || !info.IsDir() {
		t.Fatalf("the shared directory must exist: %v", err)
	}
}
