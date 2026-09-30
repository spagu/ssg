package config

import "testing"

func TestResolveBasePath(t *testing.T) {
	for _, tc := range []struct{ explicit, domain, want string }{
		{"", "example.com", ""},
		{"", "", ""},
		{"", "user.github.io/docs-site", "/docs-site"},
		{"", "https://user.github.io/docs-site/", "/docs-site"},
		{"", "example.com/a/b/", "/a/b"},
		{"docs", "example.com", "/docs"},
		{"/docs/", "example.com/other", "/docs"},
		{"/", "user.github.io/docs-site", ""},
		{" // ", "user.github.io/docs-site", ""},
	} {
		if got := ResolveBasePath(tc.explicit, tc.domain); got != tc.want {
			t.Errorf("ResolveBasePath(%q, %q) = %q, want %q", tc.explicit, tc.domain, got, tc.want)
		}
	}
}
