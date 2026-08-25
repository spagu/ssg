package generator

// Posts sitting directly in posts/ (#211).
//
// loadPostsDir walks the directories under posts/ and skips everything else, so
// a Markdown file at the top level was read by nobody and reported by nothing:
// the build counted to zero and moved on. `ssg init` scaffolds its example post
// exactly there, so every fresh site began with a post its own build ignored.
//
// The folder never meant anything. A post's category comes from frontmatter;
// the directory is used only to find assets sitting beside the file. So a flat
// post is not a malformed post — it is a post in a place the loader forgot to
// look.
//
// Loading them anyway is still a behaviour change: a file with `status:
// publish` that has been silently skipped would appear on the next build, and
// publishing something nobody asked to publish is worse than the bug. Hence
// `flat_posts`, off by default — and, when it is off, a warning that names the
// files rather than a silence that costs an hour of bisecting frontmatter.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// flatPostFiles returns the Markdown files sitting directly in dir, sorted.
func flatPostFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !isMarkdownName(e.Name()) {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// isMarkdownName reports whether a filename is one the content loader reads.
func isMarkdownName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".markdown"
}

// warnSkippedFlatPosts reports files the loader is about to ignore.
//
// By name, and with both remedies, because the failure is otherwise invisible:
// pages at the top level of pages/ load fine, so a skipped post looks like a
// frontmatter problem and the reporter of #211 bisected id, link, date quoting
// and category shapes before finding it was the folder depth.
func warnSkippedFlatPosts(dir string, quiet bool) {
	names := flatPostFiles(dir)
	if len(names) == 0 || quiet {
		return
	}
	fmt.Printf("   ⚠️  %d Markdown file(s) directly in %s are not loaded: %s\n",
		len(names), filepath.ToSlash(dir), strings.Join(names, ", "))
	fmt.Printf("      A post is read from a folder under posts/ — move them into one, " +
		"or set `flat_posts: true` to load them where they are.\n")
}
