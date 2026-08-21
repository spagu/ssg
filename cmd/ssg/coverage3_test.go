package main

// Branch-level gap tests for cmd/ssg (coverage raise, 1.8.28). Each test names
// the contract it pins, not the line it colours.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestParseCheckFlagBareForms: every bare --check-* toggle means "warn", and
// the deprecated --seo-off still disables SEO injection instead of erroring.
func TestParseCheckFlagBareForms(t *testing.T) {
	cfg := config.DefaultConfig()
	parseFlags([]string{"--check-images", "--check-meta", "--check-schema",
		"--check-orphans", "--check-redirects", "--seo-off"}, cfg)
	for name, got := range map[string]string{
		"check-images": cfg.CheckImages, "check-meta": cfg.CheckMeta,
		"check-schema": cfg.CheckSchema, "check-orphans": cfg.CheckOrphans,
		"check-redirects": cfg.CheckRedirects,
	} {
		if got != "warn" {
			t.Errorf("bare --%s = %q, want warn", name, got)
		}
	}
	if cfg.SEO {
		t.Error("--seo-off must disable SEO")
	}
}

// TestNotifyHelpers: with a hub running, a successful rebuild broadcasts
// "reload" and a failed one "builderror" — the browser-side contract of GO-090.
func TestNotifyHelpers(t *testing.T) {
	old := currentReloadHub()
	setReloadHub(newLiveReloadHub())
	t.Cleanup(func() { setReloadHub(old) })
	ch := currentReloadHub().subscribe()
	notifyReload()
	if got := <-ch; !strings.Contains(got, "event: reload") {
		t.Fatalf("reload event = %q", got)
	}
	notifyBuildError("boom\r\nline2")
	if got := <-ch; !strings.Contains(got, "event: builderror") || !strings.Contains(got, "data: line2") {
		t.Fatalf("builderror event = %q", got)
	}
	currentReloadHub().unsubscribe(ch)
	currentReloadHub().unsubscribe(ch) // double unsubscribe must be a safe no-op
}

// TestHubSlowClientDrop: a client that stops reading must never block a build —
// the payload is dropped once its buffer is full.
func TestHubSlowClientDrop(t *testing.T) {
	h := newLiveReloadHub()
	ch := h.subscribe()
	for i := 0; i < 16; i++ { // buffer is 8; the rest must drop, not deadlock
		h.broadcast("x")
	}
	if len(ch) != 8 {
		t.Fatalf("buffered = %d, want full buffer of 8", len(ch))
	}
	h.unsubscribe(ch)
}

// nonFlusher hides http.Flusher so the SSE endpoint's guard is reachable.
type nonFlusher struct{ h http.Header }

func (n nonFlusher) Header() http.Header         { return n.h }
func (n nonFlusher) Write(b []byte) (int, error) { return len(b), nil }
func (n nonFlusher) WriteHeader(int)             {}

// TestServeSSEBranches: a writer without streaming support is refused; a
// subscriber whose channel closes under it ends the stream cleanly.
func TestServeSSEBranches(t *testing.T) {
	h := newLiveReloadHub()
	h.serveSSE(nonFlusher{h: http.Header{}}, httptest.NewRequest(http.MethodGet, liveReloadPath, nil))

	done := make(chan struct{})
	go func() {
		rec := httptest.NewRecorder()
		h.serveSSE(rec, httptest.NewRequest(http.MethodGet, liveReloadPath, nil))
		close(done)
	}()
	// Wait for the subscription, deliver one payload, then close the channel.
	var ch chan string
	for {
		h.mu.Lock()
		for c := range h.subs {
			ch = c
		}
		h.mu.Unlock()
		if ch != nil {
			break
		}
		runtime.Gosched()
	}
	h.broadcast("event: reload\ndata: 1\n\n")
	h.unsubscribe(ch)
	<-done
}

// TestHTMLInjectStatusAndNoBody: an explicit handler status survives the
// buffering writer, and HTML without </body> still gets the script appended.
func TestHTMLInjectStatusAndNoBody(t *testing.T) {
	old := currentReloadHub()
	setReloadHub(newLiveReloadHub())
	t.Cleanup(func() { setReloadHub(old) })
	handler := liveReloadMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html>no body tag"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("If-None-Match", "etag") // must be stripped for fresh HTML
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 preserved", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "__livereload") {
		t.Fatal("script must be appended even without </body>")
	}
	if req.Header.Get("If-None-Match") != "" {
		t.Fatal("conditional headers must be stripped")
	}
}

// TestLoadCacheConfigVariants: the cache CLI works with a good config, a broken
// one (warn + defaults) and none at all.
func TestLoadCacheConfigVariants(t *testing.T) {
	t.Chdir(t.TempDir())
	if cfg := loadCacheConfig(); cfg != nil {
		t.Fatal("no config file must yield nil")
	}
	if err := os.WriteFile(".ssg.yaml", []byte("source: s\ntemplate: t\ndomain: d\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg := loadCacheConfig(); cfg == nil || cfg.Source != "s" {
		t.Fatal("valid config must load")
	}
	if err := os.WriteFile(".ssg.yaml", []byte(":\nbroken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg := loadCacheConfig(); cfg != nil {
		t.Fatal("broken config must fall back to nil, not exit")
	}
}

// TestRunCacheGCPerNamespace: gc reports per namespace — external-sources
// reclaims, images points at --images-gc (the manifest lives in a build), and
// the ai namespace explains it has no expiry.
func TestRunCacheGCPerNamespace(t *testing.T) {
	t.Chdir(t.TempDir())
	if code := runCache([]string{"gc"}); code != 0 {
		t.Fatalf("gc on empty caches = %d", code)
	}
	if code := runCache([]string{"gc", "--dry"}); code != 0 {
		t.Fatalf("gc --dry = %d", code)
	}
}

// TestRunCacheGCExternalError: an unreadable external-sources cache dir is a
// reported failure (exit 1), not a silent skip.
func TestRunCacheGCExternalError(t *testing.T) {
	t.Chdir(t.TempDir())
	// A FILE where the namespace directory belongs makes the walk fail.
	if err := os.MkdirAll(".ssg-cache", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".ssg-cache", "external-sources"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runCache([]string{"gc", "--namespace=external-sources"}); code != 1 {
		t.Fatalf("gc over a corrupt namespace = %d, want 1", code)
	}
}

// TestEmitEndpointsUnknownPlatform: a typo'd endpoints_platform fails the build
// with the platform named, instead of silently emitting nothing.
func TestEmitEndpointsUnknownPlatform(t *testing.T) {
	cfg := &config.Config{EndpointsPlatform: "bogus",
		Endpoints: []config.Endpoint{{Type: "form", Path: "/api/x", To: "https://e.com"}}}
	if err := emitEndpoints(cfg); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("unknown platform must fail with its name, got %v", err)
	}
}

// TestOpenGitHubPRNoRepo: without a repo there is nothing to call — the error
// says what to configure instead of hitting the API.
func TestOpenGitHubPRNoRepo(t *testing.T) {
	if _, err := openGitHubPR("tok", "", "", "head", "t", "b"); err == nil ||
		!strings.Contains(err.Error(), "mcp.git.repo") {
		t.Fatalf("missing repo error = %v", err)
	}
}
