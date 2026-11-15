package main

// Files every worker template's tests share (#423). A template's manifest and
// summary tests import the module reader from the directory above the
// template (`workers/module-manifest.ts`, and through it the summary kit and
// the two schemas), so a template scaffolded on its own needs them next to it.
// They are ssg's files, not the template's: written when missing, and an
// existing copy is left alone (never overwritten behind the user's back) with
// a note when it differs from this release's.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ssgroot "github.com/spagu/ssg"
)

// sharedModuleFiles are the files under workers/ that templates' tests import.
var sharedModuleFiles = []string{
	"module-manifest.ts",
	"module-summary-kit.ts",
	"module.schema.json",
	"module-summary.schema.json",
}

// writeSharedModuleFiles copies the shared module files from fsys's workers/
// into dir. It returns a line per file it wrote or found outdated, for the
// command's output.
func writeSharedModuleFiles(fsys fs.FS, dir string) ([]string, error) {
	var notes []string
	for _, name := range sharedModuleFiles {
		want, err := fs.ReadFile(fsys, "workers/"+name)
		if err != nil {
			return notes, err
		}
		target := filepath.Join(dir, name)
		// #nosec G304 -- target is a fixed file name under the validated ./workers directory
		have, err := os.ReadFile(target)
		switch {
		case err == nil && bytes.Equal(have, want):
			continue
		case err == nil:
			notes = append(notes, fmt.Sprintf("   ℹ️  %s differs from this ssg release's copy — kept as is; delete it and scaffold again to refresh it", target))
		case os.IsNotExist(err):
			// #nosec G306 -- scaffolded files are world-readable, like the template's own
			if err := os.WriteFile(target, want, 0644); err != nil {
				return notes, err
			}
			notes = append(notes, fmt.Sprintf("   + %s (shared by the templates' tests)", target))
		default:
			return notes, err
		}
	}
	return notes, nil
}

// scaffoldSharedModuleFiles writes the shared files next to the scaffolded
// template and reports what it did on stderr: stdout stays byte for byte what
// `ssg new worker` always printed, the config block people copy and scripts
// read (pinned by TestPagesTemplatesScaffoldAsBefore).
func scaffoldSharedModuleFiles(dir string) error {
	notes, err := writeSharedModuleFiles(ssgroot.EmbeddedWorkers, dir)
	for _, n := range notes {
		errf("%s\n", n)
	}
	return err
}
