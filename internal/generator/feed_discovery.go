package generator

// Feed autodiscovery: the <link rel="alternate"> elements a feed reader reads
// when someone presses subscribe (#86, #282).

import (
	"fmt"
	stdhtml "html"
	"path/filepath"
	"strings"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// feedAutodiscoveryLinks renders one <link rel="alternate"> per feed the site
// publishes, each with its own MIME type and title.
//
// A reader that offers a choice reads exactly these links, so publishing four
// feeds behind a single Atom <link> hides three of them. The built-in feed is
// included when `feed: true`, and a declared feed whose format is unknown is
// skipped rather than advertised with a wrong type.
//
// lang is the language of the page the links go into: under i18n the built-in
// link names that language's feed (#282).
func (g *Generator) feedAutodiscoveryLinks(lang string) string {
	var sb strings.Builder
	if g.config.Feed {
		href, title := g.builtinFeedLink(lang)
		fmt.Fprintf(&sb, `<link rel="alternate" type="application/atom+xml" title=%q href=%q>`+"\n",
			stdhtml.EscapeString(title), href)
	}
	for _, spec := range g.config.Feeds {
		rel := models.SanitizeRelPath(strings.TrimSpace(spec.Path))
		if rel == "" {
			continue
		}
		format, _, err := feedFormatOf(spec)
		if err != nil {
			continue
		}
		title := strings.TrimSpace(spec.Title)
		if title == "" {
			title = g.config.Domain
		}
		fmt.Fprintf(&sb, `<link rel="alternate" type=%q title=%q href="/%s">`+"\n",
			format.mime, stdhtml.EscapeString(title), filepath.ToSlash(rel))
	}
	return sb.String()
}

// builtinFeedLink is the address and title of the `feed: true` feed a page in
// lang should offer.
//
// Under i18n the build writes one feed per language — /feed.xml, /pl/feed.xml,
// /ro/feed.xml — and every page used to advertise /feed.xml, so a reader on a
// Polish page subscribed to the English posts, and with
// prefix_default_language set, to a file that was never written at all. A page
// in a language the site does not declare keeps the root feed.
func (g *Generator) builtinFeedLink(lang string) (href, title string) {
	if g.config.I18n.Enabled {
		if language, ok := ssgi18n.Language(g.siteData.Languages, lang); ok {
			rel := feedFileName
			if prefix := ssgi18n.Prefix(language.Code, g.config.DefaultLanguage, g.config.I18n); prefix != "" {
				rel = prefix + "/" + feedFileName
			}
			return "/" + rel, g.builtinFeedTitle(language.Name)
		}
	}
	return "/" + feedFileName, g.builtinFeedTitle("")
}

// builtinFeedTitle names the built-in feed, in the feed itself and in the link
// that offers it: the site's title where it has one, and the domain otherwise.
// The subscribe entry is the one place a person reads this string, and it said
// "example.com" on a site whose config names itself (#282).
func (g *Generator) builtinFeedTitle(languageName string) string {
	title := g.config.Domain
	if g.siteData != nil && strings.TrimSpace(g.siteData.Title) != "" {
		title = strings.TrimSpace(g.siteData.Title)
	}
	if languageName != "" {
		title += " (" + languageName + ")"
	}
	return title
}

// injectFeedLinks adds the autodiscovery <link> elements to a rendered page,
// unless the page already advertises a feed of its own. Applied to every HTML
// page — including the homepage, which carries no page context and so was
// skipped by the SEO block that used to own this (#86).
func (g *Generator) injectFeedLinks(s, lang string) string {
	if !g.config.Feed && len(g.config.Feeds) == 0 {
		return s
	}
	// feed_autodiscovery: false hands the links to the theme, for a site that
	// wants control over their order, titles or which feeds are advertised.
	if g.config.FeedAutodiscovery != nil && !*g.config.FeedAutodiscovery {
		return s
	}
	if strings.Contains(s, `rel="alternate" type="application/atom+xml"`) ||
		strings.Contains(s, `rel="alternate" type="application/rss+xml"`) ||
		strings.Contains(s, `rel="alternate" type="application/feed+json"`) {
		return s // the theme provides its own
	}
	links := g.feedAutodiscoveryLinks(lang)
	if links == "" {
		return s
	}
	if i := strings.LastIndex(s, "</head>"); i >= 0 {
		return s[:i] + links + s[i:]
	}
	return s
}
