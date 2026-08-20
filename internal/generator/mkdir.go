package generator

// Creating output directories once instead of once per file.
//
// A CPU profile of a 2,000-post build was 68% syscalls, and 87% of those were
// mkdirat — about 55% of the whole build. Not because making directories is
// slow, but because every page called os.MkdirAll on its parent, and MkdirAll
// walks the path statting each component whether or not it already exists. A
// site with 2,000 posts under one tree made the same handful of directories
// two thousand times.
//
// So the generator remembers what it has already created. The saving is not the
// mkdir — it is the stat per path component that MkdirAll does first.
//
// Correctness note: this is a cache of "we created it in this run", not of
// "it exists". A directory removed by something else mid-build would not be
// recreated — which is the same exposure the build already has for every file
// it wrote earlier, and a build racing an external rm has lost anyway.

import (
	"os"
	"path/filepath"
	"sync"
)

// dirCache remembers directories this build has already created.
type dirCache struct {
	mu   sync.RWMutex
	seen map[string]bool
}

// ensureDir creates a directory and its parents, at most once per path per
// build. Rendering runs on a worker pool, so the read path is the one that has
// to stay cheap: a shared lock and a map hit, against a syscall per component.
func (g *Generator) ensureDir(path string) error {
	if path == "" || path == "." {
		return nil
	}
	if g.dirs.has(path) {
		return nil
	}
	// #nosec G301 -- web content directories need to be world-traversable
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	g.dirs.add(path)
	return nil
}

// ensureParent creates the directory a file will be written into.
func (g *Generator) ensureParent(file string) error {
	return g.ensureDir(filepath.Dir(file))
}

func (c *dirCache) has(path string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.seen[path]
}

// add records the directory and every ancestor: MkdirAll made them all, so a
// sibling written next needs no syscall either. This is where most of the
// saving comes from on a deep tree.
func (c *dirCache) add(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = make(map[string]bool, 64)
	}
	for p := path; ; {
		if c.seen[p] {
			break // this ancestor and everything above it is already recorded
		}
		c.seen[p] = true
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
}
