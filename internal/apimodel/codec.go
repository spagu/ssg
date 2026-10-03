package apimodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Sort puts the model in its canonical order — packages by name, modules by
// ID, symbols and members by ID — so two builds of the same sources write
// byte-identical api.json whatever order the extractor met files in.
// Signatures keep their order: overloads are listed as declared.
func (a *API) Sort() {
	sort.SliceStable(a.Packages, func(i, j int) bool { return a.Packages[i].Name < a.Packages[j].Name })
	for _, p := range a.Packages {
		sort.SliceStable(p.Modules, func(i, j int) bool { return p.Modules[i].ID < p.Modules[j].ID })
		for _, m := range p.Modules {
			sortSymbols(m.Symbols)
		}
	}
}

// sortSymbols orders symbols and, recursively, their members by ID.
func sortSymbols(syms []*Symbol) {
	sort.SliceStable(syms, func(i, j int) bool { return syms[i].ID < syms[j].ID })
	for _, s := range syms {
		sortSymbols(s.Members)
	}
}

// Encode writes the model as indented JSON in canonical order, with the
// current schema version.
func Encode(a *API) ([]byte, error) {
	a.Schema = Schema
	a.Sort()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // "<T>" in a signature stays readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(a); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode reads api.json. A document from another schema version is refused
// with both numbers, rather than read into fields that mean something else.
func Decode(data []byte) (*API, error) {
	var a API
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("api model: %w", err)
	}
	if a.Schema != Schema {
		return nil, fmt.Errorf("api model: schema %d, this ssg reads schema %d", a.Schema, Schema)
	}
	return &a, nil
}
