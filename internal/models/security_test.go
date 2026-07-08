// Package models - security regression tests (SEC-001 path traversal)
package models

import (
	"strings"
	"testing"
)

// TestSanitizeRelPath verifies that untrusted slug/link values cannot escape
// their root via path traversal, absolute paths, or Windows separators (SEC-001).
func TestSanitizeRelPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain slug", "contact", "contact"},
		{"nested slug", "blog/post-1", "blog/post-1"},
		{"empty", "", ""},
		{"dot", ".", ""},
		{"leading slash", "/services/web", "services/web"},
		{"trailing slash", "services/web/", "services/web"},
		{"simple traversal", "../../etc/passwd", "etc/passwd"},
		{"embedded traversal", "blog/../../etc/passwd", "etc/passwd"},
		{"absolute escape", "/../../root/.bashrc", "root/.bashrc"},
		{"windows separators", "..\\..\\windows\\system32", "windows/system32"},
		{"mixed traversal that escapes root stays in root", "a/b/../../../../x", "x"},
		{"only dotdot collapses to empty", "..", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeRelPath(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeRelPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Invariant: the result must never contain a ".." segment nor be absolute.
			if strings.HasPrefix(got, "/") {
				t.Errorf("SanitizeRelPath(%q) = %q is absolute", tt.input, got)
			}
			for _, seg := range strings.Split(got, "/") {
				if seg == ".." {
					t.Errorf("SanitizeRelPath(%q) = %q contains '..' segment", tt.input, got)
				}
			}
		})
	}
}

// TestGetOutputPath_PathTraversal ensures malicious slug/link from an untrusted
// source (e.g. mddb) is neutralized when building the output sub-path (SEC-001).
func TestGetOutputPath_PathTraversal(t *testing.T) {
	tests := []struct {
		name string
		page Page
	}{
		{"malicious slug page", Page{Type: "page", Slug: "../../../etc/cron.d/evil"}},
		{"malicious slug post", Page{Type: "post", URLFormat: "slug", Slug: "../../evil"}},
		{"malicious link", Page{Type: "page", Slug: "x", Link: "https://d/../../root/.bashrc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.page.GetOutputPath()
			if strings.Contains(got, "..") {
				t.Errorf("GetOutputPath() = %q still contains traversal", got)
			}
			if strings.HasPrefix(got, "/") {
				t.Errorf("GetOutputPath() = %q is absolute", got)
			}
		})
	}
}
