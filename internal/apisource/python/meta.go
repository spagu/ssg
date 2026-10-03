package python

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Detect reports whether root holds a Python package: a pyproject.toml,
// setup.py or setup.cfg, or a directory with __init__.py at the root,
// directly under it or under src/.
func Detect(root string) bool {
	for _, name := range []string{"pyproject.toml", "setup.py", "setup.cfg", "__init__.py"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	for _, pattern := range []string{"*", filepath.Join("src", "*")} {
		if m, _ := filepath.Glob(filepath.Join(root, pattern, "__init__.py")); len(m) > 0 {
			return true
		}
	}
	return false
}

// projectMeta reads the distribution name and literal version from
// pyproject.toml ([project], else [tool.poetry]), else setup.cfg
// ([metadata]). Only plain values are read: no TOML or INI library.
func projectMeta(fsys fs.FS) (name, version string) {
	if data, err := fs.ReadFile(fsys, "pyproject.toml"); err == nil {
		for _, table := range []string{"project", "tool.poetry"} {
			if v := tableValues(string(data), table, "="); v["name"] != "" {
				return v["name"], v["version"]
			}
		}
	}
	if data, err := fs.ReadFile(fsys, "setup.cfg"); err == nil {
		v := tableValues(string(data), "metadata", "=:")
		if strings.Contains(v["version"], ":") { // "attr: pkg.__version__"
			v["version"] = ""
		}
		return v["name"], v["version"]
	}
	return "", ""
}

// tableValues returns the "key = value" pairs of one [table] of a TOML or
// INI file. TOML values must be quoted strings; others are skipped. seps
// lists the characters that may separate key and value.
func tableValues(data, table, seps string) map[string]string {
	out := map[string]string{}
	toml := seps == "="
	current := ""
	for _, line := range strings.Split(data, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "["):
			current = strings.Trim(t, "[] ")
			continue
		case current != table || t == "" || t[0] == '#' || t[0] == ';':
			continue
		}
		i := strings.IndexAny(t, seps)
		if i < 0 {
			continue
		}
		key, val := strings.TrimSpace(t[:i]), strings.TrimSpace(t[i+1:])
		if toml {
			var ok bool
			if val, ok = tomlString(val); !ok {
				continue
			}
		}
		if _, dup := out[key]; !dup {
			out[key] = val
		}
	}
	return out
}

// tomlString reads a quoted TOML string at the start of val.
func tomlString(val string) (string, bool) {
	if val == "" || (val[0] != '"' && val[0] != '\'') {
		return "", false
	}
	end := strings.IndexByte(val[1:], val[0])
	if end < 0 {
		return "", false
	}
	return val[1 : end+1], true
}

// readme returns the package's README (Markdown, or reStructuredText as
// is), or "".
func readme(fsys fs.FS) string {
	for _, name := range []string{"README.md", "readme.md", "Readme.md", "README.rst", "README"} {
		if data, err := fs.ReadFile(fsys, name); err == nil {
			return string(data)
		}
	}
	return ""
}
