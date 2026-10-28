package deploy

// What a deploy does when the machine, not the provider, fails: an unreadable
// file in the output, a network that refuses, no git. Each is an error, never
// a partial upload reported as success.

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// failingTransport refuses every request, as a network that is down does.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network down")
}

var offline = &http.Client{Transport: failingTransport{}}

// unreadableTree is an output directory holding one file nobody may read.
func unreadableTree(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "index.html")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	return dir
}

func TestUnreadableOutputFileFailsTheWalk(t *testing.T) {
	dir := unreadableTree(t)
	if _, err := walkFiles(dir); err == nil {
		t.Error("walkFiles: want an error")
	}
	if _, err := collectSiteFiles(dir); err == nil {
		t.Error("collectSiteFiles: want an error")
	}
}

func TestUploadsReportANetworkThatIsDown(t *testing.T) {
	ctx := context.Background()
	if err := netlifyUpload(ctx, offline, "https://api.example.com", "d1", "tok", "/index.html", []byte("x")); err == nil {
		t.Error("netlify: want an error")
	}
	if _, err := vercelCreateDeployment(ctx, offline, "tok", "", "site", nil); err == nil {
		t.Error("vercel: want an error")
	}
}

func TestGitHubPagesWithoutGit(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := deployGitHubPages(context.Background(), Options{Dir: t.TempDir(), Target: "https://example.com/r.git"})
	if err == nil {
		t.Error("want an error when git is not in PATH")
	}
}

func TestGitHubPagesIntoAMissingDirectory(t *testing.T) {
	_, err := deployGitHubPages(context.Background(), Options{
		Dir: filepath.Join(t.TempDir(), "missing"), Target: "https://example.com/r.git",
	})
	if err == nil {
		t.Error("want an error when the output directory does not exist")
	}
}
