package generator

// Google Web Stories from posts (1.8.72). A story is an invitation to the
// article, not a copy of it: a cover with the featured image and the title,
// a few full-bleed photographs from the post captioned with short hooks — its
// tags, how long a recipe takes, what it makes, the excerpt — and a last page
// linking to the full article. Never the method: a story that teaches the
// recipe is a story nobody needs to leave.
//
// Each story is a standalone AMP document at <url><slug>/, its images cut by
// ssg's own image pipeline; a hub page at <url> lists them. Stories are not
// pages or posts: they stay out of listings, feeds and the search index, and
// reach the sitemap the way a copied document does.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// WebStoriesOptions configures web_stories:.
type WebStoriesOptions struct {
	Enabled bool
	// Sections picks the content: posts (default), pages, or a content
	// directory such as posts/recipes.
	Sections []string
	// URL is where the hub lives and the stories under it; default /web-stories/.
	URL string
	// PublisherLogo is a square image (≥96px) AMP requires on every story.
	// Without one no story is made.
	PublisherLogo string
	Pages         int    // photo pages between the cover and the last page; default 4
	MinImages     int    // images a post needs, featured included; default 2
	CTA           string // the last page's link text; default "Read the full article"
	WebP          bool   // cut WebP images (the site converts to WebP); JPEG otherwise
}

// Story dimensions: Google's poster is 3:4 at 640×853, a page 9:16.
const (
	storyPosterWidth  = 640
	storyPosterHeight = 853
	storyPageWidth    = 720
	storyPageHeight   = 1280
	storyLogoSize     = 192

	defaultStoriesURL      = "/web-stories/"
	defaultStoryPages      = 4
	defaultStoryMinImages  = 2
	defaultStoryCTA        = "Read the full article"
	storyHubLayout         = "web-stories"
	storyTemplateName      = "story.html"
	storySitemapPriority   = 0.5
	storyExcerptMaxRunes   = 140
	storyDescriptionMaxLen = 160
)

// storySet is what this build made: the stories, and post URL → story URL,
// which the slot in a post reads.
type storySet struct {
	stories []*webStory
	byPost  map[string]string
}

// webStory is one story ready to write.
type webStory struct {
	ctx    StoryContext
	path   string // site path, /web-stories/<slug>/
	prefix string // the language prefix, "" for the default language
	lang   string
	date   time.Time
	source string // the post's source file, for the sitemap's lastmod
	poster StoryImage
}

// storyCandidate is a document a story may be made from.
type storyCandidate struct {
	page   models.Page
	isPost bool
}

// withStoryDefaults fills what the site left unset.
func withStoryDefaults(o WebStoriesOptions) WebStoriesOptions {
	o.URL = normalizeStoriesURL(o.URL)
	if o.Pages <= 0 {
		o.Pages = defaultStoryPages
	}
	if o.MinImages <= 0 {
		o.MinImages = defaultStoryMinImages
	}
	return o
}

// normalizeStoriesURL makes the hub path /a/b/: rooted, slash-terminated,
// unable to climb out of the site.
func normalizeStoriesURL(u string) string {
	clean := strings.Trim(models.SanitizeRelPath(strings.TrimSpace(u)), "/")
	if clean == "" {
		return defaultStoriesURL
	}
	return "/" + clean + "/"
}

// generateWebStories makes every story, writes it, and adds the hub page to
// the site. It runs before the site renders, so a post can link its story
// and the hub renders like any other page.
func (g *Generator) generateWebStories() error {
	g.webStories = storySet{}
	if !g.config.WebStories.Enabled {
		return nil
	}
	g.log("📖 Making web stories...")
	b := newStoryBuild(g, withStoryDefaults(g.config.WebStories))
	if !b.prepareLogo() {
		return nil
	}
	for _, c := range g.storyCandidates(b.opts.Sections) {
		b.add(c)
	}
	b.reportSkips()
	sort.SliceStable(b.stories, func(i, j int) bool { return b.stories[i].date.After(b.stories[j].date) })
	for _, s := range b.stories {
		if err := g.writeStory(s); err != nil {
			return fmt.Errorf("web story %s: %w", s.path, err)
		}
		g.recordStorySitemap(s)
	}
	g.webStories = storySet{stories: b.stories, byPost: b.byPost}
	hubs := g.addStoryHubs(b)
	g.log(fmt.Sprintf("   📖 web stories: %d made, %d hub page(s)", len(b.stories), hubs))
	return nil
}

// storyCandidates is the documents sections names, each once.
func (g *Generator) storyCandidates(sections []string) []storyCandidate {
	if len(sections) == 0 {
		sections = []string{"posts"}
	}
	seen := map[string]bool{}
	var out []storyCandidate
	add := func(p models.Page, isPost bool) {
		if u := p.GetURL(); !seen[u] {
			seen[u] = true
			out = append(out, storyCandidate{page: p, isPost: isPost})
		}
	}
	for _, s := range sections {
		key := strings.Trim(strings.ToLower(strings.TrimSpace(s)), "/")
		for _, p := range g.siteData.Posts {
			if key == "posts" || key == "post" || g.inStorySection(p, key) {
				add(p, true)
			}
		}
		for _, p := range g.siteData.Pages {
			if key == "pages" || key == "page" || g.inStorySection(p, key) {
				add(p, false)
			}
		}
	}
	return out
}

// inStorySection reports whether a document sits in the content directory
// key names (posts/recipes holds posts/recipes/cake.md and deeper).
func (g *Generator) inStorySection(p models.Page, key string) bool {
	if key == "" {
		return false
	}
	section := strings.ToLower(g.contentSection(p))
	return section == key || strings.HasPrefix(section, key+"/")
}

// storyLang is the language a story speaks: its post's, else the site's.
func (g *Generator) storyLang(p models.Page) string {
	switch {
	case p.Lang != "":
		return p.Lang
	case g.config.DefaultLanguage != "":
		return g.config.DefaultLanguage
	}
	return "en"
}

// storyPublisher is the name AMP shows beside the logo.
func (g *Generator) storyPublisher() string {
	for _, s := range []string{g.siteData.Title, g.config.Title, g.config.Domain} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return "ssg"
}

// storyHref is a site path as a story spells it: absolute on the site's
// domain when there is one — a story is shared and embedded far from the
// site — else rooted, under base_path.
func (g *Generator) storyHref(p string) string {
	if g.config.Domain != "" {
		return g.servedURL(g.absoluteURL(p))
	}
	return withBasePath(p, g.config.BasePath)
}

// takenURLs is every address a page or post already renders at.
func (g *Generator) takenURLs() map[string]bool {
	taken := map[string]bool{}
	for _, set := range [][]models.Page{g.siteData.Pages, g.siteData.Posts} {
		for _, p := range set {
			taken[p.GetURL()] = true
		}
	}
	return taken
}

// recordStorySitemap lists a written story the way a copied document is
// listed: it is a page to a crawler, and belongs to no collection.
func (g *Generator) recordStorySitemap(s *webStory) {
	if g.config.Domain == "" {
		return // a sitemap names absolute URLs; without a domain there are none
	}
	rel := strings.TrimPrefix(s.path, "/") + indexHTMLName
	g.staticSitemapMu.Lock()
	defer g.staticSitemapMu.Unlock()
	g.staticSitemap = append(g.staticSitemap, staticSitemapEntry{
		loc:      g.servedURL(g.absoluteURL(s.path)),
		source:   s.source,
		priority: storySitemapPriority,
		rel:      rel,
	})
}

// storySourceFile is the file a document was read from, or "".
func storySourceFile(p models.Page) string {
	if p.SourceFile == "" {
		return ""
	}
	return filepath.Join(p.SourceDir, p.SourceFile)
}
