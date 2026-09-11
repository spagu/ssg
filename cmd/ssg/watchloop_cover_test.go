package main

// The MDDB watch loop, which polls a collection's checksum and rebuilds when it
// moves. Its sibling runWatchLoop has been stoppable since #191; this one only
// became so when it needed testing, and the reason is the same either way: a
// loop with no exit cannot be owned by anything but the process.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// wcovChecksumServer answers /v1/checksum, handing out the next value each call
// and counting how many times it was asked.
func wcovChecksumServer(t *testing.T, values []string, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("no collection here"))
			return
		}
		value := values[len(values)-1]
		if n < len(values) {
			value = values[n]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collection": r.URL.Query().Get("collection"), "checksum": value, "documentCount": 3,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// wcovSite is a buildable one-page project in its own directory, so a rebuild
// has somewhere real to write.
func wcovSite(t *testing.T, url string) (generator.Config, *config.Config, string) {
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

	cfg := &config.Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(dir, "content"), TemplatesDir: filepath.Join(dir, "templates"),
		OutputDir: filepath.Join(dir, "output"), Quiet: true,
	}
	cfg.Mddb.Enabled = true
	cfg.Mddb.Watch = true
	cfg.Mddb.URL = url
	cfg.Mddb.Collection = "site"
	cfg.Mddb.AllowHTTP = true
	cfg.Mddb.WatchInterval = 1
	genCfg := generator.Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: cfg.ContentDir, TemplatesDir: cfg.TemplatesDir,
		OutputDir: cfg.OutputDir, Quiet: true,
	}
	return genCfg, cfg, dir
}

// wcovRun runs the loop and reports whether it returned before the deadline. A
// loop that ignores its stop channel must fail the test rather than hang it.
func wcovRun(t *testing.T, genCfg generator.Config, cfg *config.Config, stop chan struct{}, within time.Duration) bool {
	t.Helper()
	done := make(chan struct{})
	go func() {
		runMddbWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(within):
		close(stop) // let the goroutine finish so the test binary can exit
		return false
	}
}

// TestMddbWatchStopsWhenItsOwnerIsDone: a watcher that outlives its owner keeps
// rebuilding into whatever directory the process has wandered to by then, which
// is the failure #191 fixed for the file watcher.
func TestMddbWatchStopsWhenItsOwnerIsDone(t *testing.T) {
	srv, _ := wcovChecksumServer(t, []string{"aaa"}, http.StatusOK)
	genCfg, cfg, _ := wcovSite(t, srv.URL)
	stop := make(chan struct{})
	close(stop)
	if !wcovRun(t, genCfg, cfg, stop, 5*time.Second) {
		t.Error("the loop ignored its stop channel")
	}
}

// TestMddbWatchRebuildsWhenTheCollectionChanges: the whole point of the loop.
func TestMddbWatchRebuildsWhenTheCollectionChanges(t *testing.T) {
	srv, calls := wcovChecksumServer(t, []string{"first", "second"}, http.StatusOK)
	genCfg, cfg, _ := wcovSite(t, srv.URL)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runMddbWatchLoop(genCfg, cfg, stop)
		close(done)
	}()

	built := filepath.Join(cfg.OutputDir, "home", "index.html")
	deadline := time.After(15 * time.Second)
	for {
		if _, err := os.Stat(built); err == nil {
			break
		}
		select {
		case <-deadline:
			close(stop)
			<-done
			t.Fatalf("no rebuild after %d checksum calls", calls.Load())
		case <-time.After(50 * time.Millisecond):
		}
	}
	close(stop)
	<-done
	if calls.Load() < 2 {
		t.Errorf("the loop should have polled at least twice, got %d", calls.Load())
	}
}

// TestMddbWatchKeepsPollingAfterAFailedCheck: a collection that is briefly
// unreachable must not end the watch — the next poll is the recovery.
func TestMddbWatchKeepsPollingAfterAFailedCheck(t *testing.T) {
	srv, calls := wcovChecksumServer(t, nil, http.StatusInternalServerError)
	genCfg, cfg, _ := wcovSite(t, srv.URL)
	cfg.Quiet = false // the warning is part of the behaviour

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runMddbWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	deadline := time.After(15 * time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			close(stop)
			<-done
			t.Fatalf("the loop stopped polling after %d calls", calls.Load())
		case <-time.After(50 * time.Millisecond):
		}
	}
	close(stop)
	<-done
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "home", "index.html")); err == nil {
		t.Error("a failed checksum check must not trigger a rebuild")
	}
}

// TestMddbWatchReportsAClientItCannotCreate rather than polling nothing in
// silence.
//
// The one construction that fails outright is a Bearer key over an unencrypted
// channel to a host that is not loopback, which the gRPC client refuses rather
// than leak (SEC-004).
func TestMddbWatchReportsAClientItCannotCreate(t *testing.T) {
	genCfg, cfg, _ := wcovSite(t, "")
	cfg.Mddb.Protocol = "grpc"
	cfg.Mddb.URL = "grpc://mddb.example.com:50051"
	cfg.Mddb.APIKey = "secret"
	out := captureStderr(t, func() {
		if !wcovRun(t, genCfg, cfg, make(chan struct{}), 5*time.Second) {
			t.Error("a client that cannot be created should end the loop at once")
		}
	})
	if !strings.Contains(out, "MDDB client") {
		t.Errorf("stderr = %q", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("the key reached the message it was refused to protect: %q", out)
	}
}

// wcovFileSite is a buildable project without MDDB, for the file watcher.
func wcovFileSite(t *testing.T) (generator.Config, *config.Config, string) {
	genCfg, cfg, dir := wcovSite(t, "")
	cfg.Mddb = config.MddbConfig{}
	return genCfg, cfg, dir
}

// TestWatchLoopStopsWhenItsOwnerIsDone: `ssg migrate --watch` owns a watcher,
// and one that outlives its owner keeps rebuilding into whatever directory the
// process has wandered to by then (#191).
func TestWatchLoopStopsWhenItsOwnerIsDone(t *testing.T) {
	genCfg, cfg, _ := wcovFileSite(t)
	stop := make(chan struct{})
	close(stop)
	done := make(chan struct{})
	go func() {
		runWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the loop ignored its stop channel")
	}
}

// TestWatchLoopRebuildsWhenContentChanges, which is the loop's whole job.
func TestWatchLoopRebuildsWhenContentChanges(t *testing.T) {
	genCfg, cfg, dir := wcovFileSite(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	defer func() { close(stop); <-done }()

	// The loop records the tree's state when it starts, and polls once a
	// second. Writing before it has settled would look like no change at all.
	wcovSettle()

	page := filepath.Join(dir, "content", "site", "pages", "home.md")
	if err := os.WriteFile(page,
		[]byte("---\ntitle: Home\nslug: home\nstatus: publish\ntype: page\n---\n\nChanged.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built := filepath.Join(cfg.OutputDir, "home", "index.html")
	if !wcovEventually(t, 20*time.Second, func() bool {
		_, err := os.Stat(built)
		return err == nil
	}) {
		t.Error("a changed page was never rebuilt")
	}
}

// TestWatchLoopReloadsAnEditedConfig.
//
// The config file is a watched input of its own: an edit reloads it and
// rebuilds with the new settings, so the watcher never keeps building from the
// configuration it started with (#70). Here the output directory moves, which
// is the most visible way to prove the new settings were used.
func TestWatchLoopReloadsAnEditedConfig(t *testing.T) {
	genCfg, cfg, dir := wcovFileSite(t)
	configPath := filepath.Join(dir, ".ssg.yaml")
	base := "source: site\ntemplate: simple\ndomain: example.com\ncontent_dir: content\n" +
		"templates_dir: templates\nquiet: true\noutput_dir: "
	if err := os.WriteFile(configPath, []byte(base+"output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// runWatchLoop reads the config path from the process arguments.
	savedArgs := os.Args
	os.Args = []string{"ssg", "--config", configPath}
	defer func() { os.Args = savedArgs }()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	defer func() { close(stop); <-done }()
	wcovSettle()

	if err := os.WriteFile(configPath, []byte(base+"elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "elsewhere", "home", "index.html")
	if !wcovEventually(t, 20*time.Second, func() bool {
		_, err := os.Stat(moved)
		return err == nil
	}) {
		t.Error("the edited config was never picked up")
	}
}

// wcovSettle waits past the watcher's first poll, so an edit made afterwards
// is unambiguously later than the state the loop recorded at startup.
func wcovSettle() { time.Sleep(1500 * time.Millisecond) }

// wcovEventually polls until cond holds or the deadline passes.
func wcovEventually(t *testing.T, within time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.After(within)
	for {
		if cond() {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
}
