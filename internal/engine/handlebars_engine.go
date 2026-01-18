package engine

import (
	"html/template"
	"io"
	"os"

	"github.com/aymerick/raymond"
)

// HandlebarsEngine implements Engine using Handlebars (raymond)
type HandlebarsEngine struct{}

// HandlebarsTemplate wraps raymond.Template
type HandlebarsTemplate struct {
	tmpl *raymond.Template
}

// NewHandlebarsEngine creates a new Handlebars template engine
func NewHandlebarsEngine() *HandlebarsEngine {
	return &HandlebarsEngine{}
}

// Name returns the engine name
func (e *HandlebarsEngine) Name() string {
	return EngineHandlebars
}

// Parse parses template content
func (e *HandlebarsEngine) Parse(name, content string, funcs template.FuncMap) (Template, error) {
	// Register helpers from funcs
	for fname, fn := range funcs {
		registerHandlebarsHelper(fname, fn)
	}

	tmpl, err := raymond.Parse(content)
	if err != nil {
		return nil, err
	}
	return &HandlebarsTemplate{tmpl: tmpl}, nil
}

// ParseFile parses a template file
func (e *HandlebarsEngine) ParseFile(path string, funcs template.FuncMap) (Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return e.Parse(path, string(content), funcs)
}

// Execute renders the template
func (t *HandlebarsTemplate) Execute(w io.Writer, data interface{}) error {
	result, err := t.tmpl.Exec(data)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(result))
	return err
}

// registerHandlebarsHelper registers a Go function as a Handlebars helper
func registerHandlebarsHelper(name string, fn interface{}) {
	// Register simple helpers - full implementation would use reflection
	raymond.RegisterHelper(name, fn)
}
