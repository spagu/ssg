package config

// A setting written either as a switch or as a mapping of its options:
// `heading_anchors: true` or `heading_anchors: {levels: [2, 3]}`. A mapping
// turns the feature on unless it says `enabled: false`.

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// decodeToggleYAML decodes a bool into enabled, or a mapping into F for apply.
func decodeToggleYAML[F any](value *yaml.Node, enabled *bool, apply func(F)) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(enabled)
	}
	var f F
	if err := value.Decode(&f); err != nil {
		return err
	}
	apply(f)
	return nil
}

// decodeToggleJSON is decodeToggleYAML for JSON.
func decodeToggleJSON[F any](data []byte, enabled *bool, apply func(F)) error {
	if err := json.Unmarshal(data, enabled); err == nil {
		return nil
	}
	var f F
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	apply(f)
	return nil
}

// decodeToggleTOML is decodeToggleYAML for TOML: a table goes through JSON,
// which shares the field tags.
func decodeToggleTOML[F any](key string, data interface{}, enabled *bool, apply func(F)) error {
	switch v := data.(type) {
	case bool:
		*enabled = v
		return nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return decodeToggleJSON(raw, enabled, apply)
	}
	return fmt.Errorf("%s: expected true/false or a table", key)
}

// toggleOn is the enabled value of a mapping: on unless it says otherwise.
func toggleOn(enabled *bool) bool {
	return enabled == nil || *enabled
}
