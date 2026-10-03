package openapi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// methods is the fixed order operations of one path item are listed in.
var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// templateParam matches "{name}" placeholders in a path template.
var templateParam = regexp.MustCompile(`\{([^{}]+)\}`)

// pathItem is what a path item hands down to each of its operations.
type pathItem struct {
	path     string
	params   []Parameter
	servers  []Server
	security []Requirement
}

// operations reads every operation under paths, in document order.
func (p *parser) operations(global []Requirement) []*Operation {
	var ops []*Operation
	used := map[string]bool{}
	for _, e := range pairs(get(p.root, "paths")) {
		if isExtension(e.key) {
			continue
		}
		at := pointer("#/paths", e.key)
		node := p.deref(e.value, at)
		if node == nil {
			continue
		}
		item := pathItem{
			path:     e.key,
			params:   p.parameters(get(node, "parameters"), at+"/parameters"),
			servers:  p.servers(get(node, "servers"), at+"/servers"),
			security: global,
		}
		for _, m := range methods {
			if opNode := get(node, m); opNode != nil {
				ops = append(ops, p.operation(m, opNode, pointer(at, m), item, used))
			}
		}
	}
	return ops
}

// operation reads one operation object located at base.
func (p *parser) operation(method string, n *yaml.Node, base string, item pathItem, used map[string]bool) *Operation {
	op := &Operation{
		ID:          p.operationID(method, n, base, item.path, used),
		Method:      strings.ToUpper(method),
		Path:        item.path,
		Summary:     str(get(n, "summary")),
		Description: str(get(n, "description")),
		Tags:        strs(get(n, "tags")),
		Deprecated:  flag(get(n, "deprecated")),
		Parameters:  mergeParams(item.params, p.parameters(get(n, "parameters"), base+"/parameters")),
		Responses:   p.responses(get(n, "responses"), base+"/responses"),
		Security:    item.security,
		Servers:     item.servers,
	}
	p.checkPathParams(op, base)
	if rb := get(n, "requestBody"); rb != nil {
		op.RequestBody = p.requestBody(rb, base+"/requestBody")
	}
	if len(op.Responses) == 0 {
		p.diag(base+"/responses", "operation has no responses")
	}
	if sec := get(n, "security"); sec != nil {
		op.Security = p.security(sec, base+"/security")
	}
	if srv := get(n, "servers"); srv != nil {
		op.Servers = p.servers(srv, base+"/servers")
	}
	return op
}

// operationID returns the explicit or derived ID, made unique against used.
func (p *parser) operationID(method string, n *yaml.Node, base, path string, used map[string]bool) string {
	explicit := str(get(n, "operationId"))
	id := explicit
	if id == "" {
		id = slug(method + " " + path)
	}
	if used[id] {
		if explicit != "" {
			p.diag(base+"/operationId", "duplicate operationId %q", explicit)
		}
		stem := id
		for i := 2; used[id]; i++ {
			id = fmt.Sprintf("%s-%d", stem, i)
		}
	}
	used[id] = true
	return id
}

// slug keeps ASCII letters and digits and turns every other run into one '-'.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}

// parameters reads a parameter list, resolving $refs.
func (p *parser) parameters(n *yaml.Node, base string) []Parameter {
	var out []Parameter
	for i, item := range items(n) {
		at := pointer(base, strconv.Itoa(i))
		node := p.deref(item, at)
		if node == nil {
			continue
		}
		prm := Parameter{
			Name:        str(get(node, "name")),
			In:          str(get(node, "in")),
			Description: str(get(node, "description")),
			Required:    flag(get(node, "required")),
			Deprecated:  flag(get(node, "deprecated")),
			Schema:      p.schema(get(node, "schema"), at+"/schema"),
			Example:     p.example(node, at),
		}
		if prm.Schema == nil {
			if mts := p.content(get(node, "content"), at+"/content"); len(mts) > 0 {
				prm.Schema = mts[0].Schema
			}
		}
		out = append(out, prm)
	}
	return out
}

// mergeParams overlays operation parameters on path-item ones by name and in.
func mergeParams(shared, own []Parameter) []Parameter {
	out := append([]Parameter(nil), shared...)
	for _, prm := range own {
		replaced := false
		for i := range out {
			if out[i].Name == prm.Name && out[i].In == prm.In {
				out[i], replaced = prm, true
			}
		}
		if !replaced {
			out = append(out, prm)
		}
	}
	return out
}

// checkPathParams reports template placeholders with no "in: path" parameter.
func (p *parser) checkPathParams(op *Operation, base string) {
	for _, m := range templateParam.FindAllStringSubmatch(op.Path, -1) {
		found := false
		for _, prm := range op.Parameters {
			found = found || (prm.In == "path" && prm.Name == m[1])
		}
		if !found {
			p.diag(base+"/parameters", "path parameter %q has no matching in: path parameter", m[1])
		}
	}
}
