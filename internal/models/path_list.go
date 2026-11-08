package models

// PathList is a config value written as one string or as a list of them:
// `prefix: /openbsd/` and `prefix: [/openbsd/, /netbsd/]` mean the same kind of
// thing, and making the reader of a one-item list write brackets helps nobody
// (#386).

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// PathList holds URL path prefixes, as written.
type PathList []string

// UnmarshalYAML accepts a scalar or a sequence.
func (l *PathList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*l = PathList{value.Value}
		return nil
	}
	var items []string
	if err := value.Decode(&items); err != nil {
		return err
	}
	*l = items
	return nil
}

// UnmarshalJSON accepts a string or an array of strings.
func (l *PathList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*l = PathList{one}
		return nil
	}
	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	*l = items
	return nil
}

// UnmarshalTOML accepts a string or an array of strings.
func (l *PathList) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case string:
		*l = PathList{v}
		return nil
	case []interface{}:
		items := make(PathList, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("prefix: expected a string, got %T", item)
			}
			items = append(items, s)
		}
		*l = items
		return nil
	}
	return fmt.Errorf("prefix: expected a string or a list of strings, got %T", data)
}

// MatchesPath reports whether a site path ("/openbsd/foo/") lies under any of
// the prefixes, compared whole segment by whole segment, so /net/ does not
// claim /netbsd/x/. An empty list matches nothing: the caller decides whether
// "no prefix given" means everything.
func (l PathList) MatchesPath(p string) bool {
	p = "/" + strings.Trim(p, "/")
	for _, prefix := range l {
		if strings.TrimSpace(prefix) == "" {
			continue // a blank entry is a gap in the list, not "/"
		}
		want := path.Clean("/" + strings.Trim(strings.TrimSpace(prefix), "/"))
		if want == "/" || p == want || strings.HasPrefix(p, want+"/") {
			return true
		}
	}
	return false
}

// Set reports whether any non-blank prefix was given.
func (l PathList) Set() bool {
	for _, prefix := range l {
		if strings.TrimSpace(prefix) != "" {
			return true
		}
	}
	return false
}
