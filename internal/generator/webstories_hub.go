package generator

// The hub: one ordinary page per language listing its stories, rendered
// through the theme's web-stories layout when it has one and page.html
// otherwise — so it wears the site's chrome, reaches the sitemap like any
// page, and is kept out of menus and card grids with hide_from_lists.

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// addStoryHubs adds a hub page for every language that has stories and
// returns how many were added. A page the site already has at that address
// wins: a hand-made hub is the author's call.
func (g *Generator) addStoryHubs(b *storyBuild) int {
	type hub struct {
		prefix, lang string
		stories      []*webStory
	}
	var hubs []*hub
	byPrefix := map[string]*hub{}
	for _, s := range b.stories {
		h, ok := byPrefix[s.prefix]
		if !ok {
			h = &hub{prefix: s.prefix, lang: s.lang}
			byPrefix[s.prefix] = h
			hubs = append(hubs, h)
		}
		h.stories = append(h.stories, s)
	}
	added := 0
	for _, h := range hubs {
		path := normalizeStoriesURL(g.config.WebStories.URL)
		if h.prefix != "" {
			path = "/" + strings.Trim(h.prefix, "/") + path
		}
		if b.taken[path] {
			g.log(fmt.Sprintf("   ⚠️  web stories: %s is already a page — the hub was not generated there", path))
			continue
		}
		page := g.storyHubPage(path, h.lang, h.stories)
		g.siteData.Pages = append(g.siteData.Pages, page)
		g.markStoryHubPending(page)
		added++
	}
	return added
}

// storyHubPage is the hub as a site page: Extra.web_stories for a theme's
// layout, and a plain list as its content for page.html.
func (g *Generator) storyHubPage(path, lang string, stories []*webStory) models.Page {
	cards := make([]StoryCard, 0, len(stories))
	var newest time.Time
	var list strings.Builder
	list.WriteString(`<ul class="ssg-web-stories">` + "\n")
	for _, s := range stories {
		card := StoryCard{Title: s.ctx.Title, URL: s.path, PostURL: s.ctx.Post.GetURL(), Poster: s.poster}
		cards = append(cards, card)
		if s.date.After(newest) {
			newest = s.date
		}
		esc := template.HTMLEscapeString
		fmt.Fprintf(&list, `<li class="ssg-web-story"><a href="%s"><img src="%s" width="%d" height="%d" alt="" loading="lazy"><span>%s</span></a></li>`+"\n",
			esc(card.URL), esc(card.Poster.Path), card.Poster.Width, card.Poster.Height, esc(card.Title))
	}
	list.WriteString("</ul>")
	site := g.storyPublisher()
	return models.Page{
		Title: g.storyText(lang, "hub_title", nil), Link: path, Slug: strings.Trim(path, "/"),
		Status: "publish", Type: "page", Layout: storyHubLayout, Lang: lang,
		Description: g.storyText(lang, "hub_description", map[string]any{"site": site}),
		Content:     list.String(), PageFormat: g.config.PageFormat,
		Date: newest, Modified: newest,
		Extra: map[string]interface{}{"web_stories": cards, "hide_from_lists": true},
	}
}

// markStoryHubPending makes an incremental build rewrite the hub: its list
// changes with posts the dependency graph does not tie it to.
func (g *Generator) markStoryHubPending(page models.Page) {
	if g.pending == nil {
		return
	}
	for _, out := range g.getOutputPaths(page.GetOutputPath()) {
		g.pending[g.graphID(out)] = true
	}
}
