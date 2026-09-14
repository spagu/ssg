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

// TestThe404PageAddressesTheFileWritten covers #284: a page whose path is
// "404" is written to /404.html under every page_format, so its address must
// name that file rather than a /404/ directory nobody wrote. A language
// prefix, a post's date path or an explicit link: are not the special case.
func TestThe404PageAddressesTheFileWritten(t *testing.T) {
	cases := []struct {
		name string
		page Page
		want string
	}{
		{"directory", Page{Type: "page", Slug: "404", PageFormat: "directory"}, "/404.html"},
		{"both", Page{Type: "page", Slug: "404", PageFormat: "both"}, "/404.html"},
		{"flat", Page{Type: "page", Slug: "404", PageFormat: "flat"}, "/404.html"},
		{"link wins", Page{Type: "page", Slug: "404", Link: "/not-found/", PageFormat: "directory"}, "/not-found/"},
		{"language prefix", Page{Type: "page", Slug: "404", LangPrefix: "pl", PageFormat: "directory"}, "/pl/404/"},
		{"a slug that only contains 404", Page{Type: "page", Slug: "error-404", PageFormat: "directory"}, "/error-404/"},
	}
	for _, c := range cases {
		if got := c.page.GetURL(); got != c.want {
			t.Errorf("%s: GetURL() = %q, want %q", c.name, got, c.want)
		}
	}
}
