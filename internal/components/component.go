package components

// Typed content components (GO-093).
//
// A shortcode today is an entry in the site config: a fixed name that renders
// fixed data. `{{gallery}}` renders the one gallery the config describes, and a
// second gallery means a second entry. There is no way to say "this one, with
// these pictures, three across" from inside the content that wants it.
//
// A component is a directory with a contract:
//
//	components/gallery/
//	  component.yaml    props: their types, which are required, their defaults
//	  template.html     the markup, rendered by html/template
//	  assets/           css and js, copied and linked only on pages that use it
//
// and a call that carries its arguments:
//
//	{{< gallery src="photos/" columns=3 >}}
//
// The schema is the point. It makes a component something a person can use
// without reading its template, something the build can check before the page
// ships, and something an agent can generate a correct call for — the manifest
// this package publishes is that contract, written down.

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultDir is where components live unless the site says otherwise.
const DefaultDir = "components"

// schemaFile and templateFile are the two files a component must have; assetDir
// is the optional third.
const (
	schemaFile   = "component.yaml"
	templateFile = "template.html"
	assetDir     = "assets"
)

// Prop types. A value from content is text until the schema says what it is.
const (
	TypeString = "string"
	TypeNumber = "number"
	TypeBool   = "bool"
	TypeArray  = "array"
)

// Prop is one declared argument of a component.
type Prop struct {
	Type        string      `yaml:"type"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default"`
	Enum        []string    `yaml:"enum"`
	Description string      `yaml:"description"`
}

// schema is component.yaml as written.
type schema struct {
	Description string          `yaml:"description"`
	Props       map[string]Prop `yaml:"props"`
}

// Component is one loaded component.
type Component struct {
	Name        string
	Description string
	Props       map[string]Prop
	// Template renders the component with {Props, Site}.
	Template *template.Template
	// Assets are the files under assets/, relative to the component directory,
	// in a stable order.
	Assets []string
	// Dir is where the component was loaded from.
	Dir string
}

// PropNames lists the declared props in a stable order.
func (c *Component) PropNames() []string {
	names := make([]string, 0, len(c.Props))
	for name := range c.Props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Set is every component a site defines.
type Set struct {
	byName map[string]*Component
	order  []string
	dir    string
}

// Names lists the components in a stable order.
func (s *Set) Names() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.order...)
}

// Len is how many components were loaded; a nil set has none.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.order)
}

// Get returns one component by name.
func (s *Set) Get(name string) (*Component, bool) {
	if s == nil {
		return nil, false
	}
	c, ok := s.byName[name]
	return c, ok
}

// Dir is where this set was loaded from.
func (s *Set) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// Load reads every component under dir.
//
// A directory that is not there is not an error: components are opt-in, and a
// site that has none should build exactly as it always did. A directory that IS
// there and holds something malformed is an error, because the author meant it.
//
// funcs are the theme's template functions, so a component can call the same
// helpers a partial can.
func Load(dir string, funcs template.FuncMap) (*Set, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	set := &Set{byName: map[string]*Component{}, dir: dir}
	var warnings []string
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		c, warn, err := loadOne(filepath.Join(dir, name), name, funcs)
		if err != nil {
			return nil, warnings, err
		}
		if warn != "" {
			warnings = append(warnings, warn)
			continue
		}
		set.byName[name] = c
		set.order = append(set.order, name)
	}
	return set, warnings, nil
}

// loadOne reads one component directory. A directory with no template is
// skipped with a warning rather than failing the build: a stray folder under
// components/ is a mistake to point out, not a reason to stop.
func loadOne(dir, name string, funcs template.FuncMap) (*Component, string, error) {
	if !validName(name) {
		return nil, fmt.Sprintf("component %q: the name must be lowercase letters, digits, - or _", name), nil
	}
	tmplPath := filepath.Join(dir, templateFile)
	raw, err := os.ReadFile(tmplPath) // #nosec G304 -- the project's own components directory
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Sprintf("component %q has no %s and was skipped", name, templateFile), nil
		}
		return nil, "", err
	}
	c := &Component{Name: name, Dir: dir, Props: map[string]Prop{}}

	if data, err := os.ReadFile(filepath.Join(dir, schemaFile)); err == nil { // #nosec G304 -- same directory
		var sc schema
		if err := yaml.Unmarshal(data, &sc); err != nil {
			return nil, "", fmt.Errorf("component %q: %s: %w", name, schemaFile, err)
		}
		c.Description = sc.Description
		for prop, def := range sc.Props {
			if err := validateProp(name, prop, def); err != nil {
				return nil, "", err
			}
			c.Props[prop] = def
		}
	} else if !os.IsNotExist(err) {
		return nil, "", err
	}

	tmpl := template.New(name)
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	parsed, err := tmpl.Parse(string(raw))
	if err != nil {
		return nil, "", fmt.Errorf("component %q: %s: %w", name, templateFile, err)
	}
	c.Template = parsed

	assets, err := readAssets(filepath.Join(dir, assetDir))
	if err != nil {
		return nil, "", fmt.Errorf("component %q: %s: %w", name, assetDir, err)
	}
	c.Assets = assets
	return c, "", nil
}

// validateProp refuses a schema that cannot mean anything, at load time, where
// the author is looking — rather than at the first call that trips over it.
func validateProp(component, prop string, p Prop) error {
	switch p.Type {
	case "", TypeString, TypeNumber, TypeBool, TypeArray:
	default:
		return fmt.Errorf("component %q: prop %q has type %q (want string, number, bool or array)", component, prop, p.Type)
	}
	if len(p.Enum) > 0 && p.Type != "" && p.Type != TypeString {
		return fmt.Errorf("component %q: prop %q lists enum values, which only a string prop can have", component, prop)
	}
	if p.Required && p.Default != nil {
		return fmt.Errorf("component %q: prop %q is required and also has a default — one or the other", component, prop)
	}
	if len(p.Enum) > 0 && p.Default != nil {
		if s, ok := p.Default.(string); !ok || !contains(p.Enum, s) {
			return fmt.Errorf("component %q: prop %q defaults to %v, which is not one of its enum values", component, prop, p.Default)
		}
	}
	return nil
}

// readAssets lists the files under a component's assets directory, in a stable
// order, relative to that directory.
func readAssets(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // no assets is the common case
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// validName keeps a component addressable and safe to put in a path.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// contains reports membership.
func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// StyleAssets and ScriptAssets split a component's assets by what a page has to
// do with them.
func (c *Component) StyleAssets() []string  { return byExt(c.Assets, ".css") }
func (c *Component) ScriptAssets() []string { return byExt(c.Assets, ".js") }

func byExt(assets []string, ext string) []string {
	var out []string
	for _, a := range assets {
		if strings.EqualFold(filepath.Ext(a), ext) {
			out = append(out, a)
		}
	}
	return out
}
