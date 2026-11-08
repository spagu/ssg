package generator

// Social preview cards, drawn at build time (1.9.0). A page shared on
// Facebook, LinkedIn, X or in a messenger shows its og:image; a page without
// a featured image used to post as bare text, or with the one site-wide
// image. Each such page now gets its own card — title, category, site name —
// drawn in pure Go (internal/ogcard), cached by everything that shapes it.
//
// The file name carries the card's key, so a changed title is a new URL: the
// platforms cache og:image by URL for weeks, and a new name is the one way
// to make them fetch the new card.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/cache"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/ogcard"
)

// OGCardOptions configures build-time cards; Enabled false turns them off.
type OGCardOptions struct {
	Enabled   bool
	Dir       string   // output directory, default "og"
	Sections  []string // posts, pages; default both
	Spec      ogcard.Spec
	CacheRoot string // "" = .ssg-cache
}

// DefaultOGCardDir is where cards go when og_cards.dir is not set.
const DefaultOGCardDir = "og"

// OGCardDir is the output directory of the cards, without slashes.
func (o OGCardOptions) OGCardDir() string {
	if d := strings.Trim(o.Dir, "/"); d != "" {
		return d
	}
	return DefaultOGCardDir
}

// ogCardStats counts what a build did, for its one summary line.
type ogCardStats struct{ made, cached, kept int }

// generateOGCards draws a card for every selected page without a featured
// image and records its URL on the page, before the pages are rendered, so
// the social tags and templates can name it.
func (g *Generator) generateOGCards() error {
	opts := g.config.OGCards
	if !opts.Enabled {
		return nil
	}
	g.log("🖼️  Drawing social cards...")
	layoutKey, err := opts.Spec.Key()
	if err != nil {
		return err
	}
	layout, err := opts.Spec.Load()
	if err != nil {
		return err
	}
	for _, w := range layout.ContrastWarnings() {
		g.log("   ⚠️  " + w)
	}
	var stats ogCardStats
	for _, pages := range g.pageSets(opts.Sections, "posts", "pages") {
		for i := range *pages {
			p := &(*pages)[i]
			if p.FeaturedImage != "" || p.IsNotFound() {
				continue
			}
			if err := g.ogCardForPage(p, layout, layoutKey, &stats); err != nil {
				return err
			}
		}
	}
	g.log(fmt.Sprintf("   🖼️  social cards: %d drawn, %d from cache, %d unchanged", stats.made, stats.cached, stats.kept))
	return nil
}

// ogCardForPage publishes one page's card: unchanged output is left as it
// is, a cached card is copied, and only a new one is drawn.
func (g *Generator) ogCardForPage(p *models.Page, layout ogcard.Layout, layoutKey string, stats *ogCardStats) error {
	card := g.ogCardOf(*p)
	key := ogCardKey(layoutKey, card)
	dir := g.config.OGCards.OGCardDir()
	name := pageFileStem(*p) + "-" + key[:10] + layout.Ext()
	outDir := filepath.Join(g.config.OutputDir, filepath.FromSlash(dir))
	outPath := filepath.Join(outDir, name)
	if err := g.ensureWithinOutput(outPath); err != nil {
		return err
	}
	p.OGCard = "/" + dir + "/" + name
	if _, err := os.Stat(outPath); err == nil {
		stats.kept++
		return nil
	}
	cacheDir := cache.Dir(g.config.OGCards.CacheRoot, "ogcards")
	cached := key + layout.Ext()
	data, err := os.ReadFile(filepath.Join(cacheDir, cached)) // #nosec G304 -- our own cache file
	if err == nil {
		stats.cached++
	} else {
		if data, err = ogcard.Render(card, layout); err != nil {
			return fmt.Errorf("drawing the card of %s: %w", p.GetURL(), err)
		}
		_ = cache.WriteAtomicBytes(cacheDir, cached, 0o644, data) // a lost cache entry only costs a redraw
		stats.made++
	}
	if err := cache.WriteAtomicBytes(outDir, name, 0o644, data); err != nil {
		return fmt.Errorf("writing the card of %s: %w", p.GetURL(), err)
	}
	return nil
}

// ogCardOf is the page's words on the card: its category above the title,
// the site's name below.
func (g *Generator) ogCardOf(p models.Page) ogcard.Card {
	eyebrow := strings.TrimSpace(p.Category)
	if eyebrow == "" && len(p.Categories) > 0 {
		eyebrow = g.siteData.Categories[p.Categories[0]].Name
	}
	return ogcard.Card{Eyebrow: eyebrow, Title: p.Title, Footer: g.ogCardFooter()}
}

// ogCardFooter is the site's name: its title, the name it gives social
// platforms, or its domain.
func (g *Generator) ogCardFooter() string {
	for _, s := range []string{g.siteData.Title, g.siteData.Marketing.OGSiteName, g.config.Domain} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// ogCardKey is the card's cache key: the layout and the words.
func ogCardKey(layoutKey string, c ogcard.Card) string {
	k := cache.NewKeyer("ogcard-page-v1", 32)
	k.WriteDelim(layoutKey)
	k.WriteDelim(c.Eyebrow)
	k.WriteDelim(c.Title)
	k.WriteString(c.Footer)
	return k.Sum()
}

// pageImage is the image a page is shared with: its own, else its card.
func pageImage(p models.Page) string {
	if p.FeaturedImage != "" {
		return p.FeaturedImage
	}
	return p.OGCard
}

// ogCardSizeMeta names the card's size, so a platform can lay out the
// preview on the first share instead of after fetching the image.
func (g *Generator) ogCardSizeMeta(p models.Page) string {
	if p.FeaturedImage != "" || p.OGCard == "" {
		return ""
	}
	s := g.config.OGCards.Spec
	return fmt.Sprintf(`<meta property="og:image:width" content="%d">`+"\n"+`<meta property="og:image:height" content="%d">`+"\n",
		orDefault(s.Width, ogcard.DefaultWidth), orDefault(s.Height, ogcard.DefaultHeight))
}

// orDefault is v when set, def otherwise.
func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
