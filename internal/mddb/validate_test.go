package mddb

// Asking before storing (#192).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateCarriesTheWarningBack — the whole point. The render path already
// recognises a Go-stringified structure; this is the same finding delivered
// while the producer still has the source in hand.
func TestValidateCarriesTheWarningBack(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/validate" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":["meta.faq: looks like a Go-stringified list of objects"]}`))
	}))
	defer srv.Close()

	got, unsupported, err := NewClient(Config{BaseURL: srv.URL}).
		Validate("site", map[string][]string{"faq": {"[map[answer:Yes question:Is it free?]]"}})
	if err != nil || unsupported {
		t.Fatalf("validate: err=%v unsupported=%v", err, unsupported)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "meta.faq") {
		t.Errorf("warnings = %v", got.Warnings)
	}
	// A warning must never make the document invalid: `map[…]` is a valid
	// string, and MDDB deliberately does not reject it.
	if !got.Valid {
		t.Error("a warning must not invalidate the document")
	}
	if sent["collection"] != "site" {
		t.Errorf("request = %v", sent)
	}
}

// TestAnOlderServerIsSkippedSilently: /v1/validate arrived in MDDB 2.12.0. A
// site pointed at an older one must keep working exactly as before — not fail
// every write, and not print a complaint per document.
func TestAnOlderServerIsSkippedSilently(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, unsupported, err := NewClient(Config{BaseURL: srv.URL}).Validate("site", nil)
	if err != nil {
		t.Fatalf("a missing endpoint must not be an error: %v", err)
	}
	if !unsupported {
		t.Error("a 404 must be reported as unsupported")
	}
}

// TestErrorsComeBackToo, so a schema failure is not swallowed by the warning
// path that was added beside it.
func TestErrorsComeBackToo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"valid":false,"errors":["meta.kind: required"],"warnings":["meta.faq: stringified"]}`))
	}))
	defer srv.Close()

	got, _, err := NewClient(Config{BaseURL: srv.URL}).Validate("site", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Valid || len(got.Errors) != 1 || len(got.Warnings) != 1 {
		t.Errorf("result = %+v", got)
	}
}

// TestValidateRefusesAnIncompleteCallAndReportsFailures.
func TestValidateRefusesAnIncompleteCallAndReportsFailures(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if _, _, err := c.Validate("", nil); err == nil {
		t.Error("a call with no collection must be refused before the network")
	}
	if _, _, err := c.Validate("site", nil); err == nil {
		t.Error("an unreachable server must be an error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	if _, _, err := NewClient(Config{BaseURL: srv.URL}).Validate("site", nil); err == nil {
		t.Error("a malformed answer must be an error")
	}
}
