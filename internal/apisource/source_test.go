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

func TestFilter(t *testing.T) {
	tests := []struct {
		glob, file string
		want       bool
	}{
		{"src/*.js", "src/a.js", true},
		{"src/*.js", "src/x/a.js", false},
		{"src/**", "src/x/a.js", true},
		{"**/a.js", "a.js", true},
		{"./src/**/*.js", "src/x/y/a.js", true},
		{"src/**/b.js", "src/x/a.js", false},
		{"src", "src/a.js", false},
		{"src/[", "src/[", false},
	}
	for _, tt := range tests {
		if got := globMatch(tt.glob, tt.file); got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v", tt.glob, tt.file, got)
		}
	}
	f := Config{Include: []string{"src/**"}, Exclude: []string{"**/*.test.js"}}.Filter()
	if !f.Allows("src/a.js") || f.Allows("src/a.test.js") || f.Allows("lib/a.js") || !(Filter{}).Allows("x") {
		t.Error("Allows")
	}
}
