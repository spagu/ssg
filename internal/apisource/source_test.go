package apisource

import "testing"

func TestDiagnosticString(t *testing.T) {
	for _, tc := range []struct {
		d    Diagnostic
		want string
	}{
		{Diagnostic{File: "src/a.js", Line: 12, Message: "unexpected token"}, "src/a.js:12: unexpected token"},
		{Diagnostic{File: "a.js", Line: 0, Message: "m"}, "a.js:0: m"},
		{Diagnostic{File: "a.js", Line: -3, Message: "m"}, "a.js:-3: m"},
	} {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}
