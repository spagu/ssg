package openapi

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses the OpenAPI document at path. See Parse.
func Load(path string) (*Spec, []Diagnostic, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the configured spec file
	if err != nil {
		return nil, nil, fmt.Errorf("openapi: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse reads an OpenAPI 3.x document given as YAML or JSON. It returns an
// error only when the input is not YAML/JSON, not an object, or not OpenAPI
// 3.x; every other problem becomes a Diagnostic and the read carries on.
func Parse(data []byte) (*Spec, []Diagnostic, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("openapi: not YAML or JSON: %w", err)
	}
	root := &doc
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, nil, errors.New("openapi: document is not an object")
	}
	version := str(get(root, "openapi"))
	if version == "" {
		return nil, nil, errors.New(`openapi: missing "openapi" version field`)
	}
	if !strings.HasPrefix(version, "3.") {
		return nil, nil, fmt.Errorf("openapi: unsupported version %q, want 3.x", version)
	}
	p := &parser{root: root}
	return p.spec(version), p.diags, nil
}

// spec builds the whole model from the document root.
func (p *parser) spec(version string) *Spec {
	info := get(p.root, "info")
	s := &Spec{
		Version:     version,
		Title:       str(get(info, "title")),
		APIVersion:  str(get(info, "version")),
		Summary:     str(get(info, "summary")),
		Description: str(get(info, "description")),
		Servers:     p.servers(get(p.root, "servers"), "#/servers"),
		Security:    p.security(get(p.root, "security"), "#/security"),
	}
	s.SecuritySchemes = p.securitySchemes()
	s.Schemas = p.schemas()
	s.Operations = p.operations(s.Security)
	s.Tags = p.tags(s.Operations)
	return s
}

// schemas reads components.schemas in document order.
func (p *parser) schemas() []*NamedSchema {
	var out []*NamedSchema
	for _, e := range pairs(get(get(p.root, "components"), "schemas")) {
		if isExtension(e.key) {
			continue
		}
		at := pointer("#/components/schemas", e.key)
		out = append(out, &NamedSchema{Name: e.key, Schema: p.schema(e.value, at)})
	}
	return out
}

// tags orders declared tags first, then undeclared used ones, then "default"
// for untagged operations (which are given that tag here).
func (p *parser) tags(ops []*Operation) []Tag {
	var out []Tag
	seen := map[string]bool{}
	add := func(name, description string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, Tag{Name: name, Description: description})
		}
	}
	for _, n := range items(get(p.root, "tags")) {
		add(str(get(n, "name")), str(get(n, "description")))
	}
	untagged := false
	for _, op := range ops {
		if len(op.Tags) == 0 {
			untagged = true
			op.Tags = []string{"default"}
			continue
		}
		for _, t := range op.Tags {
			add(t, "")
		}
	}
	if untagged {
		add("default", "")
	}
	return out
}

// servers reads a servers list; nil when the list is absent.
func (p *parser) servers(n *yaml.Node, base string) []Server {
	if n == nil {
		return nil
	}
	out := make([]Server, 0, len(n.Content))
	for i, item := range items(n) {
		srv := Server{URL: str(get(item, "url")), Description: str(get(item, "description"))}
		if srv.URL == "" {
			p.diag(pointer(base, strconv.Itoa(i)), "server without url")
		}
		for _, v := range pairs(get(item, "variables")) {
			if srv.Variables == nil {
				srv.Variables = map[string]ServerVariable{}
			}
			srv.Variables[v.key] = ServerVariable{
				Default:     str(get(v.value, "default")),
				Enum:        strs(get(v.value, "enum")),
				Description: str(get(v.value, "description")),
			}
		}
		out = append(out, srv)
	}
	return out
}
