package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// strArg returns a string argument, trimmed. ok is false when the key is absent
// or not a string.
func strArg(args map[string]any, key string) (string, bool) {
	v, present := args[key]
	if !present {
		return "", false
	}
	s, ok := v.(string)
	return strings.TrimSpace(s), ok
}

// resolveIn resolves a user-supplied relative path against one of the allowed base
// directories (each relative to root) and refuses anything that escapes them. It
// returns the cleaned absolute path and the matched base. This is the single choke
// point that confines a section to its directories — path traversal, absolute
// paths and symlink-style escapes are all rejected here.
func resolveIn(root string, bases []string, rel string) (string, string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path must be relative to the project, got absolute %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q escapes the project", rel)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(absRoot, clean)
	for _, b := range bases {
		baseAbs := filepath.Join(absRoot, filepath.Clean(b))
		if target == baseAbs || strings.HasPrefix(target, baseAbs+string(filepath.Separator)) {
			return target, b, nil
		}
	}
	return "", "", fmt.Errorf("path %q is outside this section's directories (%s)", rel, strings.Join(bases, ", "))
}

// listFiles returns project-relative paths of every regular file under the given
// base directories (relative to root), sorted, optionally filtered by extension.
func listFiles(root string, bases []string, exts ...string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, b := range bases {
		baseAbs := filepath.Join(absRoot, filepath.Clean(b))
		_ = filepath.WalkDir(baseAbs, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a missing base dir is simply empty
			}
			if len(exts) > 0 && !hasExt(p, exts) {
				return nil
			}
			if rel, err := filepath.Rel(absRoot, p); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	sort.Strings(out)
	return out, nil
}

func hasExt(path string, exts []string) bool {
	e := strings.ToLower(filepath.Ext(path))
	for _, want := range exts {
		if e == strings.ToLower(want) {
			return true
		}
	}
	return false
}

// writeFile creates parent directories and writes content atomically enough for a
// dev workflow (truncate + write).
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- project source dir
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644) // #nosec G306 -- editable project source
}
