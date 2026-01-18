package engine

import (
	"html/template"
	"io"
	"os"

	"github.com/cbroglie/mustache"
)

// MustacheEngine implements Engine using Mustache
type MustacheEngine struct{}

// MustacheTemplate wraps mustache.Template
type MustacheTemplate struct {
	tmpl *mustache.Template
}

// NewMustacheEngine creates a new Mustache template engine
func NewMustacheEngine() *MustacheEngine {
	return &MustacheEngine{}
}

// Name returns the engine name
func (e *MustacheEngine) Name() string {
	return EngineMustache
}

// Parse parses template content
func (e *MustacheEngine) Parse(name, content string, funcs template.FuncMap) (Template, error) {
	// Note: Mustache doesn't support custom functions in the same way
	// Functions would need to be passed as data
	tmpl, err := mustache.ParseString(content)
	if err != nil {
		return nil, err
	}
	return &MustacheTemplate{tmpl: tmpl}, nil
}

// ParseFile parses a template file
func (e *MustacheEngine) ParseFile(path string, funcs template.FuncMap) (Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return e.Parse(path, string(content), funcs)
}

// Execute renders the template
func (t *MustacheTemplate) Execute(w io.Writer, data interface{}) error {
	return t.tmpl.FRender(w, data)
}
