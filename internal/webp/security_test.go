// Package webp - tests for the SEC-011 cwebp argument-injection hardening.
package webp

import (
	"path/filepath"
	"testing"
)

// TestSafeArgPath verifies that filenames which could be mistaken for cwebp
// flags are prefixed with "./", while already-safe paths pass through unchanged.
func TestSafeArgPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"dash-leading relative", "-o.png", "./-o.png"},
		{"double-dash relative", "--help.png", "./--help.png"},
		{"plain relative", "image.png", "./image.png"},
		{"nested relative with dash", filepath.Join("dir", "-x.png"), "./" + filepath.Join("dir", "-x.png")},
		{"already dot-slash", "./image.png", "./image.png"},
		{"parent relative", "../image.png", "../image.png"},
		{"absolute path", "/tmp/image.png", "/tmp/image.png"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeArgPath(tt.in); got != tt.want {
				t.Errorf("safeArgPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
