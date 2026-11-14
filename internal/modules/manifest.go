package modules

import (
	"encoding/json"
	"fmt"
	"slices"

	ssgroot "github.com/spagu/ssg"
)

// ManifestContract is the newest manifest contract this reader understands
// (MANIFEST_CONTRACT in workers/module-manifest.ts). A manifest with a higher
// `contract` is refused before anything else is checked: its fields may mean
// something this reader does not know.
const ManifestContract = 1

// Kinds a manifest declares (docs/WORKER-MODULES.md).
const (
	KindPagesFunction = "pages-function"
	KindWorker        = "worker"
)

// Manifest is the part of a valid module.json the build itself reads. Raw is
// the whole document as decoded, unknown fields included, so the registry can
// pass it on untouched.
type Manifest struct {
	ID       string            `json:"id"`
	Contract int               `json:"contract"`
	Version  string            `json:"version"`
	Kind     string            `json:"kind"`
	Title    map[string]string `json:"title"`
	Panel    *string           `json:"panel"`
	Nav      []struct {
		Path       string `json:"path"`
		Permission string `json:"permission"`
	} `json:"nav"`
	Permissions []struct {
		Name string `json:"name"`
	} `json:"permissions"`
	Raw any `json:"-"`
}

// embeddedSchemaPath is module.schema.json inside the binary (embedded.go).
var embeddedSchemaPath = "workers/module.schema.json"

// EmbeddedSchema parses the schema this ssg binary ships.
func EmbeddedSchema() (*Schema, error) {
	data, err := ssgroot.EmbeddedWorkers.ReadFile(embeddedSchemaPath)
	if err != nil {
		return nil, err
	}
	return ParseSchema(data)
}

// ReadManifest checks a module.json: the contract first (a newer one is
// refused with a message that says what to do), then the schema, then the
// rules that tie fields together. Unknown fields are ignored, never refused.
// The errors are readManifest's in the TypeScript reader, sorted.
func (s *Schema) ReadManifest(data []byte) (*Manifest, []string) {
	value, ok := decodeJSON(data)
	if !ok {
		return nil, []string{"manifest: not valid JSON"}
	}
	obj := asObject(value)
	if c, ok := obj["contract"].(json.Number); ok && typeOf(c) == "integer" {
		if n, _ := c.Float64(); n > ManifestContract {
			who := "this module"
			if id, ok := obj["id"].(string); ok {
				who = `module "` + id + `"`
			}
			return nil, []string{fmt.Sprintf("%s uses manifest contract %d, but this reader understands contract %d at most — upgrade ssg (and the core worker) to read it", who, int64(n), ManifestContract)}
		}
	}
	if errs := s.Validate(value); len(errs) > 0 {
		return nil, errs
	}
	var m Manifest
	_ = json.Unmarshal(data, &m) // the schema has guaranteed every field's shape
	m.Raw = value
	if errs := crossChecks(&m); len(errs) > 0 {
		return nil, errs
	}
	return &m, nil
}

// crossChecks: what a JSON Schema cannot say about a valid-shaped manifest.
func crossChecks(m *Manifest) []string {
	var errs []string
	declared := make([]string, 0, len(m.Permissions))
	for _, p := range m.Permissions {
		if slices.Contains(declared, p.Name) && !slices.Contains(errs, fmt.Sprintf("manifest.permissions: %q is declared twice", p.Name)) {
			errs = append(errs, fmt.Sprintf("manifest.permissions: %q is declared twice", p.Name))
		}
		declared = append(declared, p.Name)
	}
	var paths []string
	for i, n := range m.Nav {
		if slices.Contains(paths, n.Path) && !slices.Contains(errs, fmt.Sprintf("manifest.nav: path %q appears twice", n.Path)) {
			errs = append(errs, fmt.Sprintf("manifest.nav: path %q appears twice", n.Path))
		}
		paths = append(paths, n.Path)
		if !slices.Contains(declared, n.Permission) {
			errs = append(errs, fmt.Sprintf("manifest.nav[%d].permission: %q is not declared in permissions", i, n.Permission))
		}
	}
	if len(m.Nav) > 0 && m.Panel == nil {
		errs = append(errs, "manifest.nav: a module without a panel has no navigation entries")
	}
	slices.Sort(errs)
	return errs
}
