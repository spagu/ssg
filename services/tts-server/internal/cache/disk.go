package cache

import (
	"io/fs"
	"os"
	"regexp"
	"slices"
)

var keyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// disk keeps one file per key inside an os.Root.
type disk struct{ root *os.Root }

func (d disk) get(key string) ([]byte, error) { return d.root.ReadFile(key) }

// put writes to a temporary name and renames, so readers never see a
// half-written file.
func (d disk) put(key string, b []byte) error {
	tmp := key + ".tmp"
	if err := d.root.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return d.root.Rename(tmp, key)
}

func (d disk) del(key string) { _ = d.root.Remove(key) }

// NewDisk returns a cache stored in root, re-indexing files left by a
// previous run (oldest first, so they are evicted first). It returns nil
// when maxBytes <= 0.
func NewDisk(root *os.Root, maxBytes int64) (*Cache, error) {
	if maxBytes <= 0 {
		return nil, nil
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var files []fs.FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && info.Mode().IsRegular() && keyPattern.MatchString(e.Name()) {
			files = append(files, info)
		}
	}
	slices.SortFunc(files, func(a, b fs.FileInfo) int { return a.ModTime().Compare(b.ModTime()) })
	c := newCache(maxBytes, disk{root: root})
	for _, f := range files {
		c.addLocked(f.Name(), f.Size())
	}
	return c, nil
}
