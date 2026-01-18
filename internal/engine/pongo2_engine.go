package engine

import (
	"html/template"
	"io"
	"os"

	"github.com/flosch/pongo2/v6"
)

// Pongo2Engine implements Engine using Pongo2 (Jinja2/Django-like)
type Pongo2Engine struct{}

// Pongo2Template wraps pongo2.Template
type Pongo2Template struct {
	tmpl *pongo2.Template
}

// NewPongo2Engine creates a new Pongo2 template engine
func NewPongo2Engine() *Pongo2Engine {
	return &Pongo2Engine{}
}

// Name returns the engine name
func (e *Pongo2Engine) Name() string {
	return EnginePongo2
}

// Parse parses template content
func (e *Pongo2Engine) Parse(name, content string, funcs template.FuncMap) (Template, error) {
	// Register custom filters from Go funcs
	for fname, fn := range funcs {
		registerPongo2Filter(fname, fn)
	}

	tmpl, err := pongo2.FromString(content)
	if err != nil {
		return nil, err
	}
	return &Pongo2Template{tmpl: tmpl}, nil
}

// ParseFile parses a template file
func (e *Pongo2Engine) ParseFile(path string, funcs template.FuncMap) (Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return e.Parse(path, string(content), funcs)
}

// Execute renders the template
func (t *Pongo2Template) Execute(w io.Writer, data interface{}) error {
	// Convert data to pongo2.Context
	ctx := dataToPongo2Context(data)
	return t.tmpl.ExecuteWriter(ctx, w)
}

// dataToPongo2Context converts Go data to pongo2.Context
func dataToPongo2Context(data interface{}) pongo2.Context {
	ctx := pongo2.Context{}

	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			ctx[k] = val
		}
	case pongo2.Context:
		return v
	default:
		// Wrap in "Data" key
		ctx["Data"] = data
	}

	return ctx
}

// registerPongo2Filter registers a Go function as a pongo2 filter
func registerPongo2Filter(name string, fn interface{}) {
	// Try to register as filter - simplified version
	// Full implementation would need reflection to adapt function signatures
	_ = pongo2.RegisterFilter(name, func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		// Basic passthrough for now
		return in, nil
	})
}
