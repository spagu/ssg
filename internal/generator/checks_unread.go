package generator

// Markdown in the source tree that no step reads (#168).
//
// A build reads exactly two directories under the source: pages/ and posts/.
// Anything else — a products/ folder an export left behind, a drafts/ folder
// someone made — is not skipped so much as never looked at, and until now
// nothing said so. A shop reported 282 product documents on disk against a log
// reading "Loaded 71 pages, 4 posts", with no output and no word: the counts
// were true, and the silence between them cost an afternoon.
//
// The reporter's own diagnosis was that the `type: product` front matter caused
// it. It does not — a document with any unknown type inside pages/ builds
// perfectly well at the address its `link:` names. The directory is the whole
// story, which is worth saying plainly, because acting on the wrong cause means
// editing 282 files for nothing.
//
// Nothing changes about what is built. This only ends the silence, and names the
// key that already solves it.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// unreadDir is one directory holding Markdown the build did not read.
type unreadDir struct {
	Path  string // relative to the source directory, slash-separated
	Files int
}

// reportUnreadContent names Markdown in the source tree that no step loads.
// Silent when there is none, which is every ordinary project.
func (g *Generator) reportUnreadContent(sourcePath string) {
	if g.config.Quiet || sourcePath == "" {
		return
	}
	dirs := g.unreadContentDirs(sourcePath)
	if len(dirs) == 0 {
		return
	}

	total := 0
	for _, d := range dirs {
		total += d.Files
	}
	fmt.Printf("   ⚠️  %d Markdown file(s) in the source tree were not read — a build reads %s/ and %s/ only:\n",
		total, g.pagesPath(), g.postsPath())
	for _, d := range dirs {
		fmt.Printf("        %s/  (%d file(s))\n", d.Path, d.Files)
	}
	fmt.Println("      Add them with content_sources:, which builds each document at the address its `link:` names:")
	fmt.Printf("        content_sources:\n          - path: %s\n            type: page\n",
		filepath.ToSlash(filepath.Join(sourcePath, dirs[0].Path)))
	fmt.Println("      A document's `type:` is not what excluded it — an unknown type inside")
	fmt.Printf("      %s/ builds normally. See docs/CONFIGURATION.md#extra-content-sources\n", g.pagesPath())
}

// unreadContentDirs finds directories under the source holding Markdown that
// neither pages/, posts/ nor any configured content source covers.
func (g *Generator) unreadContentDirs(sourcePath string) []unreadDir {
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return nil
	}
	loaded := g.loadedContentRoots(sourcePath)

	var out []unreadDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if loaded[name] {
			continue
		}
		if n := countMarkdown(filepath.Join(sourcePath, name)); n > 0 {
			out = append(out, unreadDir{Path: name, Files: n})
		}
	}
	// Deterministic order so two builds report the same way.
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// loadedContentRoots names the top-level directories the build already reads:
// pages/ and posts/, plus anything content_sources points at inside the source.
func (g *Generator) loadedContentRoots(sourcePath string) map[string]bool {
	loaded := map[string]bool{
		g.pagesPath(): true,
		g.postsPath(): true,
		// media/ is what the export downloaded, not content to render.
		"media": true,
	}
	for _, src := range g.config.ContentSources {
		rel, err := filepath.Rel(sourcePath, filepath.Clean(src.Path))
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		// Only the first segment matters: a source pointing deeper still means
		// that top-level directory is accounted for.
		if first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]; first != "" && first != "." {
			loaded[first] = true
		}
	}
	return loaded
}

// countMarkdown counts .md files anywhere under dir.
func countMarkdown(dir string) int {
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // an unreadable subtree is not worth failing a report over
		}
		if strings.EqualFold(filepath.Ext(info.Name()), ".md") {
			n++
		}
		return nil
	})
	return n
}
