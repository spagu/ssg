package externalsource

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClearCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	c := diskCache{dir: dir}
	if err := c.put("k", []byte("x"), cacheMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := ClearCache(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("cache dir must be removed")
	}
	// Empty dir resolves to the default, relative to the working directory.
	t.Chdir(t.TempDir())
	if err := ClearCache(""); err != nil {
		t.Fatal(err)
	}
}

func TestRetriableUnwrap(t *testing.T) {
	cause := fmt.Errorf("boom")
	err := &retriableError{fmt.Errorf("wrap: %w", cause)}
	if !errors.Is(err, cause) {
		t.Fatal("retriableError must unwrap to its cause")
	}
}

func TestApplyAuthVariants(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/", nil)
	applyAuth(req, AuthConfig{Type: "basic", Username: "u", Password: "p"})
	if user, pass, ok := req.BasicAuth(); !ok || user != "u" || pass != "p" {
		t.Fatal("basic auth not applied")
	}
	applyAuth(req, AuthConfig{Type: "header", Header: "X-Api-Key", Value: "v"})
	if req.Header.Get("X-Api-Key") != "v" {
		t.Fatal("header auth not applied")
	}
	applyAuth(req, AuthConfig{}) // no-op
}

func TestBuildResultErrorPaths(t *testing.T) {
	conn := testConnector(t)
	src := httpSource("api", "https://api.example.com/a.json", "json")
	// Corrupt cached payload: parse fails and the entry is evicted.
	key := cacheKey(src)
	meta := cacheMeta{Source: "api", FetchedAt: time.Now()}
	if _, err := conn.buildResult(src, []byte("{broken"), meta, true, false); err == nil {
		t.Fatal("broken payload must error")
	}
	if _, _, ok := conn.cache.get(key); ok {
		t.Fatal("broken cached payload must be evicted")
	}
	// Transform failure.
	src.Transform.Select = "missing"
	if _, err := conn.buildResult(src, []byte(`{"a":1}`), meta, false, false); err == nil {
		t.Fatal("transform failure must error")
	}
	// Config-stage failure: URL fails validation at result-building time.
	src = httpSource("api", "ftp://api.example.com/a.json", "json")
	if _, err := conn.buildResult(src, []byte(`{"a":1}`), meta, false, false); err == nil {
		t.Fatal("invalid URL must error")
	}
}

func TestCachePutGetErrorPaths(t *testing.T) {
	// A file where the cache dir should be → MkdirAll fails.
	tmp := t.TempDir()
	blocked := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := diskCache{dir: blocked}
	if err := c.put("k", []byte("x"), cacheMeta{}); err == nil {
		t.Fatal("put into a file-path must error")
	}
	// Unreadable meta JSON → eviction + miss.
	dir := t.TempDir()
	c = diskCache{dir: dir}
	if err := os.WriteFile(filepath.Join(dir, "k"+metaSuffix), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "k"+bodySuffix), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.get("k"); ok {
		t.Fatal("broken meta must miss")
	}
	// Meta present, body missing → miss.
	if err := c.put("m", []byte("x"), cacheMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "m"+bodySuffix)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.get("m"); ok {
		t.Fatal("missing body must miss")
	}
}

func TestNewHTTPConnectorPolicyWiring(t *testing.T) {
	off := false
	cfg := Config{CacheDir: "/tmp/x", Offline: true, Refresh: true, Only: "api",
		StaleIfError: &off, FailOnCacheMiss: &off, AllowedHosts: []string{"a.example.com"}}
	conn := newHTTPConnector(cfg)
	if conn.cache.dir != "/tmp/x" || !conn.offline || !conn.refresh || conn.refreshOnly != "api" ||
		conn.staleIfError || conn.failOnMiss || len(conn.allowedHosts) != 1 {
		t.Fatalf("wiring = %+v", conn)
	}
	if def := newHTTPConnector(Config{}); def.cache.dir != DefaultCacheDir || !def.staleIfError || !def.failOnMiss {
		t.Fatalf("defaults = %+v", def)
	}
}

func TestValidateURLEdges(t *testing.T) {
	src := Source{Name: "s", AllowHTTP: true}
	if _, err := validateURL("://bad", src, nil); err == nil {
		t.Fatal("unparseable URL must error")
	}
	if _, err := validateURL("https:///path-only", src, nil); err == nil || !strings.Contains(err.Error(), "missing host") {
		t.Fatalf("missing host = %v", err)
	}
	u, err := validateURL("https://user:secret@api.example.com/p?token=abc", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	safe := safeIdentifier(u)
	if strings.Contains(safe, "secret") || strings.Contains(safe, "token=") {
		t.Fatalf("identifier leaks credentials: %q", safe)
	}
}

func TestSecureDialUnresolvableHost(t *testing.T) {
	src := httpSource("nx", "https://definitely-does-not-exist.invalid/x.json", "json")
	src.Retries = 0
	src.Timeout = 2 * time.Second
	if _, err := testConnector(t).Load(src); err == nil {
		t.Fatal("unresolvable host must error")
	}
}
