package openapi

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

// requestBody reads a request body, resolving a $ref; nil when unresolvable.
func (p *parser) requestBody(n *yaml.Node, at string) *RequestBody {
	node := p.deref(n, at)
	if node == nil {
		return nil
	}
	return &RequestBody{
		Description: str(get(node, "description")),
		Required:    flag(get(node, "required")),
		Content:     p.content(get(node, "content"), at+"/content"),
	}
}

// responses reads a responses map in document order, resolving $refs.
func (p *parser) responses(n *yaml.Node, base string) []Response {
	var out []Response
	for _, e := range pairs(n) {
		if isExtension(e.key) {
			continue
		}
		at := pointer(base, e.key)
		node := p.deref(e.value, at)
		if node == nil {
			continue
		}
		out = append(out, Response{
			Status:      e.key,
			Description: str(get(node, "description")),
			Content:     p.content(get(node, "content"), at+"/content"),
			Headers:     p.headers(get(node, "headers"), at+"/headers"),
		})
	}
	return out
}

// headers reads a headers map in document order, resolving $refs.
func (p *parser) headers(n *yaml.Node, base string) []Header {
	var out []Header
	for _, e := range pairs(n) {
		at := pointer(base, e.key)
		node := p.deref(e.value, at)
		if node == nil {
			continue
		}
		out = append(out, Header{
			Name:        e.key,
			Description: str(get(node, "description")),
			Schema:      p.schema(get(node, "schema"), at+"/schema"),
		})
	}
	return out
}

// content reads a content map (media type -> media type object).
func (p *parser) content(n *yaml.Node, base string) []MediaType {
	var out []MediaType
	for _, e := range pairs(n) {
		at := pointer(base, e.key)
		out = append(out, MediaType{
			Type:    e.key,
			Schema:  p.schema(get(e.value, "schema"), at+"/schema"),
			Example: p.example(e.value, at),
		})
	}
	return out
}

// example returns "example", else the value of the first entry of the
// "examples" map (resolving a $ref to components.examples), else nil.
func (p *parser) example(n *yaml.Node, at string) any {
	if ex := get(n, "example"); ex != nil {
		return toAny(ex)
	}
	examples := pairs(get(n, "examples"))
	if len(examples) == 0 {
		return nil
	}
	first := examples[0]
	return toAny(get(p.deref(first.value, pointer(at, "examples", first.key)), "value"))
}

// security reads a security requirement list. Absent gives nil; an explicit
// empty list gives a non-nil empty slice ("no authentication").
func (p *parser) security(n *yaml.Node, base string) []Requirement {
	if n == nil {
		return nil
	}
	out := make([]Requirement, 0, len(n.Content))
	for i, item := range items(n) {
		at := pointer(base, strconv.Itoa(i))
		req := Requirement{}
		for _, e := range pairs(item) {
			if !p.hasComponent("securitySchemes", e.key) {
				p.diag(at, "unknown security scheme %q", e.key)
			}
			req = append(req, RequirementItem{Scheme: e.key, Scopes: strs(e.value)})
		}
		out = append(out, req)
	}
	return out
}

// securitySchemes reads components.securitySchemes in document order.
func (p *parser) securitySchemes() []SecurityScheme {
	var out []SecurityScheme
	for _, e := range pairs(get(get(p.root, "components"), "securitySchemes")) {
		if isExtension(e.key) {
			continue
		}
		node := p.deref(e.value, pointer("#/components/securitySchemes", e.key))
		if node == nil {
			continue
		}
		out = append(out, SecurityScheme{
			Name:         e.key,
			Type:         str(get(node, "type")),
			Description:  str(get(node, "description")),
			In:           str(get(node, "in")),
			ParamName:    str(get(node, "name")),
			Scheme:       str(get(node, "scheme")),
			BearerFormat: str(get(node, "bearerFormat")),
		})
	}
	return out
}
