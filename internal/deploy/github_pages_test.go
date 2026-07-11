package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGithubPagesURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/user/repo.git":       "https://user.github.io/repo/",
		"git@github.com:user/repo.git":           "https://user.github.io/repo/",
		"https://github.com/user/user.github.io": "https://user.github.io/",
		"https://gitlab.com/user/repo.git":       "https://gitlab.com/user/repo.git", // non-GitHub → unchanged
	}
	for remote, want := range cases {
		if got := githubPagesURL(remote); got != want {
			t.Errorf("githubPagesURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

// TestDeployGitHubPagesLocalBare force-pushes the output tree to a local bare repo,
// exercising the full init→add→commit→push flow without any network.
func TestDeployGitHubPagesLocalBare(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v: %s", err, out)
	}

	site := t.TempDir()
	if err := os.WriteFile(filepath.Join(site, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	url, err := deployGitHubPages(context.Background(), Options{
		Dir: site, Target: bare, Branch: "gh-pages", Quiet: true,
		Env: func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("deployGitHubPages: %v", err)
	}
	// Non-GitHub remote → URL echoes the remote path.
	if url != bare {
		t.Errorf("url = %q, want %q", url, bare)
	}
	// The gh-pages branch now exists in the bare repo with our file.
	out, err := exec.Command("git", "--git-dir", bare, "ls-tree", "--name-only", "gh-pages").CombinedOutput()
	if err != nil {
		t.Fatalf("ls-tree: %v: %s", err, out)
	}
	if string(out) != "index.html\n" {
		t.Errorf("pushed tree = %q, want index.html", out)
	}
	// The temporary .git in the output dir must be cleaned up.
	if _, err := os.Stat(filepath.Join(site, ".git")); !os.IsNotExist(err) {
		t.Error("temporary .git not removed from output dir")
	}
}

func TestDeployGitHubPagesBadRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	site := t.TempDir()
	_ = os.WriteFile(filepath.Join(site, "x.html"), []byte("x"), 0o644)
	// An explicit, invalid target avoids resolving the real repo's origin and makes
	// the push fail cleanly. (Never call with an empty Target from a test — that would
	// fall back to this repo's origin and could push for real.)
	bad := filepath.Join(t.TempDir(), "not-a-repo")
	_, err := deployGitHubPages(context.Background(), Options{
		Dir: site, Target: bad, Quiet: true, Env: func(string) string { return "" },
	})
	if err == nil {
		t.Error("expected push to fail for an invalid remote")
	}
}
