package fetch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		body, err := Bytes(srv.URL+"/ok", Auth{Type: "bearer", Token: "tok"}, 0)
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
		if _, err := Bytes(srv.URL+"/ok", Auth{Type: "header", Header: "X-Api-Key", Value: "k"}, 0); err != nil {
			t.Fatal(err)
		}
		if gotKey != "k" {
			t.Errorf("X-Api-Key = %q", gotKey)
		}
	})
	t.Run("size cap", func(t *testing.T) {
		if _, err := Bytes(srv.URL+"/big", Auth{}, 1024); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("oversize not rejected: %v", err)
		}
	})
	t.Run("non-200 is an error", func(t *testing.T) {
		if _, err := Bytes(srv.URL+"/404", Auth{}, 0); err == nil || !strings.Contains(err.Error(), "404") {
			t.Errorf("404 not reported: %v", err)
		}
	})
	t.Run("query string kept out of errors", func(t *testing.T) {
		_, err := Bytes(srv.URL+"/404?token=leakme", Auth{}, 0)
		if err == nil || strings.Contains(err.Error(), "leakme") {
			t.Errorf("query leaked into error: %v", err)
		}
	})
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
