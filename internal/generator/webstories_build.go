package generator

// One build's stories: which documents get one, which do not and why, and
// the images each is cut from.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/images"
	"github.com/spagu/ssg/internal/models"
)

// Reasons a document gets no story, reported once per build.
const (
	storySkipNoFeatured  = "no featured image"
	storySkipNotLocal    = "featured image is not a file of this site"
	storySkipFewImages   = "too few images"
	storySkipURLTaken    = "a page already renders at the story's address"
	storySkipImageFailed = "an image could not be processed"
	storySkipListMax     = 5 // titles named per reason; the rest are counted
)

// storyBuild collects one build's stories.
type storyBuild struct {
	g       *Generator
	opts    WebStoriesOptions
	format  string // jpeg or webp
	logo    StoryImage
	taken   map[string]bool
	stories []*webStory
	byPost  map[string]string
	skips   map[string][]string
}

// newStoryBuild starts a build with the options already defaulted.
func newStoryBuild(g *Generator, opts WebStoriesOptions) *storyBuild {
	format := "jpeg"
	if opts.WebP {
		format = "webp"
	}
	return &storyBuild{g: g, opts: opts, format: format, taken: g.takenURLs(),
		byPost: map[string]string{}, skips: map[string][]string{}}
}

// prepareLogo cuts the publisher logo square. AMP rejects a story without
// one, so without it the build makes none and says so.
func (b *storyBuild) prepareLogo() bool {
	if strings.TrimSpace(b.opts.PublisherLogo) == "" {
		b.g.log("   ⚠️  web_stories: publisher_logo is not set — AMP requires a square logo (at least 96×96) on every story, so none were made")
		return false
	}
	src := b.g.storyImageSource(b.opts.PublisherLogo, models.Page{})
	if src == "" {
		b.g.log(fmt.Sprintf("   ⚠️  web_stories: publisher_logo %q is not an image of this site — no stories were made", b.opts.PublisherLogo))
		return false
	}
	logo, err := b.cut(src, storyLogoSize, storyLogoSize)
	if err != nil {
		b.g.log(fmt.Sprintf("   ⚠️  web_stories: publisher_logo: %v — no stories were made", err))
		return false
	}
	b.logo = logo
	return true
}

// add makes the story for one document, or records why it has none.
func (b *storyBuild) add(c storyCandidate) {
	p := c.page
	if strings.TrimSpace(p.FeaturedImage) == "" {
		b.skip(storySkipNoFeatured, p)
		return
	}
	featured := b.g.storyImageSource(p.FeaturedImage, p)
	if featured == "" {
		b.skip(storySkipNotLocal, p)
		return
	}
	photos := b.g.storyContentImages(p, featured)
	if 1+len(photos) < b.opts.MinImages {
		b.skip(storySkipFewImages, p)
		return
	}
	path := b.g.storyPath(p)
	if b.taken[path] {
		b.skip(storySkipURLTaken, p)
		return
	}
	s, err := b.story(c, path, featured, photos)
	if err != nil {
		b.skip(storySkipImageFailed, p)
		return
	}
	b.taken[path] = true
	b.byPost[p.GetURL()] = path
	b.stories = append(b.stories, s)
}

// story cuts the images and assembles the template context.
func (b *storyBuild) story(c storyCandidate, path, featured string, photos []string) (*webStory, error) {
	p := c.page
	poster, err := b.cut(featured, storyPosterWidth, storyPosterHeight)
	if err != nil {
		return nil, err
	}
	cover, err := b.cut(featured, storyPageWidth, storyPageHeight)
	if err != nil {
		return nil, err
	}
	lang := b.g.storyLang(p)
	captions := b.g.storyCaptions(p, c.isPost, lang)
	pages, err := b.photoPages(p, featured, photos, captions)
	if err != nil {
		return nil, err
	}
	ctx := StoryContext{
		Lang: lang, Title: p.Title, Description: b.g.storyDescription(p),
		Canonical: b.g.storyHref(path), Publisher: b.g.storyPublisher(),
		PublisherLogo: b.logo, Poster: poster,
		Cover:    StoryPage{ID: "cover", Image: cover, Alt: p.Title},
		Pages:    pages,
		CTA:      StoryPage{ID: "read-more", Image: cover, Alt: p.Title},
		CTALabel: b.g.storyCTALabel(lang), CTAURL: b.g.storyArticleURL(p),
		Post: p, Site: b.g.siteData,
	}
	return &webStory{ctx: ctx, path: path, prefix: p.LangPrefix, lang: lang,
		date: p.Date, source: storySourceFile(p), poster: poster}, nil
}

// photoPages are the pages between the cover and the last one: one per
// photograph, or one per caption when the post has more to say than
// pictures to say it over — those reuse the photographs, the featured image
// among them, in turn. Never more than the configured number.
func (b *storyBuild) photoPages(p models.Page, featured string, photos, captions []string) ([]StoryPage, error) {
	n := len(photos)
	if len(captions) > n {
		n = len(captions)
	}
	if n > b.opts.Pages {
		n = b.opts.Pages
	}
	pool := append(append([]string(nil), photos...), featured)
	pages := make([]StoryPage, 0, n)
	for i := 0; i < n; i++ {
		img, err := b.cut(pool[i%len(pool)], storyPageWidth, storyPageHeight)
		if err != nil {
			return nil, err
		}
		page := StoryPage{ID: fmt.Sprintf("page-%d", i+1), Image: img, Alt: p.Title}
		if i < len(captions) {
			page.Caption = captions[i]
		}
		pages = append(pages, page)
	}
	return pages, nil
}

// cut derives one image: cover-cropped from the centre, Lanczos-resampled,
// enlarged when the source is smaller, in the story format. WebP needs the
// optional cwebp; without it the build falls back to JPEG for the rest of the
// stories rather than losing them.
func (b *storyBuild) cut(src string, w, h int) (StoryImage, error) {
	res, err := b.resize(src, w, h)
	if err != nil && b.format == "webp" {
		b.g.log(fmt.Sprintf("   ⚠️  web_stories: WebP failed (%v) — stories use JPEG", err))
		b.format = "jpeg"
		res, err = b.resize(src, w, h)
	}
	if err != nil {
		return StoryImage{}, err
	}
	return StoryImage{URL: b.g.storyHref(res.URL), Path: res.URL, Width: res.Width, Height: res.Height}, nil
}

// resize is the pipeline call behind cut.
func (b *storyBuild) resize(src string, w, h int) (images.ImageResult, error) {
	return b.g.imageProcessor().ResizeDict(src, map[string]any{
		"width": w, "height": h, "mode": "fill", "anchor": "center",
		"upscale": true, "resample": "lanczos", "format": b.format,
	})
}

// skip records a document without a story.
func (b *storyBuild) skip(reason string, p models.Page) {
	name := p.Title
	if name == "" {
		name = p.GetURL()
	}
	b.skips[reason] = append(b.skips[reason], name)
}

// reportSkips says once, per reason, which documents got no story.
func (b *storyBuild) reportSkips() {
	reasons := make([]string, 0, len(b.skips))
	for r := range b.skips {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		names := b.skips[r]
		shown := names
		more := ""
		if len(names) > storySkipListMax {
			shown = names[:storySkipListMax]
			more = fmt.Sprintf(" and %d more", len(names)-storySkipListMax)
		}
		b.g.log(fmt.Sprintf("   ⚠️  web stories: %d skipped, %s: %s%s", len(names), r, strings.Join(shown, ", "), more))
	}
}
