package generator

// Writing a story: the built-in amp-story template or the theme's own
// story.html, and the AMP rules every later pass has to respect — a story
// carries no script but the AMP runtime and no style but amp-custom, so the
// injections ordinary pages receive are kept out of it.

import (
	"bytes"
	_ "embed"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

//go:embed assets/webstory.html
var builtinStoryHTML string

// builtinStoryTemplate is the amp-story document ssg ships.
var builtinStoryTemplate = template.Must(template.New("story").Parse(builtinStoryHTML))

// StoryImage is one derived picture: URL as the story writes it (absolute
// on the site's domain when there is one), Path rooted on the site.
type StoryImage struct {
	URL    string
	Path   string
	Width  int
	Height int
}

// StoryPage is one page of a story: a full-bleed picture and its caption.
type StoryPage struct {
	ID      string
	Image   StoryImage
	Alt     string
	Caption string
}

// StoryContext is what story.html renders: the built-in template's context,
// and a theme's when it defines its own.
type StoryContext struct {
	Lang          string
	Title         string
	Description   string
	Canonical     string // the story's own absolute URL
	Publisher     string
	PublisherLogo StoryImage // square, 192×192
	Poster        StoryImage // 640×853, poster-portrait-src
	Cover         StoryPage  // the featured image, 720×1280
	Pages         []StoryPage
	CTA           StoryPage // the last page; its picture is the cover's
	CTALabel      string
	CTAURL        string // the article
	Post          models.Page
	Site          *models.SiteData
}

// StoryCard is one story on the hub, for a theme's web-stories layout.
type StoryCard struct {
	Title   string
	URL     string // the story, rooted on the site
	PostURL string
	Poster  StoryImage
}

// ampHTMLRe finds an AMP document by its root element: <html ⚡> or <html amp>.
var ampHTMLRe = regexp.MustCompile(`(?i)<html\b[^>]*?\s(?:⚡|amp)(?:[\s=>/])`)

// isAMPDocument reports whether a rendered page is AMP HTML.
func isAMPDocument(s string) bool {
	head := s
	if len(head) > 4096 {
		head = head[:4096]
	}
	return ampHTMLRe.MatchString(head)
}

// finishAMP is everything an AMP page may still go through: whitespace
// (prettify or minify) and the invisible-character sanitizer. Every
// injection — SEO head, analytics, feed links, component assets, the listen
// button, math, diagrams, WebMCP — adds script or style AMP rejects.
func (g *Generator) finishAMP(s string) string {
	if g.config.PrettyHTML && !g.config.MinifyHTML {
		s = prettifyHTMLString(s)
	}
	if g.config.MinifyHTML {
		s = minifyHTMLString(s, g.config.MinifyHTMLKeepComments)
	}
	return g.sanitizeHTMLString(s)
}

// renderStory renders a story through the theme's story.html when it has
// one, the built-in template otherwise.
func (g *Generator) renderStory(ctx StoryContext) (string, error) {
	var buf bytes.Buffer
	var err error
	if g.tmpl != nil && g.tmpl.Lookup(storyTemplateName) != nil {
		err = g.tmpl.ExecuteTemplate(&buf, storyTemplateName, ctx)
	} else {
		err = builtinStoryTemplate.ExecuteTemplate(&buf, "story", ctx)
	}
	if err != nil {
		return "", err
	}
	return g.finishAMP(buf.String()), nil
}

// writeStory renders a story to <path>/index.html.
func (g *Generator) writeStory(s *webStory) error {
	out, err := g.renderStory(s.ctx)
	if err != nil {
		return err
	}
	file := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.TrimPrefix(s.path, "/")), indexHTMLName)
	if err := g.ensureWithinOutput(file); err != nil {
		return err
	}
	if err := g.ensureParent(file); err != nil {
		return err
	}
	// #nosec G306 -- web content files need to be world-readable
	return os.WriteFile(file, []byte(out), 0644)
}

// storySlotRe is the slot a theme puts in a post: <span data-ssg-story-slot>,
// optionally holding its own link text.
var storySlotRe = regexp.MustCompile(`(?is)<span\b([^>]*\sdata-ssg-story-slot\b[^>]*)>(.*?)</span>`)

// fillStorySlot links a post to its story where the theme left the slot, and
// removes the slot from every page without a story.
func (g *Generator) fillStorySlot(s string, page *models.Page) string {
	if !strings.Contains(s, "data-ssg-story-slot") {
		return s
	}
	href, lang := "", g.config.DefaultLanguage
	if page != nil {
		href = g.webStories.byPost[page.GetURL()]
		lang = g.storyLang(*page)
	}
	return storySlotRe.ReplaceAllStringFunc(s, func(m string) string {
		if href == "" {
			return ""
		}
		sub := storySlotRe.FindStringSubmatch(m)
		label := strings.TrimSpace(sub[2])
		if label == "" {
			label = template.HTMLEscapeString(g.storyText(lang, "watch", nil))
		}
		return `<span` + sub[1] + `><a class="ssg-story-link" href="` + template.HTMLEscapeString(href) + `">` + label + `</a></span>`
	})
}
