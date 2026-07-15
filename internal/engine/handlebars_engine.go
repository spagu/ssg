package engine

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"sync"

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
	// Register FuncMap helpers as raymond helpers via the reflection adapter
	// (GO-054); anything not adaptable is reported once, never swallowed.
	var unsupported []string
	for fname, fn := range funcs {
		if !registerHandlebarsHelper(fname, fn) {
			unsupported = append(unsupported, fname)
		}
	}
	warnUnsupported(EngineHandlebars, unsupported)

	tmpl, err := raymond.Parse(content)
	if err != nil {
		return nil, err
	}
	return &HandlebarsTemplate{tmpl: tmpl}, nil
}

// ParseFile parses a template file
func (e *HandlebarsEngine) ParseFile(path string, funcs template.FuncMap) (Template, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's template files
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

// raymondRegistered remembers registered helper names (raymond panics on
// duplicates) and whether the source helper was adaptable (GO-054).
var raymondRegistered sync.Map // name → supported bool

// registerHandlebarsHelper registers a Go FuncMap helper as a raymond helper
// through the reflection adapter (GO-054). Fixed arities 0–3 are wrapped;
// variadic or wider signatures are reported as unsupported. A runtime
// conversion failure renders a visible "[helper X error: …]" marker and warns
// on stderr — the old recover() silently dropped such helpers entirely.
func registerHandlebarsHelper(name string, fn interface{}) bool {
	if supported, seen := raymondRegistered.Load(name); seen {
		return supported.(bool)
	}
	adapter, err := adaptHelper(fn)
	numIn := 0
	if err == nil {
		t := adapter.fn.Type()
		if t.IsVariadic() || t.NumIn() > 3 {
			err = fmt.Errorf("arity not expressible as a handlebars helper")
		} else {
			numIn = t.NumIn()
		}
	}
	if err != nil {
		raymondRegistered.Store(name, false)
		return false
	}
	raymondRegistered.Store(name, true)

	call := func(args ...interface{}) interface{} {
		res, cerr := adapter.call(args...)
		if cerr != nil {
			warnHelpersOnce(EngineHandlebars+":"+name,
				fmt.Sprintf("handlebars helper %q failed: %v", name, cerr))
			return fmt.Sprintf("[helper %s error: %v]", name, cerr)
		}
		if s, safe := helperResultString(res); safe {
			return raymond.SafeString(s)
		}
		return res
	}
	switch numIn {
	case 0:
		raymond.RegisterHelper(name, func() interface{} { return call() })
	case 1:
		raymond.RegisterHelper(name, func(a interface{}) interface{} { return call(a) })
	case 2:
		raymond.RegisterHelper(name, func(a, b interface{}) interface{} { return call(a, b) })
	default:
		raymond.RegisterHelper(name, func(a, b, c interface{}) interface{} { return call(a, b, c) })
	}
	return true
}
