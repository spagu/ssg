package generator

// The AMP rules, the slot, a theme's own story.html, and the hub page.

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

func TestAMPDetectionAndFinish(t *testing.T) {
	for in, want := range map[string]bool{`<!doctype html><html amp lang="en">`: true, "<html ⚡>": true,
		`<html lang="amp">`: false, `<html class="amp-x">`: false, "<p>x</p>": false} {
		if isAMPDocument(in) != want {
			t.Errorf("isAMPDocument(%q) != %v", in, want)
		}
	}
	if isAMPDocument(strings.Repeat(" ", 5000) + "<html amp>") {
		t.Error("only the head is read")
	}
	g := storyGen(t)
	g.config.PrettyHTML = true
	if got := g.finishAMP("<html amp>\n\n<body>  </body>"); got != "<html amp>\n<body>  </body>\n" {
		t.Errorf("prettified = %q", got)
	}
	g.config.MinifyHTML = true
	if got := g.finishAMP("<html amp>\n  <body> </body>"); got != "<html amp><body></body>" {
		t.Errorf("minified = %q", got)
	}
	// An AMP page through the ordinary transforms gets none of the injections.
	g.config.AnalyticsIDs = map[string]string{"gtm": "GTM-X"}
	if out := g.transformHTMLPage(`<html amp><head></head><body></body></html>`, &models.Page{Title: "x"}, false); strings.Contains(out, "GTM-X") {
		t.Errorf("analytics injected into AMP: %s", out)
	}
}

func TestStorySlot(t *testing.T) {
	g := storyGen(t)
	post := models.Page{Slug: "cake", Type: "page"}
	g.webStories = storySet{byPost: map[string]string{post.GetURL(): "/web-stories/cake/"}}
	in := `<p>x</p><span class="s" data-ssg-story-slot>Zobacz <b>relację</b></span>`
	if got := g.fillStorySlot(in, &post); got != `<p>x</p><span class="s" data-ssg-story-slot><a class="ssg-story-link" href="/web-stories/cake/">Zobacz <b>relację</b></a></span>` {
		t.Errorf("custom label = %s", got)
	}
	if got := g.fillStorySlot(in, nil); got != "<p>x</p>" {
		t.Errorf("a page-less listing keeps no slot: %s", got)
	}
	if got := g.fillStorySlot("<p>none</p>", &post); got != "<p>none</p>" {
		t.Error("no slot, no change")
	}
}

func TestStoryThemeOverrideAndErrors(t *testing.T) {
	g := storyGen(t)
	g.tmpl = template.Must(template.New("t").Parse(`{{define "story.html"}}<html amp><body>{{.Title}} {{.CTALabel}}</body></html>{{end}}`))
	out, err := g.renderStory(StoryContext{Title: "Cake", CTALabel: "Go"})
	if err != nil || out != "<html amp><body>Cake Go</body></html>" {
		t.Errorf("override = %q, %v", out, err)
	}
	g.tmpl = template.Must(template.New("t").Parse(`{{define "story.html"}}{{.Missing}}{{end}}`))
	if _, err := g.renderStory(StoryContext{}); err == nil {
		t.Error("a failing template is an error")
	}
	if err := g.writeStory(&webStory{path: "/web-stories/x/"}); err == nil {
		t.Error("writeStory reports a render error")
	}
	g.tmpl = nil
	if err := g.writeStory(&webStory{path: "/../../escape/"}); err == nil {
		t.Error("a story may not be written outside the output directory")
	}
}

func TestStoryHubs(t *testing.T) {
	g := storyGen(t)
	g.pending = map[string]bool{}
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	mk := func(prefix string, d time.Time) *webStory {
		return &webStory{prefix: prefix, lang: "en", date: d, path: "/" + prefix + "/web-stories/x/",
			ctx: StoryContext{Title: "A & B"}, poster: StoryImage{Path: "/p.jpg", Width: 640, Height: 853}}
	}
	b := &storyBuild{taken: map[string]bool{"/pl/web-stories/": true},
		stories: []*webStory{mk("", day), mk("", day.Add(time.Hour)), mk("pl", day)}}
	if n := g.addStoryHubs(b); n != 1 || len(g.siteData.Pages) != 1 {
		t.Fatalf("hubs = %d", n)
	}
	hub := g.siteData.Pages[0]
	if hub.Link != "/web-stories/" || !hub.Date.Equal(day.Add(time.Hour)) || !strings.Contains(hub.Content, "A &amp; B") ||
		hub.Extra["hide_from_lists"] != true || len(hub.Extra["web_stories"].([]StoryCard)) != 2 || len(g.pending) == 0 {
		t.Errorf("hub = %+v", hub)
	}
}
