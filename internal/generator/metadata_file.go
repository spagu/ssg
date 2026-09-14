package generator

// metadata.json and the source directory it lives in (#277).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spagu/ssg/internal/models"
)

// readMetadata decodes metadata.json, treating an absent file as empty (#277).
//
// A hand-made site has no categories, authors or exported media, and so no
// reason to carry the file — yet its first build failed with a bare "open …:
// no such file or directory" that named neither the requirement nor the fix.
// An empty set is what such a site means, so that is what it gets, with one
// line saying so: a migrated site that lost the file still hears about it,
// because its category and author ids stop resolving.
//
// A missing source directory is not this case and still fails, in
// sourceDirError: that is a mistyped `source:`, and building an empty site
// from it would be the silence this avoids.
func (g *Generator) readMetadata(path string) (models.Metadata, error) {
	var metadata models.Metadata
	file, err := os.Open(path) // #nosec G304 -- CLI tool reads user's content files
	if errors.Is(err, fs.ErrNotExist) {
		g.log(fmt.Sprintf("   ⚠️  %s not found — building with no categories, authors, tags or media. "+
			`Write {"categories":[],"users":[],"tags":[],"media":[]} there to say so (docs/CONTENT.md#metadatajson).`,
			filepath.ToSlash(path)))
		return metadata, nil
	}
	if err != nil {
		return metadata, err
	}
	defer func() { _ = file.Close() }()
	if err := json.NewDecoder(file).Decode(&metadata); err != nil {
		return metadata, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	return metadata, nil
}

// sourceDirError reports a `source:` that names no directory, in words that say
// which key to check.
func sourceDirError(sourcePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("content source %s does not exist — check `source:` and `content_dir:`", filepath.ToSlash(sourcePath))
	}
	if !info.IsDir() {
		return fmt.Errorf("content source %s is not a directory — check `source:` and `content_dir:`", filepath.ToSlash(sourcePath))
	}
	return nil
}
