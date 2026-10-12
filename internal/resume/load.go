package resume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads and checks a document. The error is for a file that cannot be
// read or is not a JSON object of the right shape; everything the document
// could do better comes back as diagnostics.
func Load(path string) (*Resume, []Diagnostic, error) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- the site's own configured file
	if err != nil {
		return nil, nil, err
	}
	r, err := Parse(data)
	if err != nil {
		return nil, nil, err
	}
	return r, Validate(r), nil
}

// Parse decodes a document. Unknown fields are not an error: the schema
// allows custom ones, and they stay in Raw.
func Parse(data []byte) (*Resume, error) {
	var r Resume
	if err := json.Unmarshal(data, &r.Raw); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("not a Schema Resume document: %w", err)
	}
	r.Source = data
	return &r, nil
}

// contactKeys are the fields of basics that reach the person directly.
var contactKeys = []string{"email", "phone"}

// addressKeys are the parts of basics.location that find their door; the
// city, region and country stay.
var addressKeys = []string{"address", "streetAddress", "postalCode"}

// WithoutContact is the document with the person's e-mail, phone and street
// address taken out — of the typed fields, of Raw and of Source alike — so
// nothing published from it reaches them. The receiver is not changed.
func (r *Resume) WithoutContact() (*Resume, error) {
	raw := cloneMap(r.Raw)
	basics, _ := raw["basics"].(map[string]interface{})
	removed := deleteKeys(basics, contactKeys)
	if loc, ok := basics["location"].(map[string]interface{}); ok {
		removed = deleteKeys(loc, addressKeys) || removed
	}
	if !removed {
		return r, nil // nothing to take out: the bytes stay as written
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(raw); err != nil {
		return nil, err
	}
	return Parse(buf.Bytes())
}

// deleteKeys removes keys from m and reports whether any was there.
func deleteKeys(m map[string]interface{}, keys []string) bool {
	found := false
	for _, k := range keys {
		if _, ok := m[k]; ok {
			delete(m, k)
			found = true
		}
	}
	return found
}

// cloneMap deep-copies decoded JSON.
func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return cloneMap(t)
	case []interface{}:
		s := make([]interface{}, len(t))
		for i, e := range t {
			s[i] = cloneValue(e)
		}
		return s
	default:
		return v
	}
}
