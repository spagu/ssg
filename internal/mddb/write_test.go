package mddb

// Writing to a collection, and asking it a question in words (#190).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordingServer answers every request with body and records what it received.
func recordingServer(t *testing.T, status int, body string) (*httptest.Server, *[]*http.Request, *[]string) {
	t.Helper()
	var reqs []*http.Request
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(strings.Builder)
		if r.Body != nil {
			b := make([]byte, 4096)
			n, _ := r.Body.Read(b)
			buf.Write(b[:n])
		}
		reqs = append(reqs, r)
		bodies = append(bodies, buf.String())
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs, &bodies
}

// TestAddSendsAnUpsert: /v1/add replaces on the same key, which is what makes a
// theme sync "push everything" rather than "diff, then update or insert".
func TestAddSendsAnUpsert(t *testing.T) {
	srv, reqs, bodies := recordingServer(t, http.StatusOK, `{}`)
	c := NewClient(Config{BaseURL: srv.URL})

	err := c.Add(AddRequest{
		Collection: "theme", Key: "static/css/style.css", Lang: "en",
		ContentMD: "body{}", Meta: map[string][]string{"kind": {"style"}},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := (*reqs)[0].URL.Path; got != "/v1/add" {
		t.Errorf("path = %q", got)
	}
	var sent AddRequest
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Key != "static/css/style.css" || sent.ContentMD != "body{}" {
		t.Errorf("sent = %+v", sent)
	}
	if sent.Meta["kind"][0] != "style" {
		t.Errorf("metadata lost: %+v", sent.Meta)
	}
}

// TestDeleteSendsTheKey.
func TestDeleteSendsTheKey(t *testing.T) {
	srv, reqs, bodies := recordingServer(t, http.StatusOK, `{}`)
	c := NewClient(Config{BaseURL: srv.URL})

	if err := c.Delete(DeleteRequest{Collection: "theme", Key: "gone.css", Lang: "en"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := (*reqs)[0].URL.Path; got != "/v1/delete" {
		t.Errorf("path = %q", got)
	}
	if !strings.Contains((*bodies)[0], "gone.css") {
		t.Errorf("body = %s", (*bodies)[0])
	}
}

// TestIncompleteWritesAreRefusedBeforeTheNetwork: a request with no key would
// be a server-side error at best, and at worst something else's document.
func TestIncompleteWritesAreRefusedBeforeTheNetwork(t *testing.T) {
	srv, reqs, _ := recordingServer(t, http.StatusOK, `{}`)
	c := NewClient(Config{BaseURL: srv.URL})

	if err := c.Add(AddRequest{Collection: "theme"}); err == nil {
		t.Error("add without a key must be refused")
	}
	if err := c.Add(AddRequest{Key: "k"}); err == nil {
		t.Error("add without a collection must be refused")
	}
	if err := c.Delete(DeleteRequest{Collection: "theme"}); err == nil {
		t.Error("delete without a key must be refused")
	}
	if err := c.Delete(DeleteRequest{Key: "k"}); err == nil {
		t.Error("delete without a collection must be refused")
	}
	if len(*reqs) != 0 {
		t.Errorf("%d request(s) reached the server", len(*reqs))
	}
}

// TestAServerErrorSurfaces rather than being reported as a successful write.
func TestAServerErrorSurfaces(t *testing.T) {
	srv, _, _ := recordingServer(t, http.StatusInternalServerError, `boom`)
	c := NewClient(Config{BaseURL: srv.URL})

	if err := c.Add(AddRequest{Collection: "t", Key: "k"}); err == nil {
		t.Error("a 500 must be an error")
	}
	if err := c.Delete(DeleteRequest{Collection: "t", Key: "k"}); err == nil {
		t.Error("a 500 must be an error")
	}
}

// TestFTSAsksTheRightEndpointAndDecodesScores: /v1/search filters on metadata;
// /v1/fts is the one that answers a question phrased as a sentence, which is
// the entire reason this exists beside Search.
func TestFTSAsksTheRightEndpointAndDecodesScores(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"results":[
			{"document":{"key":"static/css/style.css","contentMd":"body { background: #fff; }"},"score":0.91},
			{"document":{"key":"templates/base.html","contentMd":"<body>"},"score":0.42}],"total":2}`))
	}))
	defer srv.Close()

	hits, err := NewClient(Config{BaseURL: srv.URL}).FTS(FTSRequest{
		Collection: "theme", Query: "background colour of the page", Limit: 5,
	})
	if err != nil {
		t.Fatalf("fts: %v", err)
	}
	if path != "/v1/fts" {
		t.Errorf("path = %q", path)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits", len(hits))
	}
	if hits[0].Document.Key != "static/css/style.css" || hits[0].Score != 0.91 {
		t.Errorf("first hit = %+v", hits[0])
	}
	if !strings.Contains(hits[0].Document.Content, "background") {
		t.Errorf("content not mapped: %q", hits[0].Document.Content)
	}
	if hits[0].Document.Collection != "theme" {
		t.Errorf("collection = %q", hits[0].Document.Collection)
	}
}

// TestFTSRefusesAnIncompleteQuery before spending a round trip.
func TestFTSRefusesAnIncompleteQuery(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if _, err := c.FTS(FTSRequest{Query: "x"}); err == nil {
		t.Error("a query with no collection must be refused")
	}
	if _, err := c.FTS(FTSRequest{Collection: "theme"}); err == nil {
		t.Error("an empty query must be refused")
	}
}

// TestFTSReportsTransportAndDecodeFailures instead of returning no hits, which
// a caller would read as "not found".
func TestFTSReportsTransportAndDecodeFailures(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "http://127.0.0.1:1"}).
		FTS(FTSRequest{Collection: "t", Query: "q"}); err == nil {
		t.Error("an unreachable server must be an error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	if _, err := NewClient(Config{BaseURL: srv.URL}).
		FTS(FTSRequest{Collection: "t", Query: "q"}); err == nil {
		t.Error("a malformed body must be an error")
	}
}
