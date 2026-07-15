// Embedded starter-theme scaffolding (DOC-013): when the requested theme has
// no local template files but ships inside the binary (templates/simple,
// templates/krowy), the whole theme tree — HTML, CSS, JS, images — is
// extracted into the templates directory instead of the generic fallback.
package generator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ssgroot "github.com/spagu/ssg"
)

// scaffoldEmbeddedTheme extracts the named bundled theme into templatePath.
// It reports false (and no error) when the theme is not embedded, so the
// caller can fall back to the generic scaffold.
func scaffoldEmbeddedTheme(theme, templatePath string) (bool, error) {
	root := "templates/" + strings.ToLower(strings.TrimSpace(theme))
	if entries, err := ssgroot.EmbeddedThemes.ReadDir(root); err != nil || len(entries) == 0 {
		return false, nil
	}
	err := fs.WalkDir(ssgroot.EmbeddedThemes, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(templatePath, rel)
		if d.IsDir() {
			// #nosec G301 -- Web content directories need to be world-traversable
			return os.MkdirAll(target, 0755)
		}
		data, err := ssgroot.EmbeddedThemes.ReadFile(path)
		if err != nil {
			return err
		}
		// #nosec G306 -- Web content files need to be world-readable
		return os.WriteFile(target, data, 0644)
	})
	if err != nil {
		return false, fmt.Errorf("extracting embedded theme %s: %w", theme, err)
	}
	return true, nil
}
