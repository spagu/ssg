package externalsource

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// pagedSource builds a resolved paginated HTTP source over a test server.
func pagedSource(name, url string, p PaginationConfig) Source {
	src := httpSource(name, url, "json")
	src.Pagination = p
	return src
}

// jsonHandler wraps a page-serving function with the JSON content type.
func jsonHandler(serve func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		serve(w, r)
	})
}

func TestHTTPPaginationModePage(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	pages := map[string]string{"1": `[1,2]`, "2": `[3,4]`, "3": `[5]`, "4": `[]`}
	srv := httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Query().Get("p")+"/"+r.URL.Query().Get("limit"))
		mu.Unlock()
		_, _ = fmt.Fprint(w, pages[r.URL.Query().Get("p")])
	}))
	defer srv.Close()

	src := pagedSource("api", srv.URL+"/items.json", PaginationConfig{
		Mode: "page", Param: "p", StartPage: 1, PerPage: 2, PerPageParam: "limit", MaxPages: 10})
	conn := testConnector(t)
	res, err := conn.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	data, ok := res.Data.([]interface{})
	if !ok || len(data) != 5 || res.Metadata.RecordCount != 5 {
		t.Fatalf("aggregated data = %#v meta = %+v", res.Data, res.Metadata)
	}
	// The page counter and per_page ride along on every request, in order.
	want := []string{"1/2", "2/2", "3/2", "4/2"}
	if len(seen) != 4 || strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Fatalf("requests = %v, want %v", seen, want)
	}
	// The aggregate is cached as ONE entry under the source key.
	res2, err := conn.Load(src)
	if err != nil || !res2.Metadata.FromCache || res2.Metadata.RecordCount != 5 {
		t.Fatalf("cached aggregate = %+v, %v", res2.Metadata, err)
	}
	if len(seen) != 4 {
		t.Fatalf("cache hit must not refetch: %d requests", len(seen))
	}
}

func TestHTTPPaginationModePageStartPage(t *testing.T) {
	pages := map[string]string{"5": `["a"]`, "6": `[]`}
	srv := httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, pages[r.URL.Query().Get("page")])
	}))
	defer srv.Close()
	src := pagedSource("api", srv.URL, PaginationConfig{
		Mode: "page", Param: "page", StartPage: 5, PerPageParam: "per_page", MaxPages: 10})
	res, err := testConnector(t).Load(src)
	if err != nil || res.Metadata.RecordCount != 1 {
		t.Fatalf("start_page fetch = %+v, %v", res, err)
	}
}

func TestHTTPPaginationModeLink(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/items":
			w.Header().Set("Link", "<"+srv.URL+"/items2>; rel=\"next\"")
			_, _ = fmt.Fprint(w, `["a"]`)
		case "/items2":
			// Relative next targets resolve against the current page URL.
			w.Header().Set("Link", `</items3>; rel="next", <`+srv.URL+`/items1>; rel="prev"`)
			_, _ = fmt.Fprint(w, `["b"]`)
		case "/items3":
			_, _ = fmt.Fprint(w, `["c"]`) // no Link header → natural stop
		}
	}))
	defer srv.Close()

	src := pagedSource("api", srv.URL+"/items", PaginationConfig{
		Mode: "link", Param: "page", StartPage: 1, PerPageParam: "per_page", MaxPages: 10})
	res, err := testConnector(t).Load(src)
	if err != nil {
		t.Fatal(err)
	}
	data := res.Data.([]interface{})
	if len(data) != 3 || data[0] != "a" || data[2] != "c" {
		t.Fatalf("link aggregation = %#v", data)
	}
}

func TestHTTPPaginationMaxPagesGuard(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = fmt.Fprint(w, `["x"]`) // never-ending feed
	}))
	defer srv.Close()
	src := pagedSource("api", srv.URL, PaginationConfig{
		Mode: "page", Param: "page", StartPage: 1, PerPageParam: "per_page", MaxPages: 3})
	res, err := testConnector(t).Load(src)
	if err != nil || hits != 3 || res.Metadata.RecordCount != 3 {
		t.Fatalf("max_pages guard: hits=%d meta=%+v err=%v", hits, res.Metadata, err)
	}
}

func TestHTTPPaginationNonArrayResponses(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		switch r.URL.Path {
		case "/object":
			_, _ = fmt.Fprint(w, `{"items":[1,2]}`)
		case "/mixed":
			if r.URL.Query().Get("page") == "1" {
				_, _ = fmt.Fprint(w, `["a"]`)
			} else {
				_, _ = fmt.Fprint(w, `{"done":true}`)
			}
		case "/empty-body":
			if r.URL.Query().Get("page") == "2" {
				return // 200 with an empty body → natural stop
			}
			_, _ = fmt.Fprint(w, `["a"]`)
		}
	}))
	defer srv.Close()
	conn := testConnector(t)
	p := PaginationConfig{Mode: "page", Param: "page", StartPage: 1, PerPageParam: "per_page", MaxPages: 10}

	// A non-array first page is kept verbatim, with a warning, after one request.
	res, err := conn.Load(pagedSource("obj", srv.URL+"/object", p))
	if err != nil || hits != 1 {
		t.Fatalf("object page: hits=%d err=%v", hits, err)
	}
	if _, ok := res.Data.(map[string]interface{}); !ok {
		t.Fatalf("object payload = %#v", res.Data)
	}
	// A non-array later page stops pagination, keeping the collected items.
	hits = 0
	res, err = conn.Load(pagedSource("mixed", srv.URL+"/mixed", p))
	if err != nil || hits != 2 || res.Metadata.RecordCount != 1 {
		t.Fatalf("mixed pages: hits=%d meta=%+v err=%v", hits, res.Metadata, err)
	}
	// An empty body stops pagination.
	hits = 0
	res, err = conn.Load(pagedSource("empty", srv.URL+"/empty-body", p))
	if err != nil || hits != 2 || res.Metadata.RecordCount != 1 {
		t.Fatalf("empty body: hits=%d meta=%+v err=%v", hits, res.Metadata, err)
	}
}

func TestHTTPPaginationInvalidJSONPage(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{broken`)
	}))
	defer srv.Close()
	src := pagedSource("api", srv.URL, PaginationConfig{
		Mode: "page", Param: "page", StartPage: 1, PerPageParam: "per_page", MaxPages: 10})
	if _, err := testConnector(t).Load(src); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("invalid JSON page = %v", err)
	}
}

func TestHTTPPaginationLinkSecurity(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<https://evil.example.net/steal>; rel="next"`)
		_, _ = fmt.Fprint(w, `["a"]`)
	}))
	defer srv.Close()
	conn := testConnector(t)
	conn.allowedHosts = []string{"127.0.0.1"}
	src := pagedSource("api", srv.URL, PaginationConfig{
		Mode: "link", Param: "page", StartPage: 1, PerPageParam: "per_page", MaxPages: 10})
	if _, err := conn.Load(src); err == nil || !strings.Contains(err.Error(), "allowed_hosts") {
		t.Fatalf("hostile Link target must be blocked: %v", err)
	}
}

func TestNextLinkURL(t *testing.T) {
	cases := []struct{ header, want string }{
		{`<https://x/2>; rel="next"`, "https://x/2"},
		{`<https://x/1>; rel="prev", <https://x/2>; rel="next"`, "https://x/2"},
		{`<https://x/2>; rel=next`, "https://x/2"},
		{`<https://x/2>; rel='next'`, "https://x/2"},
		{`<https://x/2>; rel="prev"`, ""},
		{`<https://x/2>`, ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := nextLinkURL(c.header); got != c.want {
			t.Errorf("nextLinkURL(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestPaginationResolveMatrix(t *testing.T) {
	httpCfg := func(p PaginationConfig, format string) Config {
		return Config{Sources: map[string]SourceConfig{"s": {
			Type: "http", URL: "https://a.example.com/x." + format, Pagination: p}}}
	}
	bad := map[string]Config{
		"bad mode":        httpCfg(PaginationConfig{Mode: "cursor"}, "json"),
		"missing mode":    httpCfg(PaginationConfig{MaxPages: 5}, "json"),
		"max too high":    httpCfg(PaginationConfig{Mode: "page", MaxPages: 1001}, "json"),
		"max negative":    httpCfg(PaginationConfig{Mode: "page", MaxPages: -1}, "json"),
		"neg per_page":    httpCfg(PaginationConfig{Mode: "page", PerPage: -1}, "json"),
		"neg start_page":  httpCfg(PaginationConfig{Mode: "page", StartPage: -1}, "json"),
		"non-json format": httpCfg(PaginationConfig{Mode: "page"}, "csv"),
	}
	for label, cfg := range bad {
		if _, err := Resolve(cfg); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
	// Defaults fill in: param, per_page_param, start_page, max_pages.
	sources, err := Resolve(httpCfg(PaginationConfig{Mode: "link"}, "json"))
	if err != nil {
		t.Fatal(err)
	}
	p := sources[0].Pagination
	if p.Mode != "link" || p.Param != "page" || p.PerPageParam != "per_page" ||
		p.StartPage != 1 || p.MaxPages != defaultMaxPages || p.PerPage != 0 {
		t.Fatalf("pagination defaults = %+v", p)
	}
	// Without a pagination block the source resolves to single-request mode.
	sources, err = Resolve(httpCfg(PaginationConfig{}, "json"))
	if err != nil || sources[0].Pagination.Mode != "" {
		t.Fatalf("no pagination = %+v, %v", sources[0].Pagination, err)
	}
}

func TestPaginationCacheKey(t *testing.T) {
	plain := httpSource("api", "https://api.example.com/a.json", "json")
	paged := pagedSource("api", "https://api.example.com/a.json", PaginationConfig{
		Mode: "page", Param: "page", StartPage: 1, PerPageParam: "per_page", MaxPages: 10})
	if cacheKey(plain) == cacheKey(paged) {
		t.Fatal("pagination must change the cache key")
	}
	other := paged
	other.Pagination.MaxPages = 20
	if cacheKey(paged) == cacheKey(other) {
		t.Fatal("different max_pages must not share a key")
	}
	same := pagedSource("api", "https://api.example.com/a.json", paged.Pagination)
	if cacheKey(paged) != cacheKey(same) {
		t.Fatal("identical paginated configs must share a key")
	}
}
