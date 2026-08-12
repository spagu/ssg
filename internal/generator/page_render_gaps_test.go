package generator

// Per-page render failure paths: SEC-001 path confinement, blocked output
// directories and template execution errors for both pages and posts, plus the
// _redirects write failure.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestGeneratePageFailures: a page whose output cannot be confined, created or
// rendered fails with the underlying error.
func TestGeneratePageFailures(t *testing.T) {
	page := models.Page{Slug: "mypage", Title: "P", Type: "page", Status: "publish"}

	// No usable output root: SEC-001 confinement rejects the path.
	g := newTestGen(t, `{{define "page.html"}}ok{{end}}`)
	g.config.OutputDir = ""
	if err := g.generatePage(page); err == nil {
		t.Error("an unconfinable output path must error")
	}

	// A file squatting on the page's directory blocks MkdirAll.
	g = newTestGen(t, `{{define "page.html"}}ok{{end}}`)
	mustWrite(t, filepath.Join(g.config.OutputDir, "mypage"), "in the way")
	if err := g.generatePage(page); err == nil {
		t.Error("a blocked page directory must error")
	}

	// A template failing for a real reason (not a missing template) is NOT
	// retried against page.html — the error surfaces as-is.
	g = newTestGen(t, `{{define "page.html"}}{{index .Title 99}}{{end}}`)
	err := g.generatePage(page)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("execution error must surface unmasked, got %v", err)
	}
}

// TestGeneratePostFailures: the same three failure shapes for posts.
func TestGeneratePostFailures(t *testing.T) {
	post := models.Page{Slug: "mypost", Title: "P", Type: "post", Status: "publish",
		URLFormat: "slug"} // slug path, so the blocked directory is predictable

	g := newTestGen(t, `{{define "post.html"}}ok{{end}}`)
	g.config.OutputDir = ""
	if err := g.generatePost(post); err == nil {
		t.Error("an unconfinable output path must error")
	}

	g = newTestGen(t, `{{define "post.html"}}ok{{end}}`)
	mustWrite(t, filepath.Join(g.config.OutputDir, "mypost"), "in the way")
	if err := g.generatePost(post); err == nil {
		t.Error("a blocked post directory must error")
	}

	g = newTestGen(t, `{{define "post.html"}}{{index .Title 99}}{{end}}`)
	if err := g.generatePost(post); err == nil {
		t.Error("a failing post template must error")
	}
}

// TestGenerateCloudflareRedirectsBlocked: _redirects is written even when
// empty; a directory squatting on it fails the Cloudflare file generation.
func TestGenerateCloudflareRedirectsBlocked(t *testing.T) {
	g := newTestGen(t, "")
	mustMkdir(t, filepath.Join(g.config.OutputDir, "_redirects"))
	if err := g.generateCloudflareFiles(); err == nil || !strings.Contains(err.Error(), "_redirects") {
		t.Fatalf("blocked _redirects must error, got %v", err)
	}
}
