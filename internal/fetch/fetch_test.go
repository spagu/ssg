package fetch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A transient failure (5xx) is retried; a 4xx is not.
func TestBytes_RetryPolicy(t *testing.T) {
	t.Run("retries 5xx then succeeds", func(t *testing.T) {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("ok"))
		}))
		defer srv.Close()
		body, err := Bytes(srv.URL, Auth{}, 0, Options{Retries: 3, RetryDelay: time.Millisecond})
		if err != nil || string(body) != "ok" {
			t.Fatalf("retry-then-succeed: body=%q err=%v (calls=%d)", body, err, calls)
		}
		if calls != 3 {
			t.Fatalf("expected 3 attempts, got %d", calls)
		}
	})

	t.Run("does not retry a 404", func(t *testing.T) {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		if _, err := Bytes(srv.URL, Auth{}, 0, Options{Retries: 5, RetryDelay: time.Millisecond}); err == nil {
			t.Fatal("expected an error for 404")
		}
		if calls != 1 {
			t.Fatalf("a 404 must not be retried, got %d attempts", calls)
		}
	})

	t.Run("gives up after exhausting retries", func(t *testing.T) {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		if _, err := Bytes(srv.URL, Auth{}, 0, Options{Retries: 2, RetryDelay: time.Millisecond}); err == nil {
			t.Fatal("expected an error after retries")
		}
		if calls != 3 { // 1 initial + 2 retries
			t.Fatalf("expected 3 attempts, got %d", calls)
		}
	})
}

func TestExpandAuth(t *testing.T) {
	t.Setenv("TK", "secret-token")
	t.Setenv("PW", "secret-pass")
	t.Setenv("KEY", "api-key-value")
	t.Setenv("EMPTY", "")

	t.Run("bearer resolves from env", func(t *testing.T) {
		got, err := ExpandAuth(Auth{Type: "bearer", Token: "$TK"})
		if err != nil || got.Token != "secret-token" {
			t.Fatalf("ExpandAuth bearer = %+v, %v", got, err)
		}
	})
	t.Run("header value resolves, header name passes", func(t *testing.T) {
		got, err := ExpandAuth(Auth{Type: "header", Header: "X-Api-Key", Value: "${KEY}"})
		if err != nil || got.Value != "api-key-value" || got.Header != "X-Api-Key" {
			t.Fatalf("ExpandAuth header = %+v, %v", got, err)
		}
	})
	t.Run("basic: password from env, username plain", func(t *testing.T) {
		got, err := ExpandAuth(Auth{Type: "basic", Username: "user", Password: "$PW"})
		if err != nil || got.Password != "secret-pass" || got.Username != "user" {
			t.Fatalf("ExpandAuth basic = %+v, %v", got, err)
		}
	})
	t.Run("literal secret is rejected", func(t *testing.T) {
		if _, err := ExpandAuth(Auth{Type: "bearer", Token: "hardcoded"}); err == nil ||
			!strings.Contains(err.Error(), "environment variable") {
			t.Fatalf("literal token accepted: %v", err)
		}
	})
	t.Run("unset variable names itself", func(t *testing.T) {
		if _, err := ExpandAuth(Auth{Type: "bearer", Token: "$NOPE"}); err == nil ||
			!strings.Contains(err.Error(), "NOPE") {
			t.Fatalf("unset var not named: %v", err)
		}
	})
	t.Run("empty (no auth) is fine", func(t *testing.T) {
		if _, err := ExpandAuth(Auth{}); err != nil {
			t.Fatalf("empty auth errored: %v", err)
		}
	})
}

func TestBytesSendsAuthAndCaps(t *testing.T) {
	var gotAuth, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("X-Api-Key")
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("workers: []\n"))
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
		case "/404":
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Run("bearer header is sent, body returned", func(t *testing.T) {
		body, err := Bytes(srv.URL+"/ok", Auth{Type: "bearer", Token: "tok"}, 0, Options{})
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if gotAuth != "Bearer tok" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok")
		}
		if string(body) != "workers: []\n" {
			t.Errorf("body = %q", body)
		}
	})
	t.Run("header auth", func(t *testing.T) {
		if _, err := Bytes(srv.URL+"/ok", Auth{Type: "header", Header: "X-Api-Key", Value: "k"}, 0, Options{}); err != nil {
			t.Fatal(err)
		}
		if gotKey != "k" {
			t.Errorf("X-Api-Key = %q", gotKey)
		}
	})
	t.Run("size cap", func(t *testing.T) {
		if _, err := Bytes(srv.URL+"/big", Auth{}, 1024, Options{}); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("oversize not rejected: %v", err)
		}
	})
	t.Run("non-200 is an error", func(t *testing.T) {
		if _, err := Bytes(srv.URL+"/404", Auth{}, 0, Options{}); err == nil || !strings.Contains(err.Error(), "404") {
			t.Errorf("404 not reported: %v", err)
		}
	})
	t.Run("query string kept out of errors", func(t *testing.T) {
		_, err := Bytes(srv.URL+"/404?token=leakme", Auth{}, 0, Options{})
		if err == nil || strings.Contains(err.Error(), "leakme") {
			t.Errorf("query leaked into error: %v", err)
		}
	})
}

// A configured server must not be able to bounce the auth credential to another
// host via a redirect (credential exfiltration, SEC).
func TestBytes_StripsAuthOnCrossHostRedirect(t *testing.T) {
	var attackerSawKey, attackerSawAuth string
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerSawKey = r.Header.Get("X-Api-Key")
		attackerSawAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer attacker.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/", http.StatusFound)
	}))
	defer origin.Close()

	t.Run("header auth is not forwarded off-origin", func(t *testing.T) {
		attackerSawKey, attackerSawAuth = "", ""
		if _, err := Bytes(origin.URL+"/", Auth{Type: "header", Header: "X-Api-Key", Value: "SECRET"}, 0, Options{}); err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if attackerSawKey != "" {
			t.Fatalf("custom auth header leaked to redirect host: %q", attackerSawKey)
		}
	})

	t.Run("bearer auth is not forwarded off-origin", func(t *testing.T) {
		attackerSawKey, attackerSawAuth = "", ""
		if _, err := Bytes(origin.URL+"/", Auth{Type: "bearer", Token: "SECRET"}, 0, Options{}); err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if attackerSawAuth != "" {
			t.Fatalf("Authorization leaked to redirect host: %q", attackerSawAuth)
		}
	})
}

// A same-origin redirect must still carry the credential (the common
// http→canonical case), so auth is not stripped needlessly.
func TestBytes_KeepsAuthOnSameHostRedirect(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		got = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	if _, err := Bytes(srv.URL+"/start", Auth{Type: "header", Header: "X-Api-Key", Value: "SECRET"}, 0, Options{}); err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if got != "SECRET" {
		t.Fatalf("same-host redirect should keep the header, got %q", got)
	}
}

func TestSafeURL_RedactsUserinfoAndQuery(t *testing.T) {
	for raw, want := range map[string]string{
		"https://ghp_abc123@example.com/x.yaml":     "https://example.com/x.yaml",
		"https://u:p@example.com/x.yaml?token=leak": "https://example.com/x.yaml",
		"https://example.com/x.yaml?q=1":            "https://example.com/x.yaml",
		"https://example.com/x.yaml":                "https://example.com/x.yaml",
	} {
		if got := safeURL(raw); got != want {
			t.Errorf("safeURL(%q) = %q, want %q", raw, got, want)
		}
		if strings.Contains(safeURL(raw), "leak") || strings.Contains(safeURL(raw), "ghp_") || strings.Contains(safeURL(raw), ":p@") {
			t.Errorf("safeURL(%q) leaked a secret: %q", raw, safeURL(raw))
		}
	}
}

func TestIsURL(t *testing.T) {
	for s, want := range map[string]bool{
		"https://example.com/a.yaml": true,
		"http://example.com/a.yaml":  true,
		"workers/comments.yaml":      false,
		"/abs/path.yaml":             false,
		"./rel.yaml":                 false,
	} {
		if IsURL(s) != want {
			t.Errorf("IsURL(%q) = %v, want %v", s, IsURL(s), want)
		}
	}
}
