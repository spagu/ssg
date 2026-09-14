package generator

// Markdown the loader read and then did not publish (#274, #279).
//
// #168 named directories the build never opens, and #211 named posts sitting
// where the loader never looks. Both left one silence behind: a file inside
// pages/ or posts/ that was opened, and then dropped for what was in it.
//
// Two ways that happens, and they are not the same kind of problem:
//
//   - The frontmatter has no `status:` line. CONTENT.md says such a file is a
//     draft, and that rule stands — but a draft is something somebody chose,
//     and an omitted line is the one case nobody did. A project with ten pages
//     and no status on any of them printed "Loaded 0 pages" and nothing else.
//     A status deliberately set to something else (`draft`, `private`) stays
//     quiet: that one was a decision.
//
//   - The frontmatter is not YAML. The commonest cause is an unquoted colon in
//     a description, which reads naturally and parses as a nested mapping.
//     That file was never excluded by anyone, so it is named with the likely
//     cause, and under `strict` it fails the build: a site missing a section
//     otherwise ships with exit status 0.
//
// Nothing about what is built changes outside strict mode.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// contentSkips is what one content load read but did not publish.
type contentSkips struct {
	// unparsed is every file whose frontmatter could not be decoded, in load order.
	unparsed []unparsedFile
	// noStatus is directory → file names parsed with no status line at all.
	noStatus map[string][]string
}

// unparsedFile is one Markdown file the parser rejected.
type unparsedFile struct {
	path string
	err  error
}

// recordUnparsed notes a file whose frontmatter could not be read.
func (s *contentSkips) recordUnparsed(path string, err error) {
	s.unparsed = append(s.unparsed, unparsedFile{path: path, err: err})
}

// recordNoStatus notes a file parsed with frontmatter but no status line.
func (s *contentSkips) recordNoStatus(dir, name string) {
	if s.noStatus == nil {
		s.noStatus = map[string][]string{}
	}
	s.noStatus[dir] = append(s.noStatus[dir], name)
}

// reportSkippedContent prints both lists, and under strict fails the build on
// the unparseable files. Silent when nothing was skipped, which is every
// ordinary project.
func (g *Generator) reportSkippedContent() error {
	skips := g.skipped
	if !g.config.Quiet {
		printNoStatus(skips.noStatus)
		printUnparsed(skips.unparsed, g.config.Strict)
	}
	if g.config.Strict && len(skips.unparsed) > 0 {
		return fmt.Errorf("%d Markdown file(s) have frontmatter that could not be parsed (strict): %s",
			len(skips.unparsed), skips.unparsed[0].path)
	}
	return nil
}

// printNoStatus names the files dropped for a missing status, per directory.
func printNoStatus(byDir map[string][]string) {
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		names := append([]string(nil), byDir[dir]...)
		sort.Strings(names)
		fmt.Printf("   ⚠️  %d Markdown file(s) in %s were parsed but not published — they have no `status: publish` line: %s\n",
			len(names), filepath.ToSlash(dir), strings.Join(names, ", "))
	}
	if len(dirs) > 0 {
		fmt.Println("      A file with frontmatter is published only with `status: publish`; any other status keeps it a draft. See docs/CONTENT.md#markdown-and-frontmatter")
	}
}

// printUnparsed names each rejected file with YAML's reason and, where the
// reason has a usual cause, the cause in an author's words.
func printUnparsed(files []unparsedFile, strict bool) {
	if len(files) == 0 {
		return
	}
	fmt.Printf("   ⚠️  %d Markdown file(s) were not published — their frontmatter could not be parsed:\n", len(files))
	for _, f := range files {
		fmt.Printf("        %s: %v\n", filepath.ToSlash(f.path), f.err)
		if hint := frontmatterHint(f.err); hint != "" {
			fmt.Printf("          %s\n", hint)
		}
	}
	if !strict {
		fmt.Println("      Set `strict: true` to fail the build on this; a file that is not content belongs in content_exclude.")
	}
}

// frontmatterHints maps YAML's phrasing to what an author most likely wrote.
var frontmatterHints = []struct{ yaml, hint string }{
	{"mapping values are not allowed", `an unquoted ":" inside a value? Wrap the whole value in double quotes.`},
	{"that cannot start any", "a value starting with @, ` or another reserved character? Wrap it in double quotes."},
}

// frontmatterHint returns the likely cause of a parse error, or "".
func frontmatterHint(err error) string {
	msg := err.Error()
	for _, h := range frontmatterHints {
		if strings.Contains(msg, h.yaml) {
			return h.hint
		}
	}
	return ""
}
