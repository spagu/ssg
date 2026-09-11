package components

// Rendering component calls in a page's content (GO-093).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Context is what a component's template sees.
//
// `Site` is the site data every template gets. `Props` are this call's
// arguments, already typed and defaulted.
//
// There is no `Page` yet, and the reason is worth writing down rather than
// leaving as an omission: content is rendered through one function map built
// once for the build, and the page a call came from is not in scope there. The
// honest options were a per-page template clone (expensive for every page of
// every site) or a goroutine-local current page (a race waiting to be written).
// A component that needs the page is a theme partial, which has it.
type Context struct {
	Props map[string]interface{}
	Site  interface{}
}

// Policy is what to do about a call to a component that exists and was made
// wrongly, mirroring `shortcode_errors` so a site has one setting for "my
// content is wrong", not two. An empty policy means the historical default,
// which is drop.
const (
	PolicyDrop   = "drop"
	PolicyKeep   = "keep"
	PolicyStrict = "strict"
)

// errUnknownComponent marks a call naming something this site does not define,
// which the renderer treats differently from a call it could have made.
var errUnknownComponent = errors.New("no component named")

// Result reports what a render did, so the caller can copy assets for the
// components a page actually used and report what it could not resolve.
type Result struct {
	// Used names the components this content rendered, in call order,
	// deduplicated.
	Used []string
	// Warnings are the calls that did not resolve, one line each.
	Warnings []string
	// Err is set when the policy is strict and something did not resolve.
	Err error
}

// Render replaces every resolvable component call in content.
//
// wrap is applied to each component's output before it goes back into the
// content — the generator uses it to protect author-written markup from the
// sanitizer, the same way it protects a shortcode's.
func (s *Set) Render(content string, site interface{}, policy, source string, wrap func(string) string) (string, Result) {
	if wrap == nil {
		wrap = func(html string) string { return html }
	}
	return s.RenderMarked(content, site, policy, source, func(_, html string) string { return wrap(html) })
}

// RenderMarked is Render with the component's name handed to the wrapper, for a
// caller that needs to know which component produced which markup.
func (s *Set) RenderMarked(content string, site interface{}, policy, source string, wrap func(name, html string) string) (string, Result) {
	var res Result
	if s == nil || s.Len() == 0 || !strings.Contains(content, "{{<") {
		return content, res
	}
	if wrap == nil {
		wrap = func(_, html string) string { return html }
	}
	seen := map[string]bool{}
	var b strings.Builder
	last := 0
	for _, call := range FindCalls(content) {
		b.WriteString(content[last:call.Start])
		last = call.End

		html, err := s.renderCall(call, site, &res, seen, source)
		if err != nil {
			if errors.Is(err, errUnknownComponent) {
				// Not a component this site has. That is not a broken call —
				// it is text that looks like one, and documentation quoting a
				// call is the ordinary case. It stays exactly as written,
				// under every policy, with a warning in case it was a typo.
				b.WriteString(call.Raw)
				continue
			}
			if policy == PolicyStrict && res.Err == nil {
				res.Err = err
			}
			// A call to a component that DOES exist, made wrongly, follows the
			// site's `shortcode_errors`: drop takes it out of the page (the
			// historical default), keep leaves it visible for an author
			// proofreading, strict stops the build. None invents markup.
			if policy == PolicyKeep || policy == PolicyStrict {
				b.WriteString(call.Raw)
			}
			continue
		}
		b.WriteString(wrap(call.Name, html))
	}
	b.WriteString(content[last:])
	return b.String(), res
}

// renderCall resolves and renders one call, recording what it used.
func (s *Set) renderCall(call Call, site interface{}, res *Result, seen map[string]bool, source string) (string, error) {
	c, ok := s.Get(call.Name)
	if !ok {
		// Not ours. Left exactly as written — see the note in call.go about
		// why an unknown call is not deleted.
		err := fmt.Errorf("%s: %w %q (have: %s) — in %s", where(source), errUnknownComponent, call.Name, strings.Join(s.Names(), ", "), call.Raw)
		res.Warnings = append(res.Warnings, err.Error())
		return "", err
	}
	props, err := c.Resolve(call)
	if err != nil {
		err = fmt.Errorf("%s: %w — in %s", where(source), err, call.Raw)
		res.Warnings = append(res.Warnings, err.Error())
		return "", err
	}
	var buf bytes.Buffer
	if err := c.Template.Execute(&buf, Context{Props: props, Site: site}); err != nil {
		err = fmt.Errorf("%s: component %q failed to render: %w — in %s", where(source), c.Name, err, call.Raw)
		res.Warnings = append(res.Warnings, err.Error())
		return "", err
	}
	if !seen[c.Name] {
		seen[c.Name] = true
		res.Used = append(res.Used, c.Name)
	}
	return buf.String(), nil
}

// where names the content file in a message.
//
// The content pipeline does not carry the file: content is rendered through one
// function map built for the whole build, and adding a per-page one would mean
// cloning the theme for every page. So a message names the CALL instead, which
// is a unique string an author can search their content for — and the call is
// what they need to look at anyway.
func where(source string) string {
	if source == "" {
		return "component"
	}
	return source
}

// Manifest is the contract, published for whoever writes a call — a person
// reading it, or an agent generating one (GO-095's site graph describes the
// site; this describes what content can invoke).
type Manifest struct {
	Schema     int                 `json:"schema"`
	Components []ManifestComponent `json:"components"`
}

// ManifestComponent is one component's public contract.
type ManifestComponent struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Example     string         `json:"example"`
	Props       []ManifestProp `json:"props"`
}

// ManifestProp is one declared argument.
type ManifestProp struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Description string      `json:"description,omitempty"`
}

// ManifestSchema versions the file.
const ManifestSchema = 1

// ManifestFileName is where the manifest is published.
const ManifestFileName = "components.json"

// Manifest describes every component this site defines.
func (s *Set) Manifest() Manifest {
	m := Manifest{Schema: ManifestSchema, Components: []ManifestComponent{}}
	for _, name := range s.Names() {
		c, _ := s.Get(name)
		mc := ManifestComponent{Name: c.Name, Description: c.Description, Props: []ManifestProp{}}
		for _, prop := range c.PropNames() {
			def := c.Props[prop]
			kind := def.Type
			if kind == "" {
				kind = TypeString
			}
			mc.Props = append(mc.Props, ManifestProp{
				Name: prop, Type: kind, Required: def.Required, Default: def.Default,
				Enum: def.Enum, Description: def.Description,
			})
		}
		mc.Example = example(c)
		m.Components = append(m.Components, mc)
	}
	return m
}

// example writes the shortest call that would validate, so a reader has
// something to copy rather than a grammar to infer.
func example(c *Component) string {
	var parts []string
	for _, prop := range c.PropNames() {
		def := c.Props[prop]
		if !def.Required {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", prop, sampleValue(def)))
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return fmt.Sprintf("{{< %s >}}", c.Name)
	}
	return fmt.Sprintf("{{< %s %s >}}", c.Name, strings.Join(parts, " "))
}

// sampleValue is a plausible value for a prop, in the syntax a call uses.
func sampleValue(def Prop) string {
	switch {
	case len(def.Enum) > 0:
		return `"` + def.Enum[0] + `"`
	case def.Type == TypeNumber:
		return "1"
	case def.Type == TypeBool:
		return "true"
	case def.Type == TypeArray:
		return `"a, b"`
	}
	return `"…"`
}

// EncodeManifest renders the manifest as the file that ships.
//
// HTML escaping is off: the examples are component calls, and `{{< youtube >}}`
// written as `{{\u003c youtube \u003e}}` is a contract nobody can copy.
func (s *Set) EncodeManifest() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.Manifest()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
