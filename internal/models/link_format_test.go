package models

import "testing"

// TestLinkWithExtensionIsFinal covers #81: an explicit frontmatter link: that
// already names a file must not be decorated further. It is documented as the
// highest-precedence URL source, so appending a slash (or, in the generator, a
// second ".html") contradicts what it declares.
func TestLinkWithExtensionIsFinal(t *testing.T) {
	cases := []struct {
		link, pageFormat, wantURL string
	}{
		{"/validator.html", "flat", "/validator.html"},
		{"/validator.html", "directory", "/validator.html"},
		{"/feed.xml", "directory", "/feed.xml"},
		{"/data.json", "flat", "/data.json"},
		// No extension ⇒ unchanged behaviour: a directory-style URL.
		{"/docs/intro", "flat", "/docs/intro/"},
		{"/docs/intro/", "directory", "/docs/intro/"},
		// A dot that is not an extension must not be mistaken for one.
		{"/spec/v1.0", "directory", "/spec/v1.0/"},
	}
	for _, c := range cases {
		p := Page{Type: "page", Slug: "x", Link: c.link, PageFormat: c.pageFormat}
		if got := p.GetURL(); got != c.wantURL {
			t.Errorf("link=%q format=%q: GetURL() = %q, want %q", c.link, c.pageFormat, got, c.wantURL)
		}
	}
}

// TestHasPageExtension pins which suffixes count as "already a file".
func TestHasPageExtension(t *testing.T) {
	for _, p := range []string{"/a.html", "/a.htm", "/feed.xml", "/d.json", "/r.txt", "/a.HTML"} {
		if !HasPageExtension(p) {
			t.Errorf("HasPageExtension(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/a", "/a/", "/spec/v1.0", "/a.png", "/dir.name/sub", ""} {
		if HasPageExtension(p) {
			t.Errorf("HasPageExtension(%q) = true, want false", p)
		}
	}
}
