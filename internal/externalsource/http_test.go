package externalsource

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// httpSource builds a resolved HTTP source pointing at a local test server
// (allow_private + allow_http, since httptest listens on 127.0.0.1).
func httpSource(name, url, format string) Source {
	return Source{Name: name, Type: "http", Format: format, URL: url,
		Required: true, MaxSize: defaultMaxSize, AllowHTTP: true, AllowPrivate: true,
		Timeout: 2 * time.Second, CacheTTL: time.Hour, StaleTTL: 24 * time.Hour,
		Retries: 2, RetryBackoff: time.Millisecond}
}

// testConnector wires a connector around a temp cache dir.
func testConnector(t *testing.T) HTTPConnector {
	t.Helper()
	return HTTPConnector{cache: diskCache{dir: t.TempDir()}, staleIfError: true, failOnMiss: true}
}

// wantAuthQuery verifies the request carries the fixture auth/query/header set.
func wantAuthQuery(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer sekret-123" &&
		r.URL.Query().Get("page") == "1" && r.Header.Get("X-Client") == "ssg"
}

func TestHTTPFetchAuthQueryAndCache(t *testing.T) {
	t.Setenv("ES_TEST_TOKEN", "sekret-123")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !wantAuthQuery(r) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[1,2,3]}`)
	}))
	defer srv.Close()

	src := httpSource("api", srv.URL+"/products.json", "json")
	token, err := expandAuth("api", AuthConfig{Type: "bearer", Token: "$ES_TEST_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	src.Auth = token
	src.Query = map[string]string{"page": "1"}
	src.Headers = map[string]string{"X-Client": "ssg"}

	conn := testConnector(t)
	res, err := conn.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata.FromCache || res.Metadata.RecordCount != 1 || res.Metadata.ContentType != "json" {
		t.Fatalf("meta = %+v", res.Metadata)
	}
	if !strings.Contains(res.Metadata.Identifier, "/products.json") || strings.Contains(res.Metadata.Identifier, "page=") {
		t.Fatalf("identifier must be query-free: %q", res.Metadata.Identifier)
	}
	// Second load: served from the fresh disk cache, no extra request.
	res2, err := conn.Load(src)
	if err != nil || !res2.Metadata.FromCache || res2.Metadata.Stale {
		t.Fatalf("cache hit = %+v, %v", res2.Metadata, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1", hits.Load())
	}
	// --refresh forces a re-fetch.
	conn.refresh = true
	if _, err := conn.Load(src); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("refresh hits = %d, want 2", hits.Load())
	}
	// --external-source narrows refresh to another name → cache hit again.
	conn.refreshOnly = "other"
	if _, err := conn.Load(src); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("narrowed refresh hits = %d, want 2", hits.Load())
	}
}

func TestHTTPRetryOn500ThenSuccess(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	res, err := testConnector(t).Load(httpSource("flaky", srv.URL, "json"))
	if err != nil || hits.Load() != 3 {
		t.Fatalf("retries: hits=%d err=%v", hits.Load(), err)
	}
	if res.Data.(map[string]interface{})["ok"] != true {
		t.Fatalf("data = %#v", res.Data)
	}
}

func TestHTTPNonRetriableStatusFailsFast(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := testConnector(t).Load(httpSource("gone", srv.URL, "json"))
	if err == nil || hits.Load() != 1 {
		t.Fatalf("404 must fail without retries: hits=%d err=%v", hits.Load(), err)
	}
}

func TestHTTPStaleIfError(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, `{"v":1}`)
	}))
	defer srv.Close()
	src := httpSource("api", srv.URL, "json")
	src.CacheTTL = time.Nanosecond // immediately expired, but within stale TTL
	src.Retries = 0
	conn := testConnector(t)
	if _, err := conn.Load(src); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	res, err := conn.Load(src)
	if err != nil || !res.Metadata.Stale || !res.Metadata.FromCache {
		t.Fatalf("stale fallback = %+v, %v", res, err)
	}
	// With stale-if-error disabled the failure surfaces.
	conn.staleIfError = false
	if _, err := conn.Load(src); err == nil {
		t.Fatal("expected fetch error without stale fallback")
	}
}

func TestHTTPOfflineModes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"v":1}`)
	}))
	defer srv.Close()
	src := httpSource("api", srv.URL, "json")
	conn := testConnector(t)
	if _, err := conn.Load(src); err != nil {
		t.Fatal(err)
	}
	srv.Close() // network gone
	conn.offline = true
	res, err := conn.Load(src)
	if err != nil || !res.Metadata.FromCache {
		t.Fatalf("offline hit = %+v, %v", res, err)
	}
	// Miss with fail_on_cache_miss (default) → hard error.
	other := httpSource("never-fetched", "https://api.example.com/x.json", "json")
	if _, err := conn.Load(other); err == nil || !strings.Contains(err.Error(), "no cached copy") {
		t.Fatalf("offline miss = %v", err)
	}
	// Miss with fail_on_cache_miss=false → sentinel that only warns.
	conn.failOnMiss = false
	_, err = conn.Load(other)
	if !errors.Is(err, errCacheMissSkip) {
		t.Fatalf("skip sentinel = %v", err)
	}
}

func TestHTTPSecurityRules(t *testing.T) {
	conn := testConnector(t)
	// Plain http without allow_http.
	src := httpSource("api", "http://api.example.com/data.json", "json")
	src.AllowHTTP = false
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "allow_http") {
		t.Fatalf("http block = %v", err)
	}
	// Unsupported scheme.
	src = httpSource("api", "ftp://api.example.com/data.json", "json")
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("scheme block = %v", err)
	}
	// Allowlist rejection.
	conn.allowedHosts = []string{"api.example.com", "*.trusted.dev"}
	src = httpSource("api", "https://evil.example.net/data.json", "json")
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "allowed_hosts") {
		t.Fatalf("allowlist block = %v", err)
	}
	mustURL := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parsing %q: %v", raw, err)
		}
		return u
	}
	if !hostAllowed(mustURL("https://api.example.com/x"), conn.allowedHosts) ||
		!hostAllowed(mustURL("https://a.b.trusted.dev/x"), conn.allowedHosts) ||
		hostAllowed(mustURL("https://trusted.dev.evil.com/x"), conn.allowedHosts) ||
		hostAllowed(mustURL("https://trusted.dev/x"), conn.allowedHosts) {
		t.Fatal("hostAllowed matrix")
	}
	// Entries may carry a port, which is then enforced (issue #35).
	ported := []string{"127.0.0.1:8787", "api.example.com"}
	if !hostAllowed(mustURL("http://127.0.0.1:8787/x"), ported) ||
		hostAllowed(mustURL("http://127.0.0.1:9999/x"), ported) ||
		!hostAllowed(mustURL("https://api.example.com/x"), ported) ||
		!hostAllowed(mustURL("https://api.example.com:443/x"), []string{"api.example.com:443"}) ||
		!hostAllowed(mustURL("https://api.example.com/x"), []string{"api.example.com:443"}) ||
		!hostAllowed(mustURL("http://api.example.com/x"), []string{"api.example.com:80"}) ||
		!hostAllowed(mustURL("https://a.b.example.com/x"), []string{"*.example.com:443"}) ||
		hostAllowed(mustURL("https://api.example.com:8443/x"), []string{"api.example.com:443"}) ||
		hostAllowed(mustURL("https://api.example.com/x"), []string{"  "}) {
		t.Fatal("hostAllowed port matrix")
	}
	// SSRF: loopback dial refused without allow_private.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	conn.allowedHosts = nil
	src = httpSource("local", srv.URL, "json")
	src.AllowPrivate = false
	src.Retries = 0
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("ssrf block = %v", err)
	}
}

func TestHTTPContentTypeAndSizeLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wrong-type":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `{"v":1}`)
		case "/big":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"v":"`+strings.Repeat("x", 200)+`"}`)
		}
	}))
	defer srv.Close()
	conn := testConnector(t)
	src := httpSource("bad", srv.URL+"/wrong-type", "json")
	src.Retries = 0
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "content-type") {
		t.Fatalf("content-type = %v", err)
	}
	src = httpSource("big", srv.URL+"/big", "json")
	src.MaxSize = 50
	src.Retries = 0
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("size limit = %v", err)
	}
	// Accepted content-type matrix.
	cases := []struct {
		format, ct string
		ok         bool
	}{
		{"json", "application/json; charset=utf-8", true},
		{"json", "application/vnd.api+json", true},
		{"json", "", true},
		{"json", "text/plain; charset=utf-8", true}, // sniffed label for headerless servers
		{"csv", "text/csv", true},
		{"xml", "application/rss+xml", true},
		{"yaml", "text/plain", true},
		{"json", "text/html", false},
		{"csv", "application/json", false},
	}
	for _, c := range cases {
		if contentTypeAccepted(c.format, c.ct) != c.ok {
			t.Errorf("contentTypeAccepted(%s, %s) != %v", c.format, c.ct, c.ok)
		}
	}
}

func TestHTTPRedirectLimit(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()
	src := httpSource("loop", srv.URL, "json")
	src.Retries = 0
	if _, err := testConnector(t).Load(src); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect limit = %v", err)
	}
}

func TestCacheCorruptionDetection(t *testing.T) {
	dir := t.TempDir()
	c := diskCache{dir: dir}
	key := cacheKey(httpSource("api", "https://api.example.com/a.json", "json"))
	meta := cacheMeta{Source: "api", FetchedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := c.put(key, []byte(`{"v":1}`), meta); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.get(key); !ok {
		t.Fatal("expected cache hit")
	}
	// Corrupt the body: checksum mismatch must evict and miss.
	if err := os.WriteFile(dir+"/"+key+bodySuffix, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.get(key); ok {
		t.Fatal("corrupted entry must miss")
	}
	if _, err := os.Stat(dir + "/" + key + metaSuffix); !os.IsNotExist(err) {
		t.Fatal("corrupted entry must be evicted")
	}
	// A zero-dir cache is inert.
	inert := diskCache{}
	if err := inert.put("k", nil, cacheMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := inert.get("k"); ok {
		t.Fatal("zero cache must miss")
	}
}

func TestCacheKeyStability(t *testing.T) {
	a := httpSource("api", "https://api.example.com/a.json", "json")
	b := a
	if cacheKey(a) != cacheKey(b) {
		t.Fatal("identical configs must share a key")
	}
	b.Query = map[string]string{"page": "2"}
	if cacheKey(a) == cacheKey(b) {
		t.Fatal("different queries must not share a key")
	}
}

func TestSecretsExpansion(t *testing.T) {
	t.Setenv("ES_SECRET", "v")
	if _, err := expandAuth("s", AuthConfig{Type: "bearer", Token: "literal"}); err == nil ||
		!strings.Contains(err.Error(), "environment variable") {
		t.Fatalf("literal secret = %v", err)
	}
	if _, err := expandAuth("s", AuthConfig{Type: "bearer", Token: "$ES_MISSING"}); err == nil ||
		!strings.Contains(err.Error(), "$ES_MISSING") {
		t.Fatalf("missing env = %v", err)
	}
	basic, err := expandAuth("s", AuthConfig{Type: "basic", Username: "u", Password: "$ES_SECRET"})
	if err != nil || basic.Password != "v" {
		t.Fatalf("basic = %+v, %v", basic, err)
	}
	if _, err := expandAuth("s", AuthConfig{Type: "basic", Password: "$ES_SECRET"}); err == nil {
		t.Fatal("basic without username must error")
	}
	hdr, err := expandAuth("s", AuthConfig{Type: "header", Header: "X-Key", Value: "$ES_SECRET"})
	if err != nil || hdr.Value != "v" {
		t.Fatalf("header = %+v, %v", hdr, err)
	}
	if _, err := expandAuth("s", AuthConfig{Type: "header", Value: "$ES_SECRET"}); err == nil {
		t.Fatal("header auth without header name must error")
	}
	if _, err := expandAuth("s", AuthConfig{Type: "oauth-dance"}); err == nil {
		t.Fatal("unknown auth type must error")
	}
	if none, err := expandAuth("s", AuthConfig{}); err != nil || none.Type != "" {
		t.Fatal("empty auth must pass through")
	}
	m, err := expandValueMap("s", "headers", map[string]string{"Accept": "text/csv", "X-Token": "$ES_SECRET"})
	if err != nil || m["Accept"] != "text/csv" || m["X-Token"] != "v" {
		t.Fatalf("map expansion = %+v, %v", m, err)
	}
}

func TestRegistryLoadsHTTPAndFileConcurrently(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate any resolver quirks
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[1,2]`)
	}))
	defer srv.Close()
	filePath := writeFile(t, t.TempDir()+"/nav.yaml", "a: 1\n")
	cfg := Config{Enabled: true, CacheDir: t.TempDir(), MaxConcurrent: 2,
		Sources: map[string]SourceConfig{
			"api":  {Type: "http", URL: srv.URL + "/d.json", AllowHTTP: boolPtr(true), AllowPrivate: boolPtr(true)},
			"file": {Type: "file", Path: filePath},
		}}
	reg, warns, err := Load(cfg)
	if err != nil || len(warns) != 0 {
		t.Fatalf("load: %v %v", err, warns)
	}
	if len(reg.Order) != 2 || reg.Meta()["api"].SourceType != "http" {
		t.Fatalf("registry = %+v", reg.Order)
	}
}

func TestHTTPResolveErrors(t *testing.T) {
	cases := map[string]SourceConfig{
		"missing url":  {Type: "http"},
		"bad timeout":  {Type: "http", URL: "https://a.example.com/x.json", Timeout: "soon"},
		"neg retries":  {Type: "http", URL: "https://a.example.com/x.json", Retries: intPtr(-1)},
		"bad format":   {Type: "http", URL: "https://a.example.com/x.bin"},
		"literal auth": {Type: "http", URL: "https://a.example.com/x.json", Auth: AuthConfig{Type: "bearer", Token: "abc"}},
	}
	for label, sc := range cases {
		if _, err := Resolve(Config{Sources: map[string]SourceConfig{"s": sc}}); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
	// Defaults layer into durations and retries.
	cfg := Config{
		Defaults: Defaults{Timeout: "3s", CacheTTL: "2h", StaleTTL: "48h", Retries: intPtr(5), RetryBackoff: "1s"},
		Sources:  map[string]SourceConfig{"s": {Type: "http", URL: "https://a.example.com/x.json", Timeout: "7s"}},
	}
	sources, err := Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := sources[0]
	if s.Timeout != 7*time.Second || s.CacheTTL != 2*time.Hour || s.StaleTTL != 48*time.Hour ||
		s.Retries != 5 || s.RetryBackoff != time.Second {
		t.Fatalf("layered durations = %+v", s)
	}
}

func intPtr(n int) *int { return &n }
