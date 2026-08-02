package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func recvServer(hits *int, payloads *[]Post) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		b, _ := io.ReadAll(r.Body)
		var p Post
		_ = json.Unmarshal(b, &p)
		*payloads = append(*payloads, p)
		w.WriteHeader(http.StatusOK)
	}))
}

// TestSendDedup: a new post is delivered once; an unchanged re-run sends nothing;
// a changed hash re-announces.
func TestSendDedup(t *testing.T) {
	var hits int
	var got []Post
	srv := recvServer(&hits, &got)
	defer srv.Close()

	state := filepath.Join(t.TempDir(), "notif.json")
	n := New([]Dest{{Name: "hook", URL: srv.URL, AllowPrivate: true}}, state)

	posts := []Post{{Slug: "a", Title: "A", URL: "/a/", Hash: "h1"}}
	if sent, err := n.Send(posts, true); err != nil || sent != 1 {
		t.Fatalf("first send = %d, %v", sent, err)
	}
	if hits != 1 || got[0].Slug != "a" || got[0].Title != "A" {
		t.Fatalf("delivery wrong: hits %d, payload %+v", hits, got)
	}
	// Same hash → nothing sent.
	if sent, _ := n.Send(posts, true); sent != 0 {
		t.Errorf("unchanged post must not resend, sent %d", sent)
	}
	if hits != 1 {
		t.Errorf("endpoint hit %d times, want 1", hits)
	}
	// Changed hash → re-announced.
	posts[0].Hash = "h2"
	if sent, _ := n.Send(posts, true); sent != 1 || hits != 2 {
		t.Errorf("changed post must resend: sent %d, hits %d", sent, hits)
	}
	// A fresh notifier reads the committed state — still deduped.
	n2 := New([]Dest{{Name: "hook", URL: srv.URL, AllowPrivate: true}}, state)
	if sent, _ := n2.Send(posts, true); sent != 0 || hits != 2 {
		t.Errorf("persisted state must dedupe: sent %d, hits %d", sent, hits)
	}
}

// TestSendFailureRetries: a destination that 5xxs is not recorded, so the post is
// retried on the next run.
func TestSendFailureRetries(t *testing.T) {
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fail.Close()
	n := New([]Dest{{Name: "bad", URL: fail.URL, AllowPrivate: true}}, filepath.Join(t.TempDir(), "s.json"))
	posts := []Post{{Slug: "a", Hash: "h1"}}
	if sent, _ := n.Send(posts, true); sent != 0 {
		t.Errorf("failed delivery must not count as sent, got %d", sent)
	}
	// Not recorded → retried (still 0 sent, but attempted again without panic).
	if sent, _ := n.Send(posts, true); sent != 0 {
		t.Errorf("still failing, sent %d", sent)
	}
}

// TestSSRFBlocked: without allow_private a loopback destination is refused at
// dial time, so the post is not recorded.
func TestSSRFBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	n := New([]Dest{{Name: "x", URL: srv.URL}}, filepath.Join(t.TempDir(), "s.json")) // AllowPrivate false
	if sent, _ := n.Send([]Post{{Slug: "a", Hash: "h"}}, true); sent != 0 {
		t.Errorf("loopback destination must be blocked, sent %d", sent)
	}
}

// TestEnabledAndDefaults: Enabled reflects destinations; empty statePath defaults.
func TestEnabledAndDefaults(t *testing.T) {
	if New([]Dest{{URL: "x"}}, "").statePath != ".ssg-notifications.json" {
		t.Error("empty statePath must default")
	}
	if !New([]Dest{{URL: "x"}}, "s").Enabled() {
		t.Error("with a destination Enabled must be true")
	}
	if New(nil, "s").Enabled() {
		t.Error("no destinations ⇒ not enabled")
	}
	var nilN *Notifier
	if nilN.Enabled() {
		t.Error("nil notifier must be disabled")
	}
}

// TestHeadersEnvAndBadState: $ENV header values are expanded; a malformed state
// file surfaces as an error.
func TestHeadersEnvAndBadState(t *testing.T) {
	t.Setenv("NOTIFY_TOKEN", "abc123")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer srv.Close()
	n := New([]Dest{{Name: "h", URL: srv.URL, AllowPrivate: true,
		Headers: map[string]string{"Authorization": "$NOTIFY_TOKEN"}}}, filepath.Join(t.TempDir(), "s.json"))
	if _, err := n.Send([]Post{{Slug: "a", Hash: "h"}}, true); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotAuth != "abc123" {
		t.Errorf("header $ENV not expanded, got %q", gotAuth)
	}

	// Malformed state file → error.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New([]Dest{{URL: srv.URL}}, bad).Send(nil, true); err == nil {
		t.Error("malformed state must error")
	}
}
