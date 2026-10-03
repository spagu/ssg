package openapipage

import (
	"encoding/json"
	"strings"

	"github.com/spagu/ssg/internal/openapi"
)

// The operation as the console needs it (GO-118). Field names match
// assets/tryit.js in the generator.
type consoleOp struct {
	Method   string          `json:"method"`
	Path     string          `json:"path"`
	Servers  []string        `json:"servers,omitempty"`
	Params   []consoleParam  `json:"params,omitempty"`
	Body     *consoleBody    `json:"body,omitempty"`
	Security []consoleScheme `json:"security,omitempty"`
}

type consoleParam struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Example     any    `json:"example,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}

type consoleBody struct {
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Example  string `json:"example,omitempty"`
}

type consoleScheme struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	In          string `json:"in,omitempty"`
	ParamName   string `json:"paramName,omitempty"`
	Scheme      string `json:"scheme,omitempty"`
	Description string `json:"description,omitempty"`
}

// tryIt writes the console's marker with the operation as JSON.
func (w *writer) tryIt(op *openapi.Operation) {
	data, err := json.Marshal(w.site.console(op))
	if err != nil {
		return
	}
	w.line(`<div class="ssg-tryit" data-ssg-tryit="%s"></div>`+"\n", escape(string(data)))
}

// console collects the operation's servers, parameters, body and schemes.
func (s *site) console(op *openapi.Operation) consoleOp {
	c := consoleOp{Method: op.Method, Path: op.Path}
	servers := op.Servers
	if servers == nil {
		servers = s.spec.Servers
	}
	for _, sv := range servers {
		c.Servers = append(c.Servers, serverURL(sv))
	}
	for _, p := range op.Parameters {
		cp := consoleParam{Name: p.Name, In: p.In, Required: p.Required, Description: oneLine(p.Description), Example: p.Example}
		if p.Schema != nil {
			cp.Enum = p.Schema.Enum
			if cp.Enum == nil && strings.HasPrefix(p.Schema.Type, "boolean") {
				cp.Enum = []any{true, false} // a choice, not a text field
			}
			if cp.Example == nil {
				cp.Example = firstOf(p.Schema.Example, p.Schema.Default)
			}
		}
		c.Params = append(c.Params, cp)
	}
	if b := op.RequestBody; b != nil && len(b.Content) > 0 {
		mt := b.Content[0]
		c.Body = &consoleBody{Type: mt.Type, Required: b.Required, Example: exampleText(mt, s.schemas)}
	}
	seen := map[string]bool{}
	for _, r := range op.Security {
		for _, it := range r {
			if seen[it.Scheme] {
				continue
			}
			seen[it.Scheme] = true
			for _, sc := range s.spec.SecuritySchemes {
				if sc.Name == it.Scheme {
					c.Security = append(c.Security, consoleScheme{Name: sc.Name, Type: sc.Type, In: sc.In,
						ParamName: sc.ParamName, Scheme: sc.Scheme, Description: oneLine(sc.Description)})
				}
			}
		}
	}
	return c
}

// serverURL fills a server's variables with their defaults.
func serverURL(sv openapi.Server) string {
	u := sv.URL
	for name, v := range sv.Variables {
		u = strings.ReplaceAll(u, "{"+name+"}", v.Default)
	}
	return u
}

func firstOf(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}
