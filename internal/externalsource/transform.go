package externalsource

import (
	"fmt"
	"strings"
)

// applyTransform runs the shared post-parse transformation layer. Phase 1
// implements `select:` — a dot path into nested maps (e.g. "data.items").
// Deliberately no scripting, eval or arbitrary code (plan §Transformacje).
func applyTransform(data interface{}, t TransformConfig) (interface{}, error) {
	if t.Select == "" {
		return data, nil
	}
	current := data
	for _, key := range strings.Split(t.Select, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("select %q: segment %q reached a non-object value (%T)", t.Select, key, current)
		}
		current, ok = m[key]
		if !ok {
			return nil, fmt.Errorf("select %q: key %q not found", t.Select, key)
		}
	}
	return current, nil
}
