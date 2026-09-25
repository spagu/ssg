package generator

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/html"
)

// symlinkOut links out/<name> to a file outside the output tree, which is the
// only way a walk over the output can reach beyond it.
func symlinkOut(t *testing.T, out, name, body string) string {
	t.Helper()
	outside := filepath.Join(t.TempDir(), "outside"+filepath.Ext(name))
	mustWrite(t, outside, body)
	link := filepath.Join(out, name)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return outside
}

// TestWalkOutputHTMLSkipsSymlinks: the checks open files through an os.Root on
// the output, so a symlinked page is not followed out of it.
func TestWalkOutputHTMLSkipsSymlinks(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "page", "index.html"), "<html></html>")
	symlinkOut(t, out, "linked.html", "<html></html>")

	g := &Generator{config: Config{OutputDir: out, Quiet: true}}
	var visited []string
	if err := g.walkOutputHTML(func(rel string, _ *html.Node) { visited = append(visited, rel) }); err != nil {
		t.Fatal(err)
	}
	if len(visited) != 1 || visited[0] != "page/index.html" {
		t.Errorf("visited = %v, want only page/index.html", visited)
	}
}

// TestFingerprintNeverDeletesOutsideTheOutput: a stale manifest entry that is
// a symlink is not followed, and the file it points at survives.
func TestFingerprintNeverDeletesOutsideTheOutput(t *testing.T) {
	out := t.TempDir()
	outside := symlinkOut(t, out, "js/app.0123abcd.js", "console.log(1)")
	mustWrite(t, filepath.Join(out, "assets-manifest.json"), `{"js/app.js":"js/app.0123abcd.js"}`)

	g := &Generator{config: Config{OutputDir: out, Fingerprint: true, Quiet: true}}
	js, _, err := g.collectFingerprintAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 0 {
		t.Errorf("a symlinked asset must not be queued: %v", js)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the file outside the output was touched: %v", err)
	}
}

// TestCollectFingerprintAssetsMissingOutput: no output tree is an error, as it
// was when the walk reported it.
func TestCollectFingerprintAssetsMissingOutput(t *testing.T) {
	g := &Generator{config: Config{OutputDir: filepath.Join(t.TempDir(), "never-built"), Quiet: true}}
	if _, _, err := g.collectFingerprintAssets(); err == nil {
		t.Error("a missing output directory must be reported")
	}
}
