package models

// A link whose meaning lives in the query string (#234): "/?cpt=123" is a
// dynamic address. A static build cannot serve two documents at "/", so such a
// link must not claim the site root — that is how a stray gallery entry
// overwrote a real front page.

import "testing"

func TestQueryOnlyLinkDoesNotClaimTheRoot(t *testing.T) {
	page := Page{Slug: "1289", Type: "page", Link: "/?modula-gallery=1289"}
	if got := page.GetOutputPath(); got == "" || got == "." {
		t.Fatalf("GetOutputPath() = %q — a query-only link claimed the site root", got)
	}
	if got := page.GetOutputPath(); got != "1289" {
		t.Errorf("GetOutputPath() = %q, want the slug-derived %q", got, "1289")
	}
	if got := page.GetURL(); got == "/" {
		t.Errorf("GetURL() = %q — listing links would all point at the front page", got)
	}
}

func TestPlainRootLinkStillClaimsTheRoot(t *testing.T) {
	// The front-page feature (#129): link "/" IS the legitimate claim.
	page := Page{Slug: "home", Type: "page", Link: "/"}
	if got := page.GetOutputPath(); got != "" {
		t.Errorf("GetOutputPath() = %q, want the root claim to survive", got)
	}
	if got := page.GetURL(); got != "/" {
		t.Errorf("GetURL() = %q, want /", got)
	}
}

func TestQueryOnAPathedLinkIsStillDropped(t *testing.T) {
	// Only the ROOT-and-query shape is redirected to the slug; a link with a
	// real path keeps that path, query discarded, as it always was.
	page := Page{Slug: "s", Type: "page", Link: "/foo/?page=2"}
	if got := page.GetOutputPath(); got != "foo" {
		t.Errorf("GetOutputPath() = %q, want %q", got, "foo")
	}
	if got := page.GetURL(); got != "/foo/" {
		t.Errorf("GetURL() = %q, want /foo/", got)
	}
}
