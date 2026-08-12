package models

import "testing"

// TestServedURLRelativeIndexStrip: a bare "index.html" with no leading slash
// trims to an empty base under strip mode, which falls back to the site root
// instead of returning an empty URL.
func TestServedURLRelativeIndexStrip(t *testing.T) {
	if got := PrettyStrip.ServedURL("index.html"); got != "/" {
		t.Errorf(`PrettyStrip.ServedURL("index.html") = %q, want "/"`, got)
	}
}
