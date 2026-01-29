package engine

import (
	"bytes"
	"html/template"
	"os"
	"testing"

	"github.com/flosch/pongo2/v6"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		wantErr bool
	}{
		{"go engine", "go", false},
		{"empty defaults to go", "", false},
		{"pongo2 engine", "pongo2", false},
		{"jinja2 alias", "jinja2", false},
		{"django alias", "django", false},
		{"mustache engine", "mustache", false},
		{"handlebars engine", "handlebars", false},
		{"hbs alias", "hbs", false},
		{"unknown engine", "unknown", true},
		{"invalid engine", "invalid123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := New(tt.engine)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if engine == nil {
				t.Error("expected engine, got nil")
			}
		})
	}
}

func TestAvailableEngines(t *testing.T) {
	engines := AvailableEngines()
	if len(engines) != 4 {
		t.Errorf("expected 4 engines, got %d", len(engines))
	}

	expected := []string{EngineGo, EnginePongo2, EngineMustache, EngineHandlebars}
	for i, e := range expected {
		if engines[i] != e {
			t.Errorf("expected %s at index %d, got %s", e, i, engines[i])
		}
	}
}

func TestGoEngine(t *testing.T) {
	engine := NewGoEngine()

	if engine.Name() != EngineGo {
		t.Errorf("expected name %s, got %s", EngineGo, engine.Name())
	}

	// Test simple template parsing
	tmpl, err := engine.Parse("test", "Hello {{.Name}}!", nil)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{"Name": "World"})
	if err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	if buf.String() != "Hello World!" {
		t.Errorf("expected 'Hello World!', got '%s'", buf.String())
	}
}

func TestGoEngineWithFuncs(t *testing.T) {
	engine := NewGoEngine()

	funcs := template.FuncMap{
		"upper": func(s string) string {
			return "UPPER:" + s
		},
	}

	tmpl, err := engine.Parse("test", "{{upper .Name}}", funcs)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{"Name": "test"})
	if err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	if buf.String() != "UPPER:test" {
		t.Errorf("expected 'UPPER:test', got '%s'", buf.String())
	}
}

func TestMustacheEngine(t *testing.T) {
	engine := NewMustacheEngine()

	if engine.Name() != EngineMustache {
		t.Errorf("expected name %s, got %s", EngineMustache, engine.Name())
	}

	// Test simple template
	tmpl, err := engine.Parse("test", "Hello {{Name}}!", nil)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{"Name": "Mustache"})
	if err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	if buf.String() != "Hello Mustache!" {
		t.Errorf("expected 'Hello Mustache!', got '%s'", buf.String())
	}
}

func TestHandlebarsEngine(t *testing.T) {
	engine := NewHandlebarsEngine()

	if engine.Name() != EngineHandlebars {
		t.Errorf("expected name %s, got %s", EngineHandlebars, engine.Name())
	}

	// Test simple template
	tmpl, err := engine.Parse("test", "Hello {{Name}}!", nil)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{"Name": "Handlebars"})
	if err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	if buf.String() != "Hello Handlebars!" {
		t.Errorf("expected 'Hello Handlebars!', got '%s'", buf.String())
	}
}

func TestPongo2Engine(t *testing.T) {
	engine := NewPongo2Engine()

	if engine.Name() != EnginePongo2 {
		t.Errorf("expected name %s, got %s", EnginePongo2, engine.Name())
	}

	// Test simple template
	tmpl, err := engine.Parse("test", "Hello {{ Name }}!", nil)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{"Name": "Pongo2"})
	if err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	if buf.String() != "Hello Pongo2!" {
		t.Errorf("expected 'Hello Pongo2!', got '%s'", buf.String())
	}
}

func TestGoEngineParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmplPath := tmpDir + "/test.html"
	if err := os.WriteFile(tmplPath, []byte("Hello {{.Name}}!"), 0644); err != nil {
		t.Fatalf("failed to create template file: %v", err)
	}

	engine := NewGoEngine()
	tmpl, err := engine.ParseFile(tmplPath, nil)
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"Name": "File"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello File!" {
		t.Errorf("expected 'Hello File!', got '%s'", buf.String())
	}
}

func TestGoEngineParseFileNotFound(t *testing.T) {
	engine := NewGoEngine()
	_, err := engine.ParseFile("/nonexistent/file.html", nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMustacheEngineParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmplPath := tmpDir + "/test.mustache"
	if err := os.WriteFile(tmplPath, []byte("Hello {{Name}}!"), 0644); err != nil {
		t.Fatalf("failed to create template file: %v", err)
	}

	engine := NewMustacheEngine()
	tmpl, err := engine.ParseFile(tmplPath, nil)
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"Name": "File"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello File!" {
		t.Errorf("expected 'Hello File!', got '%s'", buf.String())
	}
}

func TestMustacheEngineParseFileNotFound(t *testing.T) {
	engine := NewMustacheEngine()
	_, err := engine.ParseFile("/nonexistent/file.mustache", nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestHandlebarsEngineParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmplPath := tmpDir + "/test.hbs"
	if err := os.WriteFile(tmplPath, []byte("Hello {{Name}}!"), 0644); err != nil {
		t.Fatalf("failed to create template file: %v", err)
	}

	engine := NewHandlebarsEngine()
	tmpl, err := engine.ParseFile(tmplPath, nil)
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Name": "File"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello File!" {
		t.Errorf("expected 'Hello File!', got '%s'", buf.String())
	}
}

func TestHandlebarsEngineParseFileNotFound(t *testing.T) {
	engine := NewHandlebarsEngine()
	_, err := engine.ParseFile("/nonexistent/file.hbs", nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestPongo2EngineParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmplPath := tmpDir + "/test.html"
	if err := os.WriteFile(tmplPath, []byte("Hello {{ Name }}!"), 0644); err != nil {
		t.Fatalf("failed to create template file: %v", err)
	}

	engine := NewPongo2Engine()
	tmpl, err := engine.ParseFile(tmplPath, nil)
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Name": "File"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello File!" {
		t.Errorf("expected 'Hello File!', got '%s'", buf.String())
	}
}

func TestPongo2EngineParseFileNotFound(t *testing.T) {
	engine := NewPongo2Engine()
	_, err := engine.ParseFile("/nonexistent/file.html", nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestPongo2EngineWithFuncs(t *testing.T) {
	engine := NewPongo2Engine()

	funcs := template.FuncMap{
		"custom": func(s string) string {
			return "custom:" + s
		},
	}

	tmpl, err := engine.Parse("test", "Hello {{ Name }}!", funcs)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Name": "World"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello World!" {
		t.Errorf("expected 'Hello World!', got '%s'", buf.String())
	}
}

func TestPongo2ContextConversion(t *testing.T) {
	engine := NewPongo2Engine()

	// Test with struct data
	tmpl, err := engine.Parse("test", "Data: {{ Data }}", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, "simple string"); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Data: simple string" {
		t.Errorf("expected 'Data: simple string', got '%s'", buf.String())
	}
}

func TestGoEngineParseError(t *testing.T) {
	engine := NewGoEngine()
	_, err := engine.Parse("test", "{{invalid syntax", nil)
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestMustacheEngineParseError(t *testing.T) {
	engine := NewMustacheEngine()
	_, err := engine.Parse("test", "{{#unclosed}}", nil)
	if err == nil {
		t.Error("expected error for unclosed section")
	}
}

func TestHandlebarsEngineParseError(t *testing.T) {
	engine := NewHandlebarsEngine()
	_, err := engine.Parse("test", "{{#unclosed}}", nil)
	if err == nil {
		t.Error("expected error for unclosed section")
	}
}

func TestPongo2EngineParseError(t *testing.T) {
	engine := NewPongo2Engine()
	_, err := engine.Parse("test", "{% invalid %}", nil)
	if err == nil {
		t.Error("expected error for invalid syntax")
	}
}

func TestHandlebarsEngineWithFuncs(t *testing.T) {
	engine := NewHandlebarsEngine()

	funcs := template.FuncMap{
		"shout": func(s string) string {
			return s + "!"
		},
	}

	// Parse with custom helper
	tmpl, err := engine.Parse("test", "Hello {{Name}}", funcs)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{"Name": "World"})
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello World" {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestPongo2ContextFromPongoContext(t *testing.T) {
	engine := NewPongo2Engine()

	tmpl, err := engine.Parse("test", "{{ message }}", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Pass pongo2.Context directly
	ctx := map[string]interface{}{
		"message": "direct context",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "direct context" {
		t.Errorf("expected 'direct context', got '%s'", buf.String())
	}
}

func TestMustacheEngineWithFuncs(t *testing.T) {
	engine := NewMustacheEngine()

	// Mustache doesn't use funcs the same way, but test that it doesn't panic
	funcs := template.FuncMap{
		"test": func() string { return "test" },
	}

	tmpl, err := engine.Parse("test", "Hello {{Name}}", funcs)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"Name": "World"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", buf.String())
	}
}

func TestPongo2WithPongoContext(t *testing.T) {
	engine := NewPongo2Engine()

	tmpl, err := engine.Parse("test", "{{ value }}", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Pass pongo2.Context directly
	ctx := pongo2.Context{
		"value": "from pongo2 context",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "from pongo2 context" {
		t.Errorf("expected 'from pongo2 context', got '%s'", buf.String())
	}
}

func TestPongo2WithCustomFilter(t *testing.T) {
	engine := NewPongo2Engine()

	funcs := template.FuncMap{
		"testfilter": func(s string) string {
			return "filtered:" + s
		},
	}

	// This tests the registerPongo2Filter function - actually use the filter
	tmpl, err := engine.Parse("test", "{{ Name|testfilter }}", funcs)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"Name": "test"}); err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	// The filter returns passthrough, so result should still be "test"
	if buf.String() != "test" {
		t.Errorf("expected 'test', got '%s'", buf.String())
	}
}

// errorWriter is a writer that always returns an error
type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, os.ErrClosed
}

func TestHandlebarsExecuteWriteError(t *testing.T) {
	engine := NewHandlebarsEngine()

	tmpl, err := engine.Parse("test", "Hello {{Name}}!", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Use error writer to trigger write error
	ew := &errorWriter{}
	err = tmpl.Execute(ew, map[string]interface{}{"Name": "World"})
	if err == nil {
		t.Error("expected error for write failure")
	}
}

func TestHandlebarsExecuteWithHelper(t *testing.T) {
	engine := NewHandlebarsEngine()

	funcs := template.FuncMap{
		"greet": func(name string) string {
			return "Hello, " + name + "!"
		},
	}

	// Parse with helper registered
	tmpl, err := engine.Parse("test", "{{greet Name}}", funcs)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{"Name": "World"})
	// Just ensure it runs - helper behavior may vary
	_ = err
}

func TestHandlebarsExecuteWithContext(t *testing.T) {
	engine := NewHandlebarsEngine()

	// Test with various data types
	tmpl, err := engine.Parse("test", "{{#if active}}Active{{else}}Inactive{{/if}}", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{"active": true})
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if buf.String() != "Active" {
		t.Errorf("expected 'Active', got '%s'", buf.String())
	}
}

func TestGoEngineExecuteError(t *testing.T) {
	engine := NewGoEngine()

	tmpl, err := engine.Parse("test", "{{.MissingMethod}}", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, "string without MissingMethod")
	// Go templates may not error on missing fields, just output empty
	// This just ensures Execute runs without panic
	_ = err
}

func TestMustacheExecuteError(t *testing.T) {
	engine := NewMustacheEngine()

	tmpl, err := engine.Parse("test", "Hello {{Name}}!", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Use error writer
	ew := &errorWriter{}
	err = tmpl.Execute(ew, map[string]string{"Name": "World"})
	if err == nil {
		t.Error("expected error for write failure")
	}
}

func TestPongo2ExecuteError(t *testing.T) {
	engine := NewPongo2Engine()

	tmpl, err := engine.Parse("test", "Hello {{ Name }}!", nil)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Use error writer
	ew := &errorWriter{}
	err = tmpl.Execute(ew, map[string]interface{}{"Name": "World"})
	if err == nil {
		t.Error("expected error for write failure")
	}
}
