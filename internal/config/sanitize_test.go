package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSanitizesHTML covers SEC-003: an MDDB build sanitizes by default, unless
// the server is marked trusted; sanitize_html still forces it for any source.
func TestSanitizesHTML(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"local content", Config{}, false},
		{"sanitize_html", Config{SanitizeHTML: true}, true},
		{"mddb untrusted by default", Config{Mddb: MddbConfig{Enabled: true}}, true},
		{"mddb trusted", Config{Mddb: MddbConfig{Enabled: true, Trusted: true}}, false},
		{"mddb trusted but forced", Config{SanitizeHTML: true, Mddb: MddbConfig{Enabled: true, Trusted: true}}, true},
		{"trusted without mddb", Config{Mddb: MddbConfig{Trusted: true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.SanitizesHTML(); got != tt.want {
				t.Errorf("SanitizesHTML() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMddbTrustedParsed checks mddb.trusted is read from YAML.
func TestMddbTrustedParsed(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssg.yaml")
	if err := os.WriteFile(path, []byte("mddb:\n  enabled: true\n  trusted: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mddb.Trusted || cfg.SanitizesHTML() {
		t.Errorf("mddb.trusted not honoured: %+v", cfg.Mddb)
	}
}
