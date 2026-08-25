package mddb

// Fused keyword-and-vector search (#207).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHybridAsksTheRightEndpointAndKeepsTheScoresApart. The scores answer
// different questions, and a hit with a vector score and no keyword score is
// exactly the one the lexical path could never have found.
func TestHybridAsksTheRightEndpointAndKeepsTheScoresApart(t *testing.T) {
	var path string
	var sent HybridRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_, _ = w.Write([]byte(`{"results":[
			{"document":{"key":"templates/base.html","contentMd":"<nav>"},
			 "combinedScore":0.81,"ftsScore":0.0,"vectorScore":0.81,"rank":1}],"total":1}`))
	}))
	defer srv.Close()

	hits, unsupported, err := NewClient(Config{BaseURL: srv.URL}).HybridSearch(HybridRequest{
		Collection: "theme", Query: "how the navigation looks on phones", TopK: 5, IncludeContent: true,
	})
	if err != nil || unsupported {
		t.Fatalf("err=%v unsupported=%v", err, unsupported)
	}
	if path != "/v1/hybrid-search" {
		t.Errorf("path = %q", path)
	}
	if !sent.IncludeContent {
		t.Error("content must be asked for, or a hit with no lexical match cannot even be labelled")
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits", len(hits))
	}
	h := hits[0]
	if h.Document.Key != "templates/base.html" || h.VectorScore != 0.81 || h.FTSScore != 0 {
		t.Errorf("hit = %+v", h)
	}
}

// TestACollectionWithoutVectorsIsNotAnError: it is an ordinary collection, and
// the caller is expected to fall back rather than report a failure.
func TestACollectionWithoutVectorsIsNotAnError(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusBadRequest} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, unsupported, err := NewClient(Config{BaseURL: srv.URL}).
			HybridSearch(HybridRequest{Collection: "theme", Query: "q"})
		srv.Close()
		if err != nil {
			t.Errorf("status %d must not be an error: %v", status, err)
		}
		if !unsupported {
			t.Errorf("status %d must be reported as unsupported", status)
		}
	}
}

// TestARealFailureIsNotDowngradedToUnsupported. Quietly treating a 500 or a 401
// as "no vectors here" would hide a broken server behind a silent fallback,
// which is how an outage becomes a mystery.
func TestARealFailureIsNotDowngradedToUnsupported(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusUnauthorized, http.StatusBadGateway} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, unsupported, err := NewClient(Config{BaseURL: srv.URL}).
			HybridSearch(HybridRequest{Collection: "theme", Query: "q"})
		srv.Close()
		if err == nil {
			t.Errorf("status %d must be an error", status)
		}
		if unsupported {
			t.Errorf("status %d must not be reported as unsupported", status)
		}
	}
	if isUnsupported(nil) {
		t.Error("no error is not unsupported")
	}
}

// TestHybridRefusesAnIncompleteQuery and reports a malformed answer.
func TestHybridRefusesAnIncompleteQuery(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if _, _, err := c.HybridSearch(HybridRequest{Query: "x"}); err == nil {
		t.Error("no collection must be refused before the network")
	}
	if _, _, err := c.HybridSearch(HybridRequest{Collection: "t"}); err == nil {
		t.Error("no query must be refused")
	}
	if _, _, err := c.HybridSearch(HybridRequest{Collection: "t", Query: "q"}); err == nil {
		t.Error("an unreachable server must be an error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	if _, _, err := NewClient(Config{BaseURL: srv.URL}).
		HybridSearch(HybridRequest{Collection: "t", Query: "q"}); err == nil {
		t.Error("a malformed answer must be an error")
	}
}
