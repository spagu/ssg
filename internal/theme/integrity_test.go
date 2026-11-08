package theme

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveZip serves body for every request and returns the server.
func serveZip(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDownloadVerifiesSHA256 covers SEC-005: a matching sum (any case, with
// stray whitespace) extracts the theme; a wrong one extracts nothing.
func TestDownloadVerifiesSHA256(t *testing.T) {
	body := createZipBytes(t, map[string]string{"theme-main/index.html": "<html></html>"})
	sum := sha256.Sum256(body)
	good := hex.EncodeToString(sum[:])
	srv := serveZip(t, body)

	dest := filepath.Join(t.TempDir(), "ok")
	if err := Download(srv.URL+"/theme.zip", dest, " "+strings.ToUpper(good)+"\n"); err != nil {
		t.Fatalf("matching sum rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "index.html")); err != nil {
		t.Error("verified theme not extracted")
	}

	dest = filepath.Join(t.TempDir(), "bad")
	err := Download(srv.URL+"/theme.zip", dest, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want a checksum mismatch, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("a mismatched archive must not be extracted")
	}
}

// TestVerifySHA256 checks the empty expectation passes (with a warning).
func TestVerifySHA256(t *testing.T) {
	if err := verifySHA256("abc", ""); err != nil {
		t.Errorf("empty expectation must pass: %v", err)
	}
	if err := verifySHA256("abc", "ABC"); err != nil {
		t.Errorf("case must not matter: %v", err)
	}
	if err := verifySHA256("abc", "abd"); err == nil {
		t.Error("a different sum must fail")
	}
}

// TestConvertToArchiveURLPinnedRef covers "repo@ref" pinning (SEC-005).
func TestConvertToArchiveURLPinnedRef(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://github.com/user/theme@v1.2.0", "https://github.com/user/theme/archive/v1.2.0.zip"},
		{"https://github.com/user/theme.git@0a1b2c3", "https://github.com/user/theme/archive/0a1b2c3.zip"},
		{"https://github.com/user/theme@v1/", "https://github.com/user/theme/archive/v1.zip"},
		{"https://gitlab.com/user/theme@v2", "https://gitlab.com/user/theme/-/archive/v2/archive.zip"},
		{"https://gitlab.com/user/theme", "https://gitlab.com/user/theme/-/archive/main/archive.zip"},
		{"https://github.com/user/theme@", "https://github.com/user/theme@/archive/refs/heads/main.zip"},
		{"https://git@github.com/user/theme", "https://git@github.com/user/theme/archive/refs/heads/main.zip"},
		{"https://example.com/theme@v1", "https://example.com/theme@v1"},
	}
	for _, tt := range tests {
		if got := convertToArchiveURL(tt.in); got != tt.want {
			t.Errorf("convertToArchiveURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// A pinned ref has no master fallback: the ref is what was asked for.
	if got := masterFallbackURL(convertToArchiveURL("https://github.com/user/theme@main")); got != "" {
		t.Errorf("pinned ref must not fall back, got %q", got)
	}
}
