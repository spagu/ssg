// Package engine provides multiple template engine implementations
package engine

import (
	"fmt"
	"html/template"
	"io"
	"strings"
)

// Engine represents a template engine interface
type Engine interface {
	// Name returns the engine name
	Name() string
	// Parse parses template content and returns a compiled template
	Parse(name, content string, funcs template.FuncMap) (Template, error)
	// ParseFile parses a template file
	ParseFile(path string, funcs template.FuncMap) (Template, error)
}

// Template represents a compiled template
type Template interface {
	// Execute renders the template with given data
	Execute(w io.Writer, data interface{}) error
}

// Available engine types
const (
	EngineGo         = "go"
	EnginePongo2     = "pongo2"
	EngineMustache   = "mustache"
	EngineHandlebars = "handlebars"
)

// AvailableEngines returns list of available engine names
func AvailableEngines() []string {
	return []string{EngineGo, EnginePongo2, EngineMustache, EngineHandlebars}
}

// New creates a new template engine by name
func New(name string) (Engine, error) {
	switch strings.ToLower(name) {
	case EngineGo, "":
		return NewGoEngine(), nil
	case EnginePongo2, "jinja2", "django":
		return NewPongo2Engine(), nil
	case EngineMustache:
		return NewMustacheEngine(), nil
	case EngineHandlebars, "hbs":
		return NewHandlebarsEngine(), nil
	default:
		return nil, fmt.Errorf("unknown template engine: %s (available: %v)", name, AvailableEngines())
	}
}
