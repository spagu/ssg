package main

// The startup sequence, end to end: what `ssg` does between the command line
// and either an exit code or a loop that owns the process.
//
// It used to live in main(), where os.Exit made every decision in it
// untestable — and the decisions are not small ones: which subcommand claims
// the arguments, whether a refused edit-mode combination stops the run before
// anything is written, and whether a failed build is fatal or something a
// watcher is expected to fix.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rcovProject lays out a buildable project in its own directory and returns the
// arguments that build it.
func rcovProject(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	write := func(rel, body string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("content/site/metadata.json", `{"categories":[],"media":[],"users":[]}`)
	write("content/site/pages/home.md", "---\ntitle: Home\nslug: home\nstatus: publish\ntype: page\n---\n\nHi.\n")
	for _, name := range []string{"base.html", "index.html", "post.html", "page.html",
		"category.html", "tag.html", "taxonomy.html"} {
		write("templates/simple/"+name, `{{define "`+name+`"}}<html><body><p>x</p></body></html>{{end}}`)
	}
	return []string{"site", "simple", "example.com", "--quiet"}
}

// TestAOneShotBuildFinishesAndSaysSo: nothing is waiting on it, so it returns
// rather than handing the process to a loop.
func TestAOneShotBuildFinishesAndSaysSo(t *testing.T) {
	args := rcovProject(t)
	code, done := run(args)
	if !done || code != 0 {
		t.Fatalf("run = %d, %v", code, done)
	}
	if _, err := os.Stat(filepath.Join("output", "home", "index.html")); err != nil {
		t.Errorf("the site was not built: %v", err)
	}
}

// TestASubcommandClaimsTheArgumentsBeforeAnythingIsBuilt: `ssg graph` and
// friends must never reach the builder, whatever is in the directory.
func TestASubcommandClaimsTheArgumentsBeforeAnythingIsBuilt(t *testing.T) {
	rcovProject(t)
	// A verb+noun pair, and a standalone verb, each answered by its own handler.
	if _, done := run([]string{"graph", "--nope"}); !done {
		t.Error("`ssg graph` fell through to a build")
	}
	if _, done := run([]string{"cache", "stats"}); !done {
		t.Error("`ssg cache stats` fell through to a build")
	}
	if _, err := os.Stat("output"); err == nil {
		t.Error("a subcommand must not build the site on its way past")
	}
}

// TestARefusedEditModeStopsBeforeTheBuild.
//
// --edit without the server that serves it is a combination that cannot work,
// and building anyway would write a site carrying editor markup that nothing
// is going to strip.
func TestARefusedEditModeStopsBeforeTheBuild(t *testing.T) {
	args := append(rcovProject(t), "--edit")
	out := captureStderr(t, func() {
		code, done := run(args)
		if !done || code != 2 {
			t.Errorf("run = %d, %v", code, done)
		}
	})
	if !strings.Contains(strings.ToLower(out), "edit") {
		t.Errorf("stderr = %q", out)
	}
	if _, err := os.Stat("output"); err == nil {
		t.Error("a refused combination must not leave a built site behind")
	}
}

// TestABuildThatFailsIsFatalWhenNobodyIsWatching, and only then: under --watch
// or --http a person is sitting there to fix it, and exiting would take the
// preview down with the mistake.
func TestABuildThatFailsIsFatalWhenNobodyIsWatching(t *testing.T) {
	args := rcovProject(t)
	// A template that does not parse: the theme name still resolves, so this
	// fails in rendering rather than in argument checking.
	if err := os.WriteFile(filepath.Join("templates", "simple", "page.html"),
		[]byte(`{{define "page.html"}}{{ .Title | nosuchhelper }}{{end}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, done := run(args)
	if !done || code != 1 {
		t.Errorf("run = %d, %v", code, done)
	}
}
