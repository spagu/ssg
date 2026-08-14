package models

// Nested category archives (#138). WordPress serves a child category at
// /category/<parent>/<child>/; rendering it flat turns every surviving link
// into a 404 while the content sits one path away.

import "testing"

func categorySet(cats ...Category) map[int]Category {
	out := make(map[int]Category, len(cats))
	for _, c := range cats {
		out[c.ID] = c
	}
	return out
}

func TestCategoryPath(t *testing.T) {
	all := categorySet(
		Category{ID: 1, Slug: "blog"},
		Category{ID: 3, Slug: "dzien-bociana", Parent: 1},
		Category{ID: 4, Slug: "deep", Parent: 3},
		Category{ID: 5, Slug: "lost", Parent: 99}, // parent outside the export
	)
	cases := map[int]string{
		1: "blog",
		3: "blog/dzien-bociana",
		4: "blog/dzien-bociana/deep",
		5: "lost", // an absent parent is treated as absent, never guessed
	}
	for id, want := range cases {
		if got := CategoryPath(all[id], all); got != want {
			t.Errorf("CategoryPath(%d) = %q, want %q", id, got, want)
		}
	}
}

// TestCategoryPathSurvivesCycles: a taxonomy that loops must still render.
func TestCategoryPathSurvivesCycles(t *testing.T) {
	all := categorySet(
		Category{ID: 1, Slug: "a", Parent: 2},
		Category{ID: 2, Slug: "b", Parent: 1},
	)
	if got := CategoryPath(all[1], all); got != "b/a" {
		t.Fatalf("a cycle must stop where it repeats, got %q", got)
	}
	self := categorySet(Category{ID: 7, Slug: "self", Parent: 7})
	if got := CategoryPath(self[7], self); got != "self" {
		t.Fatalf("self-parented category = %q", got)
	}
}

// TestCategoryPathSanitises: a slug is a path segment, so a traversal attempt
// must not escape the archive root.
func TestCategoryPathSanitises(t *testing.T) {
	all := categorySet(
		Category{ID: 1, Slug: "../../etc"},
		Category{ID: 2, Slug: "child", Parent: 1},
	)
	got := CategoryPath(all[2], all)
	if got == "" || containsDotDot(got) {
		t.Fatalf("path escapes the root: %q", got)
	}
}

func containsDotDot(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}

func TestCategoryIsNested(t *testing.T) {
	all := categorySet(
		Category{ID: 1, Slug: "blog"},
		Category{ID: 2, Slug: "child", Parent: 1},
	)
	if CategoryIsNested(all[1], all) {
		t.Error("a top-level category is not nested")
	}
	if !CategoryIsNested(all[2], all) {
		t.Error("a child category is nested — its flat path needs a redirect")
	}
}
