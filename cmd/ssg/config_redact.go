package main

// Redaction of `ssg config view --effective` (GO-101; SR-15 of the GO-183
// security review).
//
// The effective view is the resolved configuration: a credential the file only
// referenced as $NAME is a literal by the time it is printed. Which keys carry
// a credential is decided by the configuration schema (class "secret"), the
// same table designer-ui and `ssg mcp` use, so a secret key added to the
// configuration is masked here without anyone remembering this file. Before
// SR-15 the list was written by hand and had missed several keys
// (`tts.api_key`, `workers[].auth.*`, `endpoints[].password`,
// `notifications[].headers`, `ai.models.*.key`).

import (
	"gopkg.in/yaml.v3"

	"github.com/spagu/ssg/internal/config"
)

// redactedValue is what a credential reads as in the effective view.
const redactedValue = "«redacted — read it from the file or the environment»"

// redactedWhole are keys masked whole since GO-101 although the schema does not
// class them "secret": a TLS private key's path, and sections whose free-form
// parts (worker config, deploy settings, notify targets, external sources) may
// hold a literal credential. Kept so no value printed masked before is printed
// now. Every other masked key comes from the schema.
var redactedWhole = map[string]bool{
	"tls_key": true, "external_sources.sources": true,
	"ai": true, "worker": true, "deploy": true, "notify": true,
}

// marshalYAML is yaml.Marshal; a variable so a test can make it fail.
var marshalYAML = yaml.Marshal

// redactSecrets returns the YAML with every credential-bearing value replaced
// by redactedValue: every key of class "secret" wherever it sits (inside list
// elements and map values too) and the sections in redactedWhole. Anything it
// cannot read or write back becomes one redacted line — better to print
// nothing than a credential.
func redactSecrets(data []byte) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return []byte(redactedValue + "\n")
	}
	root := &config.SchemaNode{Type: "object", Fields: config.BuildSchema().Keys}
	redactNode(doc.Content[0], root)
	out, err := marshalYAML(doc.Content[0])
	if err != nil {
		return []byte(redactedValue + "\n")
	}
	return out
}

// redactNode walks value v described by schema node n and masks the values of
// secret keys below it.
func redactNode(v *yaml.Node, n *config.SchemaNode) {
	n = shapeFor(v, n)
	switch {
	case v.Kind == yaml.SequenceNode && n.Items != nil:
		for _, e := range v.Content {
			redactNode(e, n.Items)
		}
	case v.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(v.Content); i += 2 {
			c := childNode(n, v.Content[i].Value)
			if c == nil {
				continue // an open object's own keys: no class of their own
			}
			if c.Class == config.ClassSecret || redactedWhole[c.Path] {
				v.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: redactedValue}
				continue
			}
			redactNode(v.Content[i+1], c)
		}
	}
}

// shapeFor picks the form of n the value has: the primary one, or the
// alternate (`story: true`, `languages` as full entries) when only it fits.
func shapeFor(v *yaml.Node, n *config.SchemaNode) *config.SchemaNode {
	if n.Alternate != nil && !fitsNode(v, n) && fitsNode(v, n.Alternate) {
		return n.Alternate
	}
	return n
}

// fitsNode: does the value have the node's kind — a list, a mapping, a scalar?
func fitsNode(v *yaml.Node, n *config.SchemaNode) bool {
	switch v.Kind {
	case yaml.SequenceNode:
		return n.Type == "list"
	case yaml.MappingNode:
		return n.Type == "object" || n.Type == "map"
	}
	return n.Type != "list" && n.Type != "object" && n.Type != "map"
}

// childNode is the schema node of key inside n: a map's value node or an
// object's named field; nil when n names no such key.
func childNode(n *config.SchemaNode, key string) *config.SchemaNode {
	if n.Type == "map" {
		return n.Values
	}
	for _, f := range n.Fields {
		if f.Key == key {
			return f
		}
	}
	return nil
}
