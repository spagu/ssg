package images

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolve locates a template-supplied image path inside the configured source
// roots, in order. Security: the path is cleaned, may not traverse upwards, and
// the resolved file (symlinks followed) must stay inside its root — so neither
// `../../etc/passwd` nor a symlink escape can read arbitrary files.
func (p *Processor) resolve(source string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(source))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("image path %q must be relative to a configured source directory", source)
	}
	for _, root := range p.sourceDirs {
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, clean)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		ok, err := withinRoot(root, candidate)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("image path %q escapes the source directory %q", source, root)
		}
		return candidate, nil
	}
	return "", fmt.Errorf("source image %q not found in any configured source directory", source)
}

// withinRoot reports whether path (with symlinks resolved) stays inside root.
func withinRoot(root, path string) (bool, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}
