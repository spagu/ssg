package engine

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"sync"

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
	// Register FuncMap helpers as real pongo2 filters (GO-054); helpers a
	// filter cannot express are reported once instead of passing through.
	var unsupported []string
	for fname, fn := range funcs {
		if !registerPongo2Filter(fname, fn) {
			unsupported = append(unsupported, fname)
		}
	}
	warnUnsupported(EnginePongo2, unsupported)

	tmpl, err := pongo2.FromString(content)
	if err != nil {
		return nil, err
	}
	return &Pongo2Template{tmpl: tmpl}, nil
}

// ParseFile parses a template file
func (e *Pongo2Engine) ParseFile(path string, funcs template.FuncMap) (Template, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's template files
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

// pongo2Registered remembers which filter names this process registered and
// whether the underlying helper was adaptable, so repeated Parse calls stay
// idempotent and the per-file unsupported list stays accurate (GO-054).
var pongo2Registered sync.Map // name → supported bool

// registerPongo2Filter registers a Go FuncMap helper as a real pongo2 filter
// (GO-054): `{{ value|helper }}` calls helper(value) and `{{ value|helper:arg }}`
// calls helper(value, arg). Helpers a two-slot filter cannot express (arity
// > 2, invalid signature) get an erroring filter — loud at render time, never
// a silent passthrough. Reports whether the helper is supported.
func registerPongo2Filter(name string, fn interface{}) bool {
	if supported, seen := pongo2Registered.Load(name); seen {
		return supported.(bool)
	}
	adapter, err := adaptHelper(fn)
	supported := err == nil && (adapter.accepts(1) || adapter.accepts(2))
	pongo2Registered.Store(name, supported)
	if pongo2.FilterExists(name) { // builtin (e.g. default): leave pongo2's own
		return true
	}
	if !supported {
		_ = pongo2.RegisterFilter(name, func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			return nil, &pongo2.Error{Sender: "filter:" + name,
				OrigError: fmt.Errorf("helper %q is not available as a pongo2 filter (use the Go engine)", name)}
		})
		return false
	}
	_ = pongo2.RegisterFilter(name, func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		args := []interface{}{in.Interface()}
		if param != nil && !param.IsNil() {
			args = append(args, param.Interface())
		}
		if len(args) == 1 && !adapter.accepts(1) && adapter.accepts(2) {
			args = append(args, nil) // helper requires its second argument
		}
		res, err := adapter.call(args...)
		if err != nil {
			return nil, &pongo2.Error{Sender: "filter:" + name, OrigError: err}
		}
		if s, safe := helperResultString(res); safe {
			return pongo2.AsSafeValue(s), nil
		}
		return pongo2.AsValue(res), nil
	})
	return true
}
