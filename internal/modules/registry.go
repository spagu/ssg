package modules

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// CoreID is the id of the worker that consumes the registry (GO-145). The
// build writes a registry only when a worker in `workers:` carries a
// module.json with this id; without one it writes nothing new.
const CoreID = "core"

// RegistryFormat is the version of the registry document. Readers refuse a
// newer one, as with the manifest contract.
const RegistryFormat = 1

// RegistryFile is the registry's name inside the output's functions/ tree.
// Pages Functions cannot read files at runtime, so the core function imports
// it (`import registry from "../../_ssg-modules.json" with { type: "json" }`)
// and wrangler bundles it. The leading underscore keeps it out of the route
// table, and wrangler never uploads functions/ as static assets, so the file
// is not publicly served.
const RegistryFile = "_ssg-modules.json"

// Registry is the document written for the core worker. Field order is the
// struct's and every list is sorted, so the same site gives the same bytes.
type Registry struct {
	Format  int     `json:"format"`
	Modules []Entry `json:"modules"`
	// Available lists the templates this ssg ships that the site does not
	// use, for a "modules you could add" view (GO-149).
	Available []Available `json:"available"`
	// Theme is the panels' look from `admin_theme:` (GO-152, theme.go); null
	// when the site sets none.
	Theme *Theme `json:"theme"`
}

// Entry is one module. Kind, Contract, Version and Manifest are null when the
// build has no manifest for it (a `modules:` entry without dir); core then
// fetches the manifest from the module itself.
type Entry struct {
	ID string `json:"id"`
	// Kind is the manifest's kind: pages-function or worker.
	Kind *string `json:"kind"`
	// Base is the module's base path: "/" for a Pages Function in this build,
	// the configured url for a module deployed elsewhere on the host.
	Base string `json:"base"`
	// Bundled is true for a Pages Function built into this site's functions/,
	// false for a module deployed on its own (`modules:`).
	Bundled  bool    `json:"bundled"`
	Contract *int    `json:"contract"`
	Version  *string `json:"version"`
	Manifest any     `json:"manifest"`
}

// Available is a template the site could add.
type Available struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// Encode renders the registry as indented JSON with a trailing newline.
// Manifests are re-encoded from their decoded form, so object keys come out
// sorted whatever order the module.json had.
func Encode(r Registry) ([]byte, error) {
	if r.Modules == nil {
		r.Modules = []Entry{}
	}
	if r.Available == nil {
		r.Available = []Available{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Render collects the registry and encodes it — what a build writes. The
// schema is the one this binary embeds unless in.Schema is set.
func Render(in Input) ([]byte, []Finding, error) {
	if in.Schema == nil {
		s, err := EmbeddedSchema()
		if err != nil {
			return nil, nil, err
		}
		in.Schema = s
	}
	reg, findings := Collect(in)
	reg.Theme = in.Theme
	data, err := Encode(reg)
	return data, findings, err
}

// IsCore reports whether dir holds the core worker: a module.json whose id is
// CoreID. Only the id is read — the full checks run when the registry is
// assembled, and report a broken core manifest there.
func IsCore(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "module.json")) // #nosec G304 -- a worker dir from the project's own config
	if err != nil {
		return false
	}
	var m struct {
		ID string `json:"id"`
	}
	return json.Unmarshal(data, &m) == nil && m.ID == CoreID
}

// entryFor builds a registry entry from a valid manifest.
func entryFor(m *Manifest, base string, bundled bool) Entry {
	kind, version, contract := m.Kind, m.Version, m.Contract
	return Entry{ID: m.ID, Kind: &kind, Base: base, Bundled: bundled, Contract: &contract, Version: &version, Manifest: m.Raw}
}
