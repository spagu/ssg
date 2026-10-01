package generator

// The two ways an article can be listened to (1.8.65):
//
//   - an MP3 made at build time by a TTS API (audio.go) — the same voice for
//     every visitor, playable offline and in a podcast app;
//   - a "Listen" button that reads the page with the visitor's own browser
//     voice (Web Speech API) — free, nothing to host, nothing leaves the device,
//     and the voice is whatever the device has.
//
// {{ listen .Page }} renders whichever applies: a compact "Listen" button that
// plays the page's MP3 when it has one and reads it with the browser voice when
// it does not (or the full <audio> player with listen.player: full), else
// nothing. Without the function, ssg puts the same block into the theme's
// <span data-ssg-listen-slot></span> — next to the reading time in the bundled
// themes — or after the first </h1>, unless listen.auto is false. A slot that
// gets nothing is removed, so a site with the feature off is unchanged.

import (
	"fmt"
	stdhtml "html"
	"html/template"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// ListenOptions configures the browser read-aloud button.
type ListenOptions struct {
	Enabled  bool
	Label    string   // default "Listen"
	Voice    string   // preferred browser voice, matched by name ("Natural", "Google UK English Female")
	NoAuto   bool     // do not place it on pages whose theme did not
	Sections []string // posts (default), pages
	// FullPlayer renders an MP3 as the browser's <audio controls> bar instead
	// of the compact button (listen.player: full).
	FullPlayer bool
}

// listenMarker is the attribute every block carries, so the auto placement
// and the script injection can tell the theme already placed one. It is
// always followed by a space in the markup, which tells it apart from
// listenSlot.
const listenMarker = "data-ssg-listen"

// listenSlot is the empty element a theme leaves where the block belongs.
const listenSlot = `<span data-ssg-listen-slot></span>`

// listenIcon is a speaker, decorative: the label says what the button does.
const listenIcon = `<svg class="ssg-listen-icon" aria-hidden="true" focusable="false" width="16" height="16" viewBox="0 0 24 24">` +
	`<path fill="currentColor" d="M3 9v6h4l5 5V4L7 9H3zm13.5 3a4.5 4.5 0 0 0-2.5-4.03v8.05A4.5 4.5 0 0 0 16.5 12zM14 3.23v2.06a7 7 0 0 1 0 13.42v2.06a9 9 0 0 0 0-17.54z"/></svg>`

// listenBlock is the HTML {{ listen .Page }} renders for a page.
func (g *Generator) listenBlock(p models.Page) template.HTML {
	if p.AudioURL != "" && g.config.Listen.FullPlayer {
		// #nosec G203 -- the URL is built by publishAudio from a sanitised name
		return template.HTML(fmt.Sprintf(`<audio class="ssg-audio" %s controls preload="none" src="%s"></audio>`,
			listenMarker, stdhtml.EscapeString(p.AudioURL)))
	}
	attrs := ""
	switch {
	case p.AudioURL != "":
		attrs = ` data-audio-src="` + stdhtml.EscapeString(p.AudioURL) + `"`
	case !g.config.Listen.Enabled || !g.listenSelected(g.isPost(p)):
		return ""
	}
	if v := strings.TrimSpace(g.config.Listen.Voice); v != "" {
		attrs += ` data-voice="` + stdhtml.EscapeString(v) + `"`
	}
	label := strings.TrimSpace(g.config.Listen.Label)
	if label == "" {
		label = "Listen"
	}
	button := `<button type="button" class="ssg-listen" ` + listenMarker + attrs + ` aria-pressed="false" hidden>` +
		listenIcon + `<span class="ssg-listen-label">` + stdhtml.EscapeString(label) + `</span></button>`
	if p.AudioURL != "" {
		// Without JavaScript the button stays hidden; the MP3 is still a link.
		button += `<noscript><a class="ssg-listen-link" href="` + stdhtml.EscapeString(p.AudioURL) + `">` +
			stdhtml.EscapeString(label) + ` (MP3)</a></noscript>`
	}
	// #nosec G203 -- every value in it is HTML-escaped above
	return template.HTML(button)
}

// listenSelected reports whether a page is in the listen sections.
func (g *Generator) listenSelected(isPost bool) bool {
	sections := g.config.Listen.Sections
	if len(sections) == 0 {
		return isPost
	}
	for _, s := range sections {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "posts", "post":
			if isPost {
				return true
			}
		case "pages", "page":
			if !isPost {
				return true
			}
		}
	}
	return false
}

// isPost reports whether p is one of the site's posts. The set of post URLs
// is built once per build: pages render on a worker pool, and the frontmatter
// type is optional, so membership is the only reliable answer.
func (g *Generator) isPost(p models.Page) bool {
	g.postURLsOnce.Do(func() {
		g.postURLs = make(map[string]bool, len(g.siteData.Posts))
		for _, post := range g.siteData.Posts {
			g.postURLs[post.GetURL()] = true
		}
	})
	return g.postURLs[p.GetURL()]
}

// listenHTMLString places the block on a page whose theme did not — into the
// theme's slot, else after the first </h1> — removes a slot left empty, and
// adds the script where a button needs it.
func (g *Generator) listenHTMLString(s string, page *models.Page) string {
	if page != nil && !strings.Contains(s, " "+listenMarker+" ") && !g.config.Listen.NoAuto {
		if block := string(g.listenBlock(*page)); block != "" {
			s = placeListenBlock(s, block)
		}
	}
	s = strings.ReplaceAll(s, listenSlot, "")
	if strings.Contains(s, `<button type="button" class="ssg-listen"`) && !strings.Contains(s, "data-ssg-listen-script") {
		if i := strings.LastIndex(s, "</body>"); i >= 0 {
			s = s[:i] + listenScript + s[i:]
		}
	}
	return s
}

// placeListenBlock puts block into the first slot, or after the first </h1>.
func placeListenBlock(s, block string) string {
	if i := strings.Index(s, listenSlot); i >= 0 {
		return s[:i] + block + s[i+len(listenSlot):]
	}
	if i := strings.Index(s, "</h1>"); i >= 0 {
		return s[:i+5] + "\n" + block + s[i+5:]
	}
	return s
}
