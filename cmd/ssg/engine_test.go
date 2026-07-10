package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestValidateTemplateEngine covers GO-007: all four back-ends now render, so the
// pongo2/mustache/handlebars engines (and their aliases) are accepted; only an
// entirely unknown engine is rejected.
func TestValidateTemplateEngine(t *testing.T) {
	// engine value -> whether an error is expected.
	wantErrByEngine := map[string]bool{
		"":           false, // empty defaults to go
		"go":         false,
		"GO":         false, // case-insensitive
		"pongo2":     false, // now wired (GO-007)
		"jinja2":     false, // pongo2 alias
		"mustache":   false,
		"handlebars": false,
		"hbs":        false, // handlebars alias
		"twig":       true,  // entirely unknown
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
