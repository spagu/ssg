package webp

// Offering AVIF to browsers that take it, without breaking the ones that do not
// (#178).
//
// The WebP pass leaves `<img src="x.webp">`. A <source> cannot be added to an
// <img>, so the tag is wrapped in a <picture>: AVIF first because it is both
// smaller and better, WebP second, and the <img> stays exactly as it was as the
// last resort. A browser that understands neither source sees the img it always
// saw — which is why this is a wrap rather than a rewrite.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// alreadyPictured matches an <img> that is already inside a <picture>, so a
// second pass — a rebuild over an existing output tree — does not nest one
// inside another.
var alreadyPictured = regexp.MustCompile(`(?is)<picture\b[^>]*>.*?</picture>`)

// EmitPicture wraps every <img> whose .webp has an .avif sibling in a <picture>
// offering the AVIF first. Files with no AVIF are left untouched.
func EmitPicture(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return err
		}
		content, err := os.ReadFile(path) // #nosec G304 -- CLI reads its own output
		if err != nil {
			return err
		}
		out := wrapImagesInPicture(string(content), dir, path)
		if out == string(content) {
			return nil
		}
		// #nosec G306 -- web content must be world-readable
		return os.WriteFile(path, []byte(out), 0644)
	})
}

// wrapImagesInPicture is the pure transformation, so the rule is testable
// without a filesystem.
func wrapImagesInPicture(html, root, htmlPath string) string {
	// Protect what is already a <picture>: a rebuild must be idempotent.
	protected := alreadyPictured.FindAllStringIndex(html, -1)
	inside := func(i int) bool {
		for _, r := range protected {
			if i >= r[0] && i < r[1] {
				return true
			}
		}
		return false
	}

	return replaceAllIndexed(html, imgTagRe, func(at int, tag string) string {
		if inside(at) {
			return tag
		}
		m := imgSrcRe.FindStringSubmatch(tag)
		if m == nil {
			return tag
		}
		webpSrc := m[1]
		avifSrc := strings.TrimSuffix(webpSrc, ".webp") + ".avif"
		if !avifExistsFor(root, htmlPath, avifSrc) {
			return tag
		}
		// srcset carries the responsive widths the WebP pass emitted; the AVIF
		// pass made the same widths, so the same list applies with the
		// extension swapped.
		srcset := avifSrcsetFrom(tag)
		source := `<source type="image/avif" ` + srcset + `src="` + avifSrc + `">`
		return "<picture>" + source + tag + "</picture>"
	})
}

// avifSrcsetFrom converts an <img>'s webp srcset into the avif one, returning
// a ready `srcset="..." sizes="..." ` fragment, or "" when the tag has none.
func avifSrcsetFrom(tag string) string {
	m := srcsetAttrRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	avif := strings.ReplaceAll(m[1], ".webp", ".avif")
	out := `srcset="` + avif + `" `
	if s := sizesAttrRe.FindStringSubmatch(tag); s != nil {
		out += `sizes="` + s[1] + `" `
	}
	return out
}

var (
	srcsetAttrRe = regexp.MustCompile(`(?:^|[\s"'>])srcset\s*=\s*"([^"]+)"`)
	sizesAttrRe  = regexp.MustCompile(`(?:^|[\s"'>])sizes\s*=\s*"([^"]+)"`)
)

// avifExistsFor resolves a src as written in a page and reports whether the
// file is on disk. Both root-relative and page-relative forms occur, and a
// missing file must leave the tag alone rather than promise a 404.
func avifExistsFor(root, htmlPath, src string) bool {
	if strings.Contains(src, "://") {
		return false // a remote image is not ours to offer
	}
	var candidate string
	if strings.HasPrefix(src, "/") {
		candidate = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(src, "/")))
	} else {
		candidate = filepath.Join(filepath.Dir(htmlPath), filepath.FromSlash(src))
	}
	return fileExists(candidate)
}

// replaceAllIndexed is regexp.ReplaceAllStringFunc with the match offset, which
// the caller needs to tell an <img> already inside a <picture> from one that is
// not.
func replaceAllIndexed(s string, re *regexp.Regexp, fn func(at int, match string) string) string {
	locs := re.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(locs)*64)
	last := 0
	for _, loc := range locs {
		b.WriteString(s[last:loc[0]])
		b.WriteString(fn(loc[0], s[loc[0]:loc[1]]))
		last = loc[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// AVIFSummary is the one line a build prints about the pass.
func AVIFSummary(made int) string {
	return fmt.Sprintf("   🖼️  Wrote %d AVIF file(s) — offered before WebP via <picture>", made)
}
