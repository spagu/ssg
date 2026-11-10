package mcp

// How designer_config_set turns a string argument into a value and writes it
// into the config file without losing what the author wrote around it.

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spagu/ssg/internal/config"
)

// typedConfigValue converts the string argument to the key's declared type, so
// the written YAML holds a real bool/int rather than a quoted string.
func typedConfigValue(kind, raw string) (interface{}, error) {
	switch kind {
	case kindBool, kindSwitch:
		return strconv.ParseBool(raw)
	case kindInt:
		return strconv.Atoi(raw)
	case kindList:
		list := []string{}
		for _, item := range strings.Split(raw, ",") {
			if item = strings.TrimSpace(item); item != "" {
				list = append(list, item)
			}
		}
		return list, nil
	default:
		return raw, nil
	}
}

// writeConfigKey writes one allowed key the way its kind asks.
//
// A switch whose key holds a mapping gets `<key>.enabled`, so the feature's
// other settings survive; one written as `key: true` stays a scalar. Any other
// kind refuses to replace a mapping (`outputs:` per content type) with a scalar
// or a list, which would silently drop what the author wrote.
func writeConfigKey(src []byte, key, kind string, value interface{}) ([]byte, error) {
	existing := configNodeKind(src, key)
	switch {
	case kind == kindSwitch && existing != yaml.ScalarNode:
		return config.SetYAMLPath(src, key+".enabled", value)
	case kind != kindSwitch && existing == yaml.MappingNode:
		return nil, fmt.Errorf("%s is a mapping in the file; edit that shape in the file itself", key)
	case strings.Contains(key, "."):
		return config.SetYAMLPath(src, key, value)
	}
	return config.SetYAMLKey(src, key, value)
}

// configNodeKind is the kind of the value at a dotted key, or 0 when the key
// (or the file's mapping) is absent.
func configNodeKind(src []byte, key string) yaml.Kind {
	var doc yaml.Node
	if yaml.Unmarshal(src, &doc) != nil || len(doc.Content) == 0 {
		return 0
	}
	node := doc.Content[0]
	for _, seg := range strings.Split(key, ".") {
		if node.Kind != yaml.MappingNode {
			return 0
		}
		var next *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == seg {
				next = node.Content[i+1]
			}
		}
		if next == nil {
			return 0
		}
		node = next
	}
	return node.Kind
}
