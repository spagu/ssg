package engine

import (
	"bytes"
	"html/template"
	"testing"
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
