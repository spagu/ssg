package generator

// A self-link on each heading that has an id (#364).
//
// Every Markdown heading already has an id, so /docs/api/#overview works, but
// a reader could not get that address without developer tools. Themes added
// the links in a script after load. Writing them at build time means they work
// without JavaScript, are in the HTML crawlers read, and every theme gets them.
//
// They are added to the page body only. The table of contents, the search
// index, feeds, summaries and the audio narration all read the Markdown, not
// this HTML, so a "#" never turns up in any of them.

import (
	stdhtml "html"
	"regexp"
	"strconv"
	"strings"
)

// HeadingAnchorsOptions configures the links. The zero value adds none.
type HeadingAnchorsOptions struct {
	Enabled bool
	Levels  []int  // default 1, 2, 3
	Symbol  string // default "#"
	Class   string // default "heading-anchor"
	Label   string // aria-label, {text} is the heading; default "Link to the section {text}"
}

// headingWithIDRe is a heading element carrying an id, as the Markdown renderer
// writes it: the id is the first attribute.
var headingWithIDRe = regexp.MustCompile(`(?s)<h([1-6]) id="([^"]+)"([^>]*)>(.*?)</h([1-6])>`)

// addHeadingAnchors appends the self-link inside each heading of a configured
// level. A heading that already links to itself — a heading render hook may
// do that — is left alone.
func (g *Generator) addHeadingAnchors(s string) string {
	o := g.config.HeadingAnchors
	if !o.Enabled || !strings.Contains(s, " id=\"") {
		return s
	}
	levels := map[string]bool{}
	for _, l := range firstNonEmptyInts(o.Levels, []int{1, 2, 3}) {
		levels[strconv.Itoa(l)] = true
	}
	symbol := stdhtml.EscapeString(firstNonEmpty(o.Symbol, "#"))
	class := stdhtml.EscapeString(firstNonEmpty(o.Class, "heading-anchor"))
	label := firstNonEmpty(o.Label, "Link to the section {text}")

	return headingWithIDRe.ReplaceAllStringFunc(s, func(h string) string {
		m := headingWithIDRe.FindStringSubmatch(h)
		level, id, attrs, inner, closing := m[1], m[2], m[3], m[4], m[5]
		if level != closing || !levels[level] || strings.Contains(inner, `href="#`+id+`"`) {
			return h
		}
		text := strings.TrimSpace(stdhtml.UnescapeString(tmplStripHTML(inner)))
		aria := stdhtml.EscapeString(strings.ReplaceAll(label, "{text}", text))
		return `<h` + level + ` id="` + id + `"` + attrs + `>` + inner +
			` <a class="` + class + `" href="#` + id + `" aria-label="` + aria + `">` + symbol + `</a></h` + level + `>`
	})
}

// firstNonEmptyInts returns the first list that has anything in it.
func firstNonEmptyInts(lists ...[]int) []int {
	for _, l := range lists {
		if len(l) > 0 {
			return l
		}
	}
	return nil
}
