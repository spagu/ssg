package generator

// Where a story's pictures come from: the featured image, then the post's
// own photographs in the order the reader meets them.

import (
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

var (
	// storyMarkdownImageRe is a Markdown image: ![alt](src "title").
	storyMarkdownImageRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)`)
	// storyHTMLImageRe is an <img> element's src.
	storyHTMLImageRe = regexp.MustCompile(`(?i)<img\b[^>]*?\ssrc\s*=\s*["']([^"']+)["']`)
	// storySizeSuffixRe is a WordPress thumbnail suffix, -300x200.jpg: the
	// original is the better picture to cut a full-screen page from.
	storySizeSuffixRe = regexp.MustCompile(`-\d+x\d+(\.[A-Za-z0-9]+)$`)
)

// storyPath is the address of a document's story: the hub, then the slug,
// under the document's language prefix.
func (g *Generator) storyPath(p models.Page) string {
	slug := slugify(path.Base(strings.Trim(p.Slug, "/")))
	if slug == "" || slug == "." {
		slug = slugify(p.Title)
	}
	base := normalizeStoriesURL(g.config.WebStories.URL)
	if p.LangPrefix != "" {
		base = "/" + strings.Trim(p.LangPrefix, "/") + base
	}
	return base + slug + "/"
}

// storyContentImages is every picture in a document's body the pipeline can
// read, in order, once each, without the featured image.
func (g *Generator) storyContentImages(p models.Page, featured string) []string {
	body := fixMediaPaths(p.Content, g.siteData.Media)
	type ref struct {
		at  int
		src string
	}
	var refs []ref
	for _, re := range []*regexp.Regexp{storyMarkdownImageRe, storyHTMLImageRe} {
		for _, m := range re.FindAllStringSubmatchIndex(body, -1) {
			refs = append(refs, ref{at: m[0], src: body[m[2]:m[3]]})
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].at < refs[j].at })
	seen := map[string]bool{featured: true}
	var out []string
	for _, r := range refs {
		src := g.storyImageSource(r.src, p)
		if src != "" && !seen[src] {
			seen[src] = true
			out = append(out, src)
		}
	}
	return out
}

// storyImageSource turns a reference as written — /media/x.jpg, a path
// beside the post, a URL on the site's own domain — into a path the image
// pipeline resolves, or "" when it names nothing this site holds.
func (g *Generator) storyImageSource(ref string, p models.Page) string {
	for _, c := range g.storyImageCandidates(ref, p) {
		if _, err := g.imageProcessor().Info(c); err == nil {
			return c
		}
	}
	return ""
}

// storyImageCandidates are the paths a reference may mean, best first.
func (g *Generator) storyImageCandidates(ref string, p models.Page) []string {
	local, ok := g.localImagePath(ref)
	if !ok {
		return nil
	}
	var paths []string
	if !strings.HasPrefix(local, "/") && p.SourceDir != "" {
		// Beside the document: relative to the content root the pipeline searches.
		root := filepath.Join(g.config.ContentDir, g.config.Source)
		if rel, err := filepath.Rel(root, p.SourceDir); err == nil && !strings.HasPrefix(rel, "..") {
			paths = append(paths, path.Join(filepath.ToSlash(rel), local))
		}
	}
	paths = append(paths, strings.TrimPrefix(path.Clean("/"+local), "/"))
	var out []string
	for _, c := range paths {
		if full := storySizeSuffixRe.ReplaceAllString(c, "$1"); full != c {
			out = append(out, full)
		}
		out = append(out, c)
	}
	return out
}

// localImagePath strips a reference to its path on this site: the query and
// fragment go, the site's own scheme and host go, base_path goes. A data: URI
// or another host's URL is not a file here.
func (g *Generator) localImagePath(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(strings.ToLower(ref), "data:") {
		return "", false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	if u.Host != "" || u.Scheme != "" {
		host := strings.TrimSuffix(hostOnly(g.config.Domain), "/")
		if host == "" || !strings.EqualFold(strings.TrimPrefix(u.Host, "www."), strings.TrimPrefix(host, "www.")) {
			return "", false
		}
	}
	local := u.Path
	if local == "" {
		return "", false
	}
	if strings.HasPrefix(local, "/") {
		if rest, ok := cutBasePath(local, g.config.BasePath); ok {
			local = rest
		}
	}
	return local, true
}
