package apisource

import (
	"path"
	"strings"
)

// Filter decides which files may be read, from the Include and Exclude
// globs of the configuration. Globs are slash paths relative to the root;
// "*" matches within one path segment and "**" matches any number of
// segments, none included.
type Filter struct {
	Include []string
	Exclude []string
}

// Allows reports whether a file may be read: it matches an include glob
// (or there are none) and no exclude glob.
func (f Filter) Allows(file string) bool {
	if len(f.Include) > 0 && !matchAny(f.Include, file) {
		return false
	}
	return !matchAny(f.Exclude, file)
}

// matchAny reports whether any glob matches the path.
func matchAny(globs []string, file string) bool {
	for _, g := range globs {
		if globMatch(g, file) {
			return true
		}
	}
	return false
}

// globMatch matches a slash path against a glob with "**" support. A
// malformed glob matches nothing.
func globMatch(glob, file string) bool {
	glob = strings.TrimPrefix(glob, "./")
	return matchSegments(strings.Split(glob, "/"), strings.Split(file, "/"))
}

// matchSegments matches path segments against glob segments.
func matchSegments(glob, file []string) bool {
	for len(glob) > 0 {
		if glob[0] == "**" {
			for i := 0; i <= len(file); i++ {
				if matchSegments(glob[1:], file[i:]) {
					return true
				}
			}
			return false
		}
		if len(file) == 0 {
			return false
		}
		if ok, err := path.Match(glob[0], file[0]); err != nil || !ok {
			return false
		}
		glob, file = glob[1:], file[1:]
	}
	return len(file) == 0
}

// Filter is the configuration's Include and Exclude globs as a Filter.
func (c Config) Filter() Filter { return Filter{Include: c.Include, Exclude: c.Exclude} }
