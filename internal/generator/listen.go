package generator

// The two ways an article can be listened to (1.8.65):
//
//   - an MP3 made at build time by a TTS API (audio.go) — the same voice for
//     every visitor, playable offline and in a podcast app;
//   - a "Listen" button that reads the page with the visitor's own browser
//     voice (Web Speech API) — free, nothing to host, nothing leaves the device,
//     and the voice is whatever the device has.
//
// {{ listen .Page }} renders whichever applies: the player when the page has
// audio, else the button when listen is on, else nothing. A theme that does
// not call it gets it after the first </h1> of a post, unless listen.auto is
// false.

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
	NoAuto   bool     // do not place it on pages whose theme did not
	Sections []string // posts (default), pages
}

// listenMarker is the attribute every block carries, so the auto placement
// and the script injection can tell the theme already placed one.
const listenMarker = "data-ssg-listen"

// listenScript reads the article with speechSynthesis. It speaks paragraph
// by paragraph, because Chrome stops a single long utterance after about 15
// seconds; the button toggles between play and stop, reports its state with
// aria-pressed, and stays hidden where the API is missing.
const listenScript = `<script data-ssg-listen-script>(function(){` +
	`var s=window.speechSynthesis;if(!s||!window.SpeechSynthesisUtterance)return;` +
	`document.querySelectorAll('button[data-ssg-listen]').forEach(function(b){` +
	`b.hidden=false;var label=b.textContent,stopText=b.getAttribute('data-stop')||'Stop';` +
	`function reset(){b.setAttribute('aria-pressed','false');b.textContent=label;}` +
	`b.addEventListener('click',function(){if(s.speaking){s.cancel();reset();return;}` +
	`var root=b.closest('article')||document.querySelector('main')||document.body;` +
	`var parts=[].map.call(root.querySelectorAll('h1,h2,h3,h4,p,li,blockquote,td'),function(e){return e.innerText.trim();}).filter(Boolean);` +
	`var lang=document.documentElement.lang||'';` +
	`parts.forEach(function(t,i){var u=new SpeechSynthesisUtterance(t);if(lang)u.lang=lang;` +
	`if(i===parts.length-1){u.onend=reset;u.onerror=reset;}s.speak(u);});` +
	`b.setAttribute('aria-pressed','true');b.textContent=stopText;});});` +
	`window.addEventListener('pagehide',function(){s.cancel();});})();</script>`

// listenBlock is the HTML {{ listen .Page }} renders for a page.
func (g *Generator) listenBlock(p models.Page) template.HTML {
	if p.AudioURL != "" {
		// #nosec G203 -- the URL is built by publishAudio from a sanitised name
		return template.HTML(fmt.Sprintf(`<audio class="ssg-audio" %s controls preload="none" src="%s"></audio>`,
			listenMarker, stdhtml.EscapeString(p.AudioURL)))
	}
	if !g.config.Listen.Enabled || !g.listenSelected(g.isPost(p)) {
		return ""
	}
	label := strings.TrimSpace(g.config.Listen.Label)
	if label == "" {
		label = "Listen"
	}
	// #nosec G203 -- the label is escaped
	return template.HTML(`<button type="button" class="ssg-listen" ` + listenMarker +
		` aria-pressed="false" hidden>` + stdhtml.EscapeString(label) + `</button>`)
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

// listenHTMLString places the block on a page whose theme did not, and adds
// the script where a button needs it.
func (g *Generator) listenHTMLString(s string, page *models.Page) string {
	if page != nil && !strings.Contains(s, listenMarker) && !g.config.Listen.NoAuto {
		if block := string(g.listenBlock(*page)); block != "" {
			if i := strings.Index(s, "</h1>"); i >= 0 {
				s = s[:i+5] + "\n" + block + s[i+5:]
			}
		}
	}
	if strings.Contains(s, `<button type="button" class="ssg-listen"`) && !strings.Contains(s, "data-ssg-listen-script") {
		if i := strings.LastIndex(s, "</body>"); i >= 0 {
			s = s[:i] + listenScript + s[i:]
		}
	}
	return s
}
