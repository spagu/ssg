package generator

// Sites served under a path (#306): a GitHub project page lives at
// https://user.github.io/<repo>/, so a link to /docs/ leaves the site. Every
// root-relative URL the build writes — from Markdown, templates, autolinks,
// rewritten .md links, the search index — gets the base path in front.
//
// It is one pass over the finished output rather than a change in every place
// a URL is made: there are dozens of those, templates among them, and a theme
// writing href="/css/style.css" would otherwise need to know about the base
// path too. The pass runs after fingerprinting, so hashed names are already in
// place, and before the checks, so check_links sees what will be served.

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
)

// withBasePath prefixes a root-relative URL with base. Anything else — a
// relative URL, a protocol-relative or absolute one, a fragment — and a URL
// already under base is returned unchanged, so the pass is safe to run twice.
func withBasePath(u, base string) string {
	if base == "" || !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") || underBasePath(u, base) {
		return u
	}
	return base + u
}

// underBasePath reports whether a root-relative URL already starts with base
// as a whole path segment: /docs-site/x is under /docs-site, /docs-sitemap is not.
func underBasePath(u, base string) bool {
	rest, ok := strings.CutPrefix(u, base)
	return ok && (rest == "" || strings.ContainsAny(rest[:1], "/?#"))
}

// cutBasePath strips base from a root-relative URL, reporting whether it was
// under base at all. The root of the site is "/".
func cutBasePath(u, base string) (string, bool) {
	if base == "" {
		return u, true
	}
	if !underBasePath(u, base) {
		return u, false
	}
	rest := strings.TrimPrefix(u, base)
	if rest == "" || rest[0] != '/' {
		rest = "/" + rest
	}
	return rest, true
}

var (
	// baseAttrRe finds URL-valued attributes, quoted or not (a minifier may
	// drop the quotes). Group 1 is everything up to the value, group 2 the
	// quote, group 3 the value.
	baseAttrRe = regexp.MustCompile(`(?i)(\s(?:href|src|action|formaction|poster|data-src)\s*=\s*)(["']?)([^"'\s>]*)`)
	// baseSrcsetRe finds srcset lists, whose every candidate is a URL.
	baseSrcsetRe = regexp.MustCompile(`(?i)(\s(?:srcset|data-srcset)\s*=\s*)(["'])([^"']*)`)
	// baseRefreshRe finds the target of a meta refresh (alias stub pages).
	baseRefreshRe = regexp.MustCompile(`(?i)(content\s*=\s*["']\s*\d+\s*;\s*url\s*=\s*)(/[^"'\s>]*)`)
	// baseCSSURLRe finds url(/...) in stylesheets and style attributes.
	baseCSSURLRe = regexp.MustCompile(`(url\(\s*["']?)(/[^)"'\s]*)`)
)

var (
	// baseTagRe finds start tags. Attributes are rewritten only inside them, so
	// text in a code block that happens to read ` src=/usr/bin` is left alone.
	baseTagRe = regexp.MustCompile(`<[a-zA-Z][^<>]*>`)
	// baseStyleRe finds <style> blocks, whose url() references are URLs too.
	baseStyleRe = regexp.MustCompile(`(?is)(<style\b[^>]*>)(.*?)(</style>)`)
)

// basePathHTML prefixes the root-relative URLs of an HTML document: in tag
// attributes (href, src, srcset, a meta refresh target, a style attribute's
// url()) and in <style> blocks.
func basePathHTML(s, base string) string {
	s = baseTagRe.ReplaceAllStringFunc(s, func(tag string) string {
		return basePathTag(tag, base)
	})
	return baseStyleRe.ReplaceAllStringFunc(s, func(m string) string {
		p := baseStyleRe.FindStringSubmatch(m)
		return p[1] + basePathCSS(p[2], base) + p[3]
	})
}

// basePathTag prefixes the URL-valued attributes of one start tag.
func basePathTag(tag, base string) string {
	tag = baseAttrRe.ReplaceAllStringFunc(tag, func(m string) string {
		p := baseAttrRe.FindStringSubmatch(m)
		return p[1] + p[2] + withBasePath(p[3], base)
	})
	tag = baseSrcsetRe.ReplaceAllStringFunc(tag, func(m string) string {
		p := baseSrcsetRe.FindStringSubmatch(m)
		return p[1] + p[2] + basePathSrcset(p[3], base)
	})
	tag = baseRefreshRe.ReplaceAllStringFunc(tag, func(m string) string {
		p := baseRefreshRe.FindStringSubmatch(m)
		return p[1] + withBasePath(p[2], base)
	})
	return basePathCSS(tag, base)
}

// basePathSrcset prefixes each candidate URL of a srcset list.
func basePathSrcset(list, base string) string {
	parts := strings.Split(list, ",")
	for i, part := range parts {
		lead := part[:len(part)-len(strings.TrimLeft(part, " \t\n"))]
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		fields[0] = withBasePath(fields[0], base)
		parts[i] = lead + strings.Join(fields, " ")
	}
	return strings.Join(parts, ",")
}

// basePathCSS prefixes root-relative url() references.
func basePathCSS(s, base string) string {
	return baseCSSURLRe.ReplaceAllStringFunc(s, func(m string) string {
		p := baseCSSURLRe.FindStringSubmatch(m)
		return p[1] + withBasePath(p[2], base)
	})
}

// applyBasePath rewrites the finished output for a site served under a path.
// Files are read and written through an os.Root on the output, so a symlink in
// it is never followed (see G122 in 1.8.63).
func (g *Generator) applyBasePath() error {
	base := g.config.BasePath
	if base == "" {
		return nil
	}
	root, err := os.OpenRoot(g.config.OutputDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		var rewrite func(string, string) string
		switch strings.ToLower(path.Ext(rel)) {
		case ".html", ".htm":
			rewrite = basePathHTML
		case ".css":
			rewrite = basePathCSS
		default:
			return nil
		}
		data, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		out := rewrite(string(data), base)
		if out == string(data) {
			return nil
		}
		// #nosec G306 -- web content must be world-readable
		return root.WriteFile(rel, []byte(out), 0644)
	})
}
