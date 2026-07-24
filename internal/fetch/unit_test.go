package fetch

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthApply(t *testing.T) {
	tests := []struct {
		name    string
		auth    Auth
		wantHdr map[string]string
		wantErr string
	}{
		{"none", Auth{}, nil, ""},
		{"bearer", Auth{Type: "bearer", Token: "t"}, map[string]string{"Authorization": "Bearer t"}, ""},
		{"header", Auth{Type: "header", Header: "X-Api-Key", Value: "k"}, map[string]string{"X-Api-Key": "k"}, ""},
		{"bearer no token", Auth{Type: "bearer"}, nil, "needs a token"},
		{"basic no user", Auth{Type: "basic"}, nil, "needs a username"},
		{"header no name", Auth{Type: "header"}, nil, "needs a header name"},
		{"unknown", Auth{Type: "oauth"}, nil, "unsupported auth type"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
			err := tc.auth.apply(req)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			for h, v := range tc.wantHdr {
				if got := req.Header.Get(h); got != v {
					t.Errorf("%s = %q, want %q", h, got, v)
				}
			}
		})
	}
}

func TestBasicAuthReachesServer(t *testing.T) {
	var u, p string
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok = r.BasicAuth()
		_, _ = w.Write([]byte("x: 1\n"))
	}))
	defer srv.Close()
	if _, err := Bytes(srv.URL, Auth{Type: "basic", Username: "me", Password: "pw"}, 0, Options{}); err != nil {
		t.Fatal(err)
	}
	if !ok || u != "me" || p != "pw" {
		t.Errorf("basic auth not received: %q/%q ok=%v", u, p, ok)
	}
}

func TestBytesInvalidURL(t *testing.T) {
	if _, err := Bytes("://not-a-url", Auth{}, 0, Options{}); err == nil {
		t.Fatal("malformed URL accepted")
	}
}

func TestArchiveSizeCap(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("big.txt")
	_, _ = w.Write(bytes.Repeat([]byte("x"), 4096))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	old := maxArchiveBytes
	maxArchiveBytes = 512
	defer func() { maxArchiveBytes = old }()
	if err := Archive(srv.URL+"/x.zip", Auth{}, filepath.Join(t.TempDir(), "w"), Options{}); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize archive not rejected: %v", err)
	}
}

func TestMasterFallback(t *testing.T) {
	cases := map[string]string{
		"https://github.com/u/r/archive/refs/heads/main.zip": "https://github.com/u/r/archive/refs/heads/master.zip",
		"https://gitlab.com/u/r/-/archive/main/archive.zip":  "https://gitlab.com/u/r/-/archive/master/archive.zip",
		"https://example.com/w.zip":                          "",
	}
	for in, want := range cases {
		if got := masterFallback(in); got != want {
			t.Errorf("masterFallback(%q) = %q, want %q", in, got, want)
		}
	}
}

// A zip whose entries do not share one top directory is extracted as-is (no
// wrapper stripped).
func TestArchiveNoWrapperWhenMixedTops(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"a/one.txt", "b/two.txt"} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte("x"))
	}
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "w")
	if err := Archive(srv.URL+"/x.zip", Auth{}, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"a/one.txt", "b/two.txt"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s kept as-is: %v", rel, err)
		}
	}
}
