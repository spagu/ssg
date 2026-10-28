package models

import "testing"

// TestSlugify: one rule for every URL built from a label, so two producers
// never disagree about an address.
func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"  Hello, World!  ": "hello-world",
		"Zażółć gęślą":      "za-g-l",
		"--a--b--":          "a-b",
		"2026 Release":      "2026-release",
		"":                  "",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMarketingEmpty: any discovered field makes the block worth emitting.
func TestMarketingEmpty(t *testing.T) {
	if !(Marketing{}).Empty() {
		t.Error("zero value is empty")
	}
	if (Marketing{Logo: "/logo.png"}).Empty() || (Marketing{Colors: map[string]string{"accent": "#000"}}).Empty() {
		t.Error("a logo or a colour is not empty")
	}
}
