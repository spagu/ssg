package config

// `domain:` taken from the environment, and checked before it becomes every
// absolute URL of the site (#439).

import (
	"strings"
	"testing"
)

// TestExpandDomain: both reference spellings resolve, a literal is untouched,
// and a reference that resolves to nothing is an error naming the variable —
// never a site built for an empty host.
func TestExpandDomain(t *testing.T) {
	t.Setenv("SSG_TEST_SITE_DOMAIN", "staging.example.com")
	t.Setenv("SSG_TEST_EMPTY_DOMAIN", "")

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr []string // substrings of the error; nil when expansion succeeds
	}{
		{"no reference is unchanged", "example.com", "example.com", nil},
		{"scheme is left to NormalizeDomain", "https://example.com/", "https://example.com/", nil},
		{"braced", "${SSG_TEST_SITE_DOMAIN}", "staging.example.com", nil},
		{"bare", "$SSG_TEST_SITE_DOMAIN", "staging.example.com", nil},
		{"inside a value", "${SSG_TEST_SITE_DOMAIN}/blog", "staging.example.com/blog", nil},
		{"dollar that is no reference stays", "example.com/a$", "example.com/a$", nil},
		{"unset", "${SSG_TEST_UNSET_DOMAIN}", "", []string{"$SSG_TEST_UNSET_DOMAIN", "not set", "${SSG_TEST_UNSET_DOMAIN}"}},
		{"empty", "$SSG_TEST_EMPTY_DOMAIN", "", []string{"$SSG_TEST_EMPTY_DOMAIN", "empty"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandDomain(tc.in)
			if tc.wantErr == nil {
				if err != nil || got != tc.want {
					t.Fatalf("ExpandDomain(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ExpandDomain(%q) = %q, want an error", tc.in, got)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
}

// TestValidateDomain: a host, optionally with a port and a path, is accepted;
// anything that cannot be the host of https://<domain>/ is refused.
func TestValidateDomain(t *testing.T) {
	for _, ok := range []string{
		"example.com", "staging.example.com", "example.com:8080",
		"example.com/blog", "localhost", "192.0.2.10", "[2001:db8::1]:8443",
	} {
		if err := ValidateDomain(ok); err != nil {
			t.Errorf("ValidateDomain(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"", "exa mple.com", "example.com\n", "user@example.com", "example.com?x=1",
		"example.com#top", "example.com/$", `example.com"`, "<example.com>",
		"example.com:port", "/blog", ":8080",
	} {
		if err := ValidateDomain(bad); err == nil {
			t.Errorf("ValidateDomain(%q) = nil, want an error", bad)
		}
	}
}
