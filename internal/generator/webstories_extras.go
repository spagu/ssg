package generator

// What a Web Story takes from the post's story: settings (1.9.4), so the
// presentation on the article and the AMP story are one thing, as far as AMP
// allows (no script): the header's logo on every page, the widgets flying in
// over the photographs, the footer's promos rotating at the bottom (by CSS)
// and gathered on a page of their own before the last, today's music, and —
// from the recipe's structured data — the ingredients as a panel the reader
// pulls up. Never the method: the story stays an invitation to the article.
// A post without story: gets none of it.

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// storyNow is the build's clock for the music's date windows (a var for tests).
var storyNow = time.Now

const (
	storyFooterRotate = 4 // seconds each footer promo stays up
	storyWidgetDelay  = 1.2
	storyWidgetStep   = 2.5
	storyMaxWidgets   = 2 // per page; more would cover the photograph
)

// StoryLogo is the site's logo over every page, as wide as it needs.
type StoryLogo struct {
	URL    string
	Height int
	Alt    string
}

// StoryPromo is one link of the footer.
type StoryPromo struct {
	URL   string
	Image string
	Text  string
}

// StoryFooter is the footer's promos: rotating at the bottom of each page,
// and listed on the page before the last.
type StoryFooter struct {
	Label string
	Items []StoryPromo
}

// StoryWidget is a tip that flies in over a photograph.
type StoryWidget struct {
	Type  string
	Stat  string
	Title string
	Text  string
	Delay string // animate-in-delay, "1.2s"
}

// StoryRecipe is the ingredients panel.
type StoryRecipe struct {
	Label       string // the button and the panel's title
	Ingredients []string
}

// storyExtras fills the context from the post's story settings and recipe.
func (b *storyBuild) storyExtras(ctx *StoryContext, c storyCandidate) {
	settings, ok := b.g.storySettings(c.page, c.isPost)
	if !ok {
		return
	}
	show, _ := settings["slideshow"].(map[string]any)
	ctx.Header = b.g.storyHeader(show, ctx.Publisher)
	ctx.Footer = b.g.storyFooter(show)
	ctx.RecommendedLabel = b.g.storyText(ctx.Lang, "recommended", nil)
	ctx.Music = b.g.storyMusic(settings["audio"])
	widgets := storyWidgets(settings["widgets"])
	for i := range ctx.Pages {
		lo, hi := i*storyMaxWidgets, (i+1)*storyMaxWidgets
		if lo >= len(widgets) {
			break
		}
		if hi > len(widgets) {
			hi = len(widgets)
		}
		ctx.Pages[i].Widgets = widgets[lo:hi]
	}
	if r, ok := b.g.pageRecipe(c.page, c.isPost); ok && len(r.Ingredients) > 0 {
		ctx.Recipe = &StoryRecipe{Label: b.g.storyText(ctx.Lang, "ingredients", nil), Ingredients: r.Ingredients}
	}
	ctx.ExtraCSS = storyFooterCSS(ctx.Footer)
}

// storyHeader is slideshow.header.logo as an image over every page.
func (g *Generator) storyHeader(show map[string]any, publisher string) *StoryLogo {
	h, _ := show["header"].(map[string]any)
	logo := storyString(h["logo"])
	if logo == "" {
		return nil
	}
	height := 32
	if n, ok := h["logoHeight"].(int); ok && n > 0 {
		height = n
	}
	alt := storyString(h["text"])
	if alt == "" {
		alt = publisher
	}
	return &StoryLogo{URL: g.storyAssetURL(logo), Height: height, Alt: alt}
}

// storyFooter is slideshow.footer: one promo or `items:`.
func (g *Generator) storyFooter(show map[string]any) *StoryFooter {
	f, _ := show["footer"].(map[string]any)
	if f == nil {
		return nil
	}
	list, _ := f["items"].([]any)
	if len(list) == 0 {
		list = []any{f}
	}
	out := &StoryFooter{Label: storyString(f["label"])}
	for _, x := range list {
		it, _ := x.(map[string]any)
		link := storyString(it["link"])
		if link == "" {
			continue
		}
		p := StoryPromo{URL: g.storyAssetURL(link), Text: storyString(it["text"])}
		if img := storyString(it["image"]); img != "" {
			p.Image = g.storyAssetURL(img)
		}
		out.Items = append(out.Items, p)
	}
	if len(out.Items) == 0 {
		return nil
	}
	return out
}

// storyFooterCSS is the rotation: each of n promos visible for 1/n of the
// cycle, its turn set by nth-of-type (AMP stories take no inline style);
// visibility too, so a hidden link cannot be tapped.
func storyFooterCSS(f *StoryFooter) template.CSS {
	if f == nil {
		return ""
	}
	n := len(f.Items)
	if n < 2 {
		return ".strip .slot{opacity:1;visibility:visible;animation:none}"
	}
	on := 100.0 / float64(n)
	var b strings.Builder
	fmt.Fprintf(&b, ".strip .slot{animation-duration:%ds}@keyframes story-rot{0%%{opacity:0;visibility:visible}"+
		"%.2f%%{opacity:1}%.2f%%{opacity:1;visibility:visible}%.2f%%{opacity:0;visibility:hidden}100%%{opacity:0;visibility:hidden}}",
		n*storyFooterRotate, on*0.08, on*0.92, on)
	for k := 1; k < n; k++ {
		fmt.Fprintf(&b, ".strip .slot:nth-of-type(%d){animation-delay:%ds}", k+1, k*storyFooterRotate)
	}
	// #nosec G203 -- numbers formatted here, nothing from the site
	return template.CSS(b.String())
}

// storyWidgets are the widgets a story can show: a stat, a title or a text;
// the recipe widget is the ingredients panel instead.
func storyWidgets(v any) []StoryWidget {
	list, _ := v.([]any)
	var out []StoryWidget
	for _, x := range list {
		w, _ := x.(map[string]any)
		typ := storyString(w["type"])
		sw := StoryWidget{Type: typ, Stat: storyString(w["stat"]), Title: storyString(w["title"]), Text: storyString(w["text"])}
		if typ == "recipe" || sw.Stat+sw.Title+sw.Text == "" {
			continue
		}
		if sw.Type == "" {
			sw.Type = "card"
		}
		sw.Delay = fmt.Sprintf("%.1fs", storyWidgetDelay+float64(len(out)%storyMaxWidgets)*storyWidgetStep)
		out = append(out, sw)
	}
	return out
}

// storyMusic is today's track from story.audio: the first entry whose date
// window holds today (MM-DD, YYYY-MM-DD, or a list of `dates`), its src or
// the first of its playlist.
func (g *Generator) storyMusic(v any) string {
	list, ok := v.([]any)
	if !ok {
		list = []any{v}
	}
	now := storyNow()
	for _, x := range list {
		a, _ := x.(map[string]any)
		if a == nil || !storyToday(a, now) {
			continue
		}
		src := storyString(a["src"])
		if pl, _ := a["playlist"].([]any); src == "" && len(pl) > 0 {
			src = storyString(pl[0])
		}
		if src != "" {
			return g.storyAssetURL(src)
		}
	}
	return ""
}

// storyToday is story.js's date rule.
func storyToday(a map[string]any, now time.Time) bool {
	if dates, _ := a["dates"].([]any); len(dates) > 0 {
		for _, d := range dates {
			w, _ := d.(map[string]any)
			if storyInWindow(storyString(w["from"]), storyString(w["to"]), now) {
				return true
			}
		}
		return false
	}
	from, to := storyString(a["from"]), storyString(a["to"])
	return from == "" && to == "" || storyInWindow(from, to, now)
}

// storyInWindow: MM-DD windows wrap the year end; full dates do not.
func storyInWindow(from, to string, now time.Time) bool {
	full := len(from) > 5 || len(to) > 5
	day, lo, hi := now.Format("01-02"), "01-01", "12-31"
	if full {
		day, lo, hi = now.Format("2006-01-02"), "0000-01-01", "9999-12-31"
	}
	if from == "" {
		from = lo
	}
	if to == "" {
		to = hi
	}
	if from <= to {
		return from <= day && day <= to
	}
	return day >= from || day <= to
}

// storyAssetURL is a site path as the story spells it; URLs pass.
func (g *Generator) storyAssetURL(p string) string {
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") {
		return p
	}
	clean := "/" + models.SanitizeRelPath(p)
	if strings.HasSuffix(p, "/") && clean != "/" {
		clean += "/" // a pretty URL keeps its slash
	}
	return g.storyHref(clean)
}

// storyString is a scalar setting as text.
func storyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	}
	return fmt.Sprint(v)
}
