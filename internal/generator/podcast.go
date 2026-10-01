package generator

// A podcast feed of the articles read aloud (1.8.65): RSS 2.0 with an
// <enclosure> per MP3 and the iTunes tags podcast apps read. It lists the
// pages generateAudio gave an MP3, newest first.

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

type podcastRSS struct {
	XMLName xml.Name       `xml:"rss"`
	Version string         `xml:"version,attr"`
	ITunes  string         `xml:"xmlns:itunes,attr"`
	Channel podcastChannel `xml:"channel"`
}

type podcastChannel struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Description string        `xml:"description"`
	Language    string        `xml:"language,omitempty"`
	Author      string        `xml:"itunes:author,omitempty"`
	Image       *podcastImage `xml:"itunes:image,omitempty"`
	Explicit    string        `xml:"itunes:explicit"`
	Items       []podcastItem `xml:"item"`
}

type podcastImage struct {
	Href string `xml:"href,attr"`
}

type podcastItem struct {
	Title       string           `xml:"title"`
	Link        string           `xml:"link"`
	GUID        string           `xml:"guid"`
	PubDate     string           `xml:"pubDate,omitempty"`
	Description string           `xml:"description,omitempty"`
	Enclosure   podcastEnclosure `xml:"enclosure"`
}

type podcastEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

// generatePodcastFeed writes the feed when tts.feed is on and any page has audio.
func (g *Generator) generatePodcastFeed() error {
	opts := g.config.Audio
	if opts.Client == nil || !opts.Feed {
		return nil
	}
	pages := g.pagesWithAudio()
	if len(pages) == 0 {
		return nil
	}
	path := strings.TrimLeft(opts.FeedPath, "/")
	if path == "" {
		path = "podcast.xml"
	}
	out := filepath.Join(g.config.OutputDir, filepath.FromSlash(path))
	if err := g.ensureWithinOutput(out); err != nil {
		return err
	}
	if err := g.ensureParent(out); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(g.podcastFeed(pages), "", "  ")
	if err != nil {
		return err
	}
	// #nosec G306 -- a feed is public web content
	return os.WriteFile(out, append([]byte(xml.Header), data...), 0644)
}

// pagesWithAudio is every page with an MP3, newest first, capped at FeedLimit.
func (g *Generator) pagesWithAudio() []models.Page {
	var pages []models.Page
	for _, set := range [][]models.Page{g.siteData.Posts, g.siteData.Pages} {
		for _, p := range set {
			if p.AudioURL != "" {
				pages = append(pages, p)
			}
		}
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Date.After(pages[j].Date) })
	limit := g.config.Audio.FeedLimit
	if limit <= 0 {
		limit = 50
	}
	if len(pages) > limit {
		pages = pages[:limit]
	}
	return pages
}

// podcastFeed builds the document; every URL is absolute on the site's domain.
func (g *Generator) podcastFeed(pages []models.Page) podcastRSS {
	opts := g.config.Audio
	site := "https://" + strings.TrimRight(g.config.Domain, "/")
	title := opts.FeedTitle
	if title == "" {
		title = g.config.Domain
	}
	ch := podcastChannel{Title: title, Link: site + "/", Description: title, Language: g.config.DefaultLanguage,
		Author: opts.FeedAuthor, Explicit: "false"}
	if opts.FeedImage != "" {
		img := opts.FeedImage
		if strings.HasPrefix(img, "/") {
			img = site + img
		}
		ch.Image = &podcastImage{Href: img}
	}
	for _, p := range pages {
		link := site + p.GetURL()
		item := podcastItem{Title: p.Title, Link: link, GUID: link, Description: p.Excerpt,
			Enclosure: podcastEnclosure{URL: site + p.AudioURL, Length: p.AudioLength, Type: "audio/mpeg"}}
		if !p.Date.IsZero() {
			item.PubDate = p.Date.UTC().Format(time.RFC1123Z)
		}
		ch.Items = append(ch.Items, item)
	}
	return podcastRSS{Version: "2.0", ITunes: "http://www.itunes.com/dtds/podcast-1.0.dtd", Channel: ch}
}
