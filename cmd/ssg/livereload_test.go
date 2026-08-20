package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestHTMLInjectWriter_InjectsIntoHTML(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &htmlInjectWriter{ResponseWriter: rr}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<html><body><h1>hi</h1></body></html>"))
	w.finish()
	body := rr.Body.String()
	if !strings.Contains(body, liveReloadScript) {
		t.Fatalf("script not injected: %s", body)
	}
	if strings.Index(body, liveReloadScript) > strings.Index(body, "</body>") {
		t.Fatalf("script must sit before </body>: %s", body)
	}
}

func TestHTMLInjectWriter_LeavesNonHTML(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &htmlInjectWriter{ResponseWriter: rr}
	w.Header().Set("Content-Type", "text/markdown")
	_, _ = w.Write([]byte("# Title\n\nbody"))
	w.finish()
	if strings.Contains(rr.Body.String(), "__livereload") {
		t.Fatalf("markdown must not get the reload script: %s", rr.Body.String())
	}
}

func TestHTMLInjectWriter_SkipsGzipEncoded(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &htmlInjectWriter{ResponseWriter: rr}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Encoding", "gzip")
	_, _ = w.Write([]byte("compressed-bytes"))
	w.finish()
	if strings.Contains(rr.Body.String(), "__livereload") {
		t.Fatal("must not inject into gzip-encoded HTML")
	}
}

func TestSSEEvent_MultiLineData(t *testing.T) {
	got := sseEvent("builderror", "line one\nline two")
	want := "event: builderror\ndata: line one\ndata: line two\n\n"
	if got != want {
		t.Fatalf("sseEvent:\n got:  %q\n want: %q", got, want)
	}
}

func TestHub_BroadcastReachesSubscriber(t *testing.T) {
	h := newLiveReloadHub()
	ch := h.subscribe()
	h.broadcast(sseEvent("reload", "1"))
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "event: reload") {
			t.Fatalf("unexpected payload: %q", msg)
		}
	default:
		t.Fatal("subscriber did not receive the broadcast")
	}
	h.unsubscribe(ch)
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

func TestNotifyHelpers_NilHubAreNoops(t *testing.T) {
	setReloadHub(nil)
	notifyReload()           // must not panic
	notifyBuildError("boom") // must not panic
}

func TestNotifyBuildError_Broadcasts(t *testing.T) {
	setReloadHub(newLiveReloadHub())
	defer func() { setReloadHub(nil) }()
	ch := currentReloadHub().subscribe()
	notifyBuildError("template: index.html:13: unterminated quoted string")
	msg := <-ch
	if !strings.Contains(msg, "event: builderror") || !strings.Contains(msg, "unterminated quoted string") {
		t.Fatalf("build error not delivered: %q", msg)
	}
}

func TestLiveReloadMiddleware_ServesSSEAndInjects(t *testing.T) {
	setReloadHub(newLiveReloadHub())
	defer func() { setReloadHub(nil) }()
	h := liveReloadMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<body></body>"))
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rr.Body.String(), "__livereload") {
		t.Fatalf("HTML should carry the reload script: %s", rr.Body.String())
	}

	rr2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, liveReloadPath, nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // the SSE loop returns as soon as the context is done
	h.ServeHTTP(rr2, req.WithContext(ctx))
	if ct := rr2.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("SSE content-type = %q", ct)
	}
}

func TestAutoReloadEnabled(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name string
		cfg  config.Config
		want bool
	}{
		{"http+watch default", config.Config{HTTP: true, Watch: true}, true},
		{"http+watch explicit on", config.Config{HTTP: true, Watch: true, AutoReload: &on}, true},
		{"http+watch disabled", config.Config{HTTP: true, Watch: true, AutoReload: &off}, false},
		{"no watch", config.Config{HTTP: true}, false},
		{"no http", config.Config{Watch: true}, false},
	}
	for _, c := range cases {
		if got := autoReloadEnabled(&c.cfg); got != c.want {
			t.Errorf("%s: autoReloadEnabled = %v, want %v", c.name, got, c.want)
		}
	}
}
