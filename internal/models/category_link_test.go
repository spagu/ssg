package models

// A category archive belongs where the source site serves it (#143).
// WordPress lets a site drop the /category/ base, and many do; the export
// records each term's real address in `link`, so building a fixed
// /category/<term>/ turns every archive link the content carries into a 404.

import "testing"

func TestCategoryArchivePathFromLink(t *testing.T) {
	all := map[int]Category{
		7: {ID: 7, Slug: "realizacje", Link: "https://example.com/realizacje/"},
		9: {ID: 9, Slug: "adaptacje", Parent: 7,
			Link: "https://example.com/realizacje/adaptacje/"},
	}
	if got := CategoryArchivePath(all[9], all, "category"); got != "realizacje/adaptacje" {
		t.Fatalf("the site's own address must win: %q", got)
	}
	if got := CategoryArchivePath(all[7], all, "category"); got != "realizacje" {
		t.Fatalf("top-level term: %q", got)
	}
}

// TestCategoryArchivePathFallsBack: a hand-authored site has no link and keeps
// the layout it has always had — nesting included (#138).
func TestCategoryArchivePathFallsBack(t *testing.T) {
	all := map[int]Category{
		1: {ID: 1, Slug: "blog"},
		3: {ID: 3, Slug: "news", Parent: 1},
	}
	if got := CategoryArchivePath(all[3], all, "category"); got != "category/blog/news" {
		t.Fatalf("built-in layout: %q", got)
	}
	if got := CategoryArchivePath(all[1], all, "category"); got != "category/blog" {
		t.Fatalf("built-in layout, top level: %q", got)
	}
}

// TestCategoryArchivePathRejectsUnusableLinks: a link that says nothing about
// the path sends the caller back to the built-in layout rather than producing
// an empty or escaping path.
func TestCategoryArchivePathRejectsUnusableLinks(t *testing.T) {
	cases := []string{"", "   ", "https://example.com", "https://example.com/", "://broken"}
	for _, link := range cases {
		cat := Category{ID: 1, Slug: "news", Link: link}
		got := CategoryArchivePath(cat, map[int]Category{1: cat}, "category")
		if got != "category/news" {
			t.Errorf("link %q must fall back, got %q", link, got)
		}
	}
}

// TestPathFromLinkSanitises: the path becomes output directories, so a
// traversal attempt must not escape the site root.
func TestPathFromLinkSanitises(t *testing.T) {
	got := pathFromLink("https://example.com/../../etc/passwd/")
	if got == "" {
		return // refusing outright is also a correct answer
	}
	for i := 0; i+1 < len(got); i++ {
		if got[i] == '.' && got[i+1] == '.' {
			t.Fatalf("path escapes the root: %q", got)
		}
	}
}
