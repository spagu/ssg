package generator

// A worker's public/ files are copied into the output after everything else
// the build writes — pages, assets, static files, feeds, sitemaps, host
// files. Until #420 only a clash between two workers was caught: a site page
// at /core/ was silently replaced by the core worker's /core/index.html. Now
// a worker's file meeting ANY file this build already wrote stops the build
// and names both, like two documents at one address do (#346): only the
// site's owner can say which one the site should serve. A byte-identical
// file is no clash — nothing would be lost.
//
// "Already wrote" is this build, not an earlier one: without `clean`, the
// output still holds the worker's own copy from the last build (an older
// template, or core's generated theme.css). snapshotWorkerPublic records,
// before the build writes anything, the size and modification time of every
// output file a worker's public/ is about to land on; a file still exactly as
// recorded was left there and is replaced as before.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// stamp identifies a file's state well enough to tell whether it was written
// since: any write changes the modification time.
type stamp struct {
	size int64
	mod  time.Time
}

// workerPublicDir is where a worker's public/ files come from, before any
// fetch: a remote source not fetched yet has none to record.
func workerPublicDir(w WorkerConfig) string {
	dir := w.Dir
	if dir == "" && w.Source != "" && w.Name != "" {
		dir = filepath.Join("workers", sanitizeWorkerName(w.Name))
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "public")
}

// snapshotWorkerPublic records the output files the workers' public/ trees
// will be copied over, as they are before this build writes anything.
func (g *Generator) snapshotWorkerPublic() {
	g.publicBefore = map[string]stamp{}
	for _, w := range g.config.ResolvedWorkers() {
		src := workerPublicDir(w)
		if src == "" {
			continue
		}
		_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil // a missing public/ has nothing to record
			}
			rel, _ := filepath.Rel(src, path) // WalkDir yields paths below src
			if fi, err := os.Stat(filepath.Join(g.config.OutputDir, rel)); err == nil {
				g.publicBefore[filepath.ToSlash(rel)] = stamp{fi.Size(), fi.ModTime()}
			}
			return nil
		})
	}
}

// copyWorkerPublic copies a worker's public/ tree (its client-side assets, e.g.
// a consent banner's js/css) into the output root, so they are served as static
// files. A no-op when the worker has no public/. Two workers shipping the same
// asset path, or a worker's file landing on one the build already wrote, is a
// hard error, never a silent overwrite.
func (g *Generator) copyWorkerPublic(dir, label string, seen map[string]string) error {
	src := filepath.Join(dir, "public")
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return nil
	}
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path) // Walk yields paths below src
		key := filepath.ToSlash(rel)
		if other, clash := seen[key]; clash {
			return fmt.Errorf("worker %s and %s both provide public asset %s", label, other, key)
		}
		seen[key] = label
		if owner := g.outputOwner(key, path); owner != "" {
			return fmt.Errorf("output collision at /%s: worker %s (public/%s) and %s are both written there, and one would silently replace the other — "+
				"give the page a distinct link: or slug:, move or rename the file, or leave the worker out; the build will not choose which one the site serves",
				key, label, key, owner)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := g.copyDir(src, g.config.OutputDir); err != nil {
		return fmt.Errorf("worker %s: copying public assets: %w", label, err)
	}
	return nil
}

// outputOwner names what this build already wrote at rel, where the worker's
// file src is about to go, or returns "" when nothing did or the two files
// are identical.
func (g *Generator) outputOwner(rel, src string) string {
	dst := filepath.Join(g.config.OutputDir, filepath.FromSlash(rel))
	fi, err := os.Stat(dst)
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return fmt.Sprintf("the directory /%s/ this build wrote", rel)
	}
	if same(src, dst) {
		return ""
	}
	if page := g.pageAt(rel); page != "" {
		return page
	}
	if before, ok := g.publicBefore[rel]; ok && before.size == fi.Size() && before.mod.Equal(fi.ModTime()) {
		return "" // left over from an earlier build; this one has not touched it
	}
	staticDir := g.config.StaticDir
	if staticDir == "" {
		staticDir = defaultStaticDir
	}
	if sfi, err := os.Stat(filepath.Join(staticDir, filepath.FromSlash(rel))); err == nil && sfi.Mode().IsRegular() {
		return fmt.Sprintf("the static file %s", filepath.ToSlash(filepath.Join(staticDir, filepath.FromSlash(rel))))
	}
	return "the file the build already wrote there (a rendered page, a theme asset, a static source or a generated file such as a feed or sitemap)"
}

// pageAt names the page or post whose index.html is rel, or "".
func (g *Generator) pageAt(rel string) string {
	if g.siteData == nil {
		return ""
	}
	for _, list := range [][]models.Page{g.siteData.Pages, g.siteData.Posts} {
		for _, p := range list {
			out := p.GetOutputPath()
			html := indexHTMLName
			if !isRootOutputPath(out) {
				html = out + "/" + indexHTMLName
			}
			if html == rel {
				return documentLabel(p)
			}
		}
	}
	return ""
}

// same reports whether two files have identical bytes.
func same(a, b string) bool {
	x, err := os.ReadFile(a) // #nosec G304 -- a worker file from the project's own config
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b) // #nosec G304 -- a file inside the build's own output
	return err == nil && bytes.Equal(x, y)
}
