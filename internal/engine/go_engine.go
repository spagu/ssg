package engine

import (
	"html/template"
	"io"
	"os"
)

// GoEngine implements Engine using Go's html/template
type GoEngine struct{}

// GoTemplate wraps Go's template.Template
type GoTemplate struct {
	tmpl *template.Template
}

// NewGoEngine creates a new Go template engine
func NewGoEngine() *GoEngine {
	return &GoEngine{}
}

// Name returns the engine name
func (e *GoEngine) Name() string {
	return EngineGo
}

// Parse parses template content
func (e *GoEngine) Parse(name, content string, funcs template.FuncMap) (Template, error) {
	tmpl := template.New(name)
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	parsed, err := tmpl.Parse(content)
	if err != nil {
		return nil, err
	}
	return &GoTemplate{tmpl: parsed}, nil
}

// ParseFile parses a template file
func (e *GoEngine) ParseFile(path string, funcs template.FuncMap) (Template, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's template files
	if err != nil {
		return nil, err
	}
	return e.Parse(path, string(content), funcs)
}

// Execute renders the template
func (t *GoTemplate) Execute(w io.Writer, data interface{}) error {
	return t.tmpl.Execute(w, data)
}
