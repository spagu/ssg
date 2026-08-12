package fetch

import "testing"

func TestDefaultOptions(t *testing.T) {
	o := DefaultOptions()
	if o.Timeout != defaultTimeout || o.Retries != DefaultRetries || o.RetryDelay != defaultRetryDelay {
		t.Fatalf("DefaultOptions = %+v", o)
	}
}

// TestSafeURL: credentials and query strings never reach logs.
func TestSafeURL(t *testing.T) {
	cases := map[string]string{
		"https://user:pass@example.com/a?token=secret": "https://example.com/a",
		"https://example.com/plain":                    "https://example.com/plain",
		"not a url ?q=1":                               "not a url ",
		"plain-string":                                 "plain-string",
	}
	for in, want := range cases {
		if got := safeURL(in); got != want {
			t.Errorf("safeURL(%q) = %q, want %q", in, got, want)
		}
	}
}
