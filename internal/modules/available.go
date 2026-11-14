package modules

import (
	"io/fs"
	"path"
)

// available lists the templates under workers/ in fsys whose manifest is
// valid and whose id the site does not use, sorted by id. A template that is
// not a module (an untracked local leftover with no module.json, see
// embedded.go) is not offered.
func available(fsys fs.FS, schema *Schema, used []sourced) []Available {
	taken := map[string]bool{}
	for _, s := range used {
		taken[s.entry.ID] = true
	}
	dirs, _ := fs.ReadDir(fsys, "workers") // sorted by name; none when absent
	var out []Available
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join("workers", d.Name(), "module.json"))
		if err != nil {
			continue
		}
		m, errs := schema.ReadManifest(data)
		if len(errs) > 0 || taken[m.ID] {
			continue
		}
		out = append(out, Available{ID: m.ID, Kind: m.Kind, Title: m.Title["en"]})
	}
	return out
}
