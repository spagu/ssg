package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestValidateTemplateEngine is a regression test for GO-002: only the Go
// engine is actually wired into rendering, so unsupported engines must be
// rejected with an error instead of being silently ignored.
func TestValidateTemplateEngine(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		wantErr bool
	}{
		{"empty defaults to go", "", false},
		{"go", "go", false},
		{"GO uppercase", "GO", false},
		{"pongo2 not implemented", "pongo2", true},
		{"jinja2 alias not implemented", "jinja2", true},
		{"mustache not implemented", "mustache", true},
		{"handlebars not implemented", "handlebars", true},
		{"hbs alias not implemented", "hbs", true},
		{"unknown engine", "twig", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Engine: tt.engine}
			err := validateTemplateEngine(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTemplateEngine(engine=%q) err=%v, wantErr=%v", tt.engine, err, tt.wantErr)
			}
		})
	}
}
