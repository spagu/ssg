package python

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

func TestExtractBrokenFiles(t *testing.T) {
	pkg, diags, err := Extract(apisource.Config{Root: filepath.Join("testdata", "broken")})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	tests := []struct {
		file string
		line int
		text string
	}{
		{"brk/strings.py", 4, "unterminated string"},
		{"brk/brackets.py", 4, "unterminated bracket"},
		{"brk/indent.py", 4, "inconsistent dedent"},
	}
	for _, tt := range tests {
		found := false
		for _, d := range diags {
			if d.File == tt.file && d.Line == tt.line && d.Severity == apisource.Error && strings.Contains(d.Message, tt.text) {
				found = true
			}
		}
		if !found {
			t.Errorf("no %q diagnostic at %s:%d; got %v", tt.text, tt.file, tt.line, diags)
		}
	}
	if pkg.Name != "brk-dist" || pkg.Version != "" {
		t.Errorf("name/version = %q/%q, want brk-dist and no version (attr:)", pkg.Name, pkg.Version)
	}
	init := moduleNamed(t, pkg, "brk")
	if s := symbolNamed(t, init, "fine"); s.Kind != apimodel.KindFunction {
		t.Errorf("fine kind = %s", s.Kind)
	}
	if s := symbolNamed(t, moduleNamed(t, pkg, "brk/strings"), "ok"); s.Source.Line != 1 {
		t.Errorf("ok line = %d", s.Source.Line)
	}
}

func TestExtractFixtureDiagnostics(t *testing.T) {
	_, diags := extractFixture(t)
	if !hasDiag(diags, apisource.Warning, "src/textkit/__init__.py", "__all__ lists missing") {
		t.Errorf("missing __all__ warning; got %v", diags)
	}
	if len(diags) != 1 {
		t.Errorf("diagnostics = %v, want only the __all__ warning", diags)
	}
}
