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

// New creates a new template engine by name.
//
// A note on the Go case, because an audit keeps finding it (GO-044): the
// generator's default path does NOT go through this package. It renders with
// html/template directly, and only reaches for an engine when the site asked
// for a different one — so `case EngineGo` is not on any production path.
//
// GoEngine stays anyway, and deliberately. It is the reference implementation
// that keeps the Engine interface honest: three alternative engines are
// written against a contract, and a contract with no implementation from the
// language it models is a contract nobody has checked. `AvailableEngines`
// lists "go", so a caller passing it must get something that works rather than
// an error. Routing the default path through it would re-render every existing
// site through a second code path for no behaviour anyone asked for, which is
// exactly the kind of change a static site generator should not make.
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
