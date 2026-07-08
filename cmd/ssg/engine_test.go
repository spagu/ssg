package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestValidateTemplateEngine is a regression test for GO-002: only the Go
// engine is actually wired into rendering, so unsupported engines must be
// rejected with an error instead of being silently ignored.
func TestValidateTemplateEngine(t *testing.T) {
	// engine value -> whether an error is expected.
	wantErrByEngine := map[string]bool{
		"":           false, // empty defaults to go
		"go":         false,
		"GO":         false, // case-insensitive
		"pongo2":     true,  // recognized but not implemented
		"jinja2":     true,  // pongo2 alias
		"mustache":   true,
		"handlebars": true,
		"hbs":        true, // handlebars alias
		"twig":       true, // entirely unknown
	}
	for eng, wantErr := range wantErrByEngine {
		eng, wantErr := eng, wantErr
		t.Run("engine="+eng, func(t *testing.T) {
			gotErr := validateTemplateEngine(&config.Config{Engine: eng}) != nil
			if gotErr != wantErr {
				t.Errorf("validateTemplateEngine(%q): gotErr=%v, wantErr=%v", eng, gotErr, wantErr)
			}
		})
	}
}
