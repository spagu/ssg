package generator

// Link checking inside stylesheets (#286).
//
// check_links read HTML only, so a stylesheet's own references were never
// followed. Rename a self-hosted font and every page still built green, then
// rendered in the fallback face: a missing @font-face source is not an error in
// any browser, it is a font that quietly does not apply. The same gap covered
// background images, masks and @import — the one class of broken reference a
// site that self-hosts its assets has, and the one the checker could not see.
//
// A stylesheet's references resolve against the stylesheet, not against the
// page that links it, which is what CSS itself does.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	// cssURLRe matches url(...) with a double-quoted, single-quoted or bare target.
	cssURLRe = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)"'\s]*))\s*\)`)
	// cssImportRe matches the string form of @import; @import url(...) is cssURLRe's.
	cssImportRe = regexp.MustCompile(`(?i)@import\s+(?:"([^"]*)"|'([^']*)')`)
	// cssCommentRe matches a comment, which may hold a reference nobody uses.
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// checkCSSLinks reports internal url() and @import targets, in every stylesheet
// of the output, that do not resolve to a file. from is "path:line".
func (g *Generator) checkCSSLinks() ([]brokenLink, error) {
	root := g.config.OutputDir
	var broken []brokenLink
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".css") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- CLI reads its own output
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, ref := range cssRefs(string(data)) {
			target := g.stripOwnDomain(ref.target)
			if !isInternalRef(target) || g.refResolves(target, filepath.Dir(path)) {
				continue
			}
			broken = append(broken, brokenLink{
				from: filepath.ToSlash(rel) + ":" + strconv.Itoa(ref.line),
				href: target,
			})
		}
		return nil
	})
	return broken, err
}

// cssRef is one reference in a stylesheet, with the line it is on.
type cssRef struct {
	target string
	line   int
}

// cssRefs extracts url() and @import targets in source order. Comments are
// blanked first, keeping their newlines so line numbers still match the file.
func cssRefs(css string) []cssRef {
	css = cssCommentRe.ReplaceAllStringFunc(css, func(c string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return ' '
		}, c)
	})
	var refs []cssRef
	for _, re := range []*regexp.Regexp{cssURLRe, cssImportRe} {
		for _, m := range re.FindAllStringSubmatchIndex(css, -1) {
			target := ""
			for i := 2; i+1 < len(m); i += 2 {
				if m[i] >= 0 {
					target = css[m[i]:m[i+1]]
					break
				}
			}
			if strings.TrimSpace(target) == "" {
				continue
			}
			refs = append(refs, cssRef{target: target, line: 1 + strings.Count(css[:m[0]], "\n")})
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].line < refs[j].line })
	return refs
}
