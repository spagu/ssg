package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestRunFlags(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, env(nil), &out); code != 0 || !strings.Contains(out.String(), "tts-server") {
		t.Fatal(code, out.String())
	}
	if code := run(context.Background(), []string{"-nope"}, env(nil), &out); code != 2 {
		t.Fatal(code)
	}
	if code := run(context.Background(), nil, env(map[string]string{"TTS_TIMEOUT": "x"}), &out); code != 2 {
		t.Fatal(code)
	}
}

func TestHealthcheck(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	addr := func(s *httptest.Server) string { return strings.TrimPrefix(s.URL, "http://") }
	var out bytes.Buffer
	if code := run(context.Background(), []string{"-healthcheck"}, env(map[string]string{"TTS_ADDR": addr(ok)}), &out); code != 0 {
		t.Fatal("healthy", code)
	}
	if healthcheck(context.Background(), addr(bad)) != 1 {
		t.Fatal("unhealthy")
	}
	_, port, _ := net.SplitHostPort(addr(ok))
	if healthcheck(context.Background(), ":"+port) != 0 {
		t.Fatal("empty host means loopback")
	}
	if healthcheck(context.Background(), "no-port") != 1 || healthcheck(context.Background(), "127.0.0.1:1") != 1 {
		t.Fatal("bad addresses")
	}
	if healthcheck(context.Background(), "[::1%zz]:x\x7f") != 1 {
		t.Fatal("bad url")
	}
}

func TestServeLifecycle(t *testing.T) {
	dir := t.TempDir()
	base := map[string]string{"TTS_VOICES_DIR": filepath.Join(dir, "v"), "TTS_ADDR": "127.0.0.1:0", "TTS_LOG_LEVEL": "error"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int)
	var out bytes.Buffer
	go func() { done <- run(ctx, nil, env(base), &out) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	if code := <-done; code != 0 {
		t.Fatal(code, out.String())
	}
	base["TTS_ADDR"] = "256.0.0.1:99999"
	if code := run(context.Background(), nil, env(base), &out); code != 1 {
		t.Fatal("listen error", code)
	}
	base["TTS_VOICES_DIR"] = "/proc/forbidden/voices"
	if code := run(context.Background(), nil, env(base), &out); code != 1 {
		t.Fatal("build error", code)
	}
}
