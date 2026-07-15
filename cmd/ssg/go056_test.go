package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// captureStderr runs fn and returns everything it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

// GO-056: incomplete TLS/HTTP3 configurations must be loud, never silent.
func TestWarnTLSMisconfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"http3 without tls", config.Config{HTTP3: true}, "--http3 requires TLS"},
		{"tls-auto without domain", config.Config{TLSAuto: true}, "--tls-auto needs --tls-domain"},
		{"cert without key", config.Config{TLSCert: "c.pem"}, "--tls-cert given without --tls-key"},
		{"key without cert", config.Config{TLSKey: "k.pem"}, "--tls-key given without --tls-cert"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			out := captureStderr(t, func() { warnTLSMisconfig(&cfg, serverTLSMode(&cfg)) })
			if !strings.Contains(out, tc.want) {
				t.Errorf("stderr = %q, want substring %q", out, tc.want)
			}
		})
	}

	t.Run("complete tls stays quiet", func(t *testing.T) {
		cfg := config.Config{TLSCert: "c.pem", TLSKey: "k.pem", HTTP3: true}
		out := captureStderr(t, func() { warnTLSMisconfig(&cfg, serverTLSMode(&cfg)) })
		if out != "" {
			t.Errorf("expected no warnings, got %q", out)
		}
	})
}
