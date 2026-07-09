// Package mddb - tests for the SEC-009 bounded response reads.
package mddb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClient_ErrorBodyBounded verifies SEC-009: an oversized error body is read
// through an io.LimitReader, so the resulting error message stays bounded
// instead of pulling the whole (potentially huge) response into memory.
func TestClient_ErrorBodyBounded(t *testing.T) {
	const bodyLen = 4 * maxErrBodySize // far larger than the cap
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", bodyLen)))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.Get(GetRequest{Collection: "blog", Key: "k"})
	if err == nil {
		t.Fatal("expected an error for HTTP 500")
	}

	// The message embeds the (truncated) body; it must be capped, not full-size.
	if len(err.Error()) > maxErrBodySize+256 {
		t.Errorf("error message length %d exceeds bounded cap ~%d; body was not limited",
			len(err.Error()), maxErrBodySize)
	}
}
