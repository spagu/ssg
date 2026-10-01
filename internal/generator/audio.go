package generator

// Articles read aloud, made at build time (1.8.65). Each selected page's text
// goes to the configured TTS API once; the MP3 is cached by everything that
// shapes it — provider, voice, language, jingle and the text itself — so a
// rebuild calls the API only for pages whose words changed.
//
// When the API cannot answer, the build does not hang on it page after page:
// the client's breaker stops calling it, and on_failure decides what a page
// gets instead (see audioFallback).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	stdhtml "html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/cache"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/tts"
)

// AudioOptions configures build-time audio; a nil Client turns it off.
type AudioOptions struct {
	Client    *tts.Client
	OnFailure string   // stale (default), skip, fail
	Sections  []string // posts (default), pages
	Dir       string   // output directory, default "audio"
	JingleURL string
	Timeout   time.Duration // for the jingle download
	CacheRoot string        // "" = .ssg-cache
	Feed      bool
	FeedPath  string
	FeedTitle string
	FeedAuthor,
	FeedImage string
	FeedLimit int
}

// audioStats counts what a build did, for its one summary line.
type audioStats struct {
	made, cached, stale, missing int
	breakerNoted                 bool // the open circuit is reported once, not per page
}

// audioPreBlockRe matches code blocks, which are not read aloud: "open brace,
// return err, close brace" helps no listener.
var audioPreBlockRe = regexp.MustCompile(`(?is)<pre\b.*?</pre>`)

// audioNameRe is what an output file name may contain.
var audioNameRe = regexp.MustCompile(`[^a-z0-9._-]+`)

// generateAudio renders every selected page to MP3 and records its URL on the
// page, before the pages are rendered, so templates can link it.
func (g *Generator) generateAudio() error {
	opts := g.config.Audio
	if opts.Client == nil {
		return nil
	}
	g.log("🎧 Reading articles aloud...")
	jingle, err := g.audioJingle()
	if err != nil {
		return err
	}
	var stats audioStats
	for _, pages := range g.audioPageSets() {
		for i := range *pages {
			if err := g.audioForPage(&(*pages)[i], jingle, &stats); err != nil {
				return err
			}
		}
	}
	g.log(fmt.Sprintf("   🎧 audio: %d made, %d from cache, %d stale, %d without", stats.made, stats.cached, stats.stale, stats.missing))
	return nil
}

// audioPageSets is the page lists sections names: posts unless told otherwise.
func (g *Generator) audioPageSets() []*[]models.Page {
	sections := g.config.Audio.Sections
	if len(sections) == 0 {
		sections = []string{"posts"}
	}
	var sets []*[]models.Page
	for _, s := range sections {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "posts", "post":
			sets = append(sets, &g.siteData.Posts)
		case "pages", "page":
			sets = append(sets, &g.siteData.Pages)
		}
	}
	return sets
}

// audioCacheDir is .ssg-cache/tts (or under the configured root).
func (g *Generator) audioCacheDir() string { return cache.Dir(g.config.Audio.CacheRoot, "tts") }

// audioJingle returns the intro, downloading it once per jingle URL.
func (g *Generator) audioJingle() ([]byte, error) {
	u := strings.TrimSpace(g.config.Audio.JingleURL)
	if u == "" {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(u))
	name := "jingle-" + hex.EncodeToString(sum[:8]) + ".mp3"
	path := filepath.Join(g.audioCacheDir(), name)
	if data, err := os.ReadFile(path); err == nil { // #nosec G304 -- our own cache file
		return data, nil
	}
	data, err := tts.FetchJingle(context.Background(), u, g.config.Audio.Timeout)
	if err != nil {
		// A missing intro is not worth a missing episode: say so and go on.
		g.log("   ⚠️  " + err.Error() + " — articles are read without it")
		return nil, nil
	}
	_ = cache.WriteAtomicBytes(g.audioCacheDir(), name, 0o644, data)
	return data, nil
}

// audioForPage makes, reuses or falls back for one page's audio.
func (g *Generator) audioForPage(p *models.Page, jingle []byte, stats *audioStats) error {
	text := g.audioText(*p)
	if text == "" {
		return nil
	}
	lang := p.Lang
	key := audioKey(g.config.Audio.Client.Describe(), lang, jingle, text)
	cacheDir := g.audioCacheDir()
	data, err := os.ReadFile(filepath.Join(cacheDir, key+".mp3")) // #nosec G304 -- our own cache file
	if err == nil {
		stats.cached++
	} else {
		data, err = g.config.Audio.Client.Synthesize(context.Background(), text, lang)
		if err != nil {
			return g.audioFallback(p, err, stats)
		}
		data = tts.Join(jingle, data)
		if err := cache.WriteAtomicBytes(cacheDir, key+".mp3", 0o644, data); err != nil {
			return fmt.Errorf("caching audio for %s: %w", p.GetURL(), err)
		}
		stats.made++
	}
	_ = cache.WriteAtomicBytes(filepath.Join(cacheDir, "last"), audioName(*p), 0o644, data)
	return g.publishAudio(p, data)
}

// audioFallback decides what a page gets when its audio could not be made.
// "fail" — or any mode under strict — stops the build. Otherwise the page's
// last good MP3 is used if there is one ("stale", the default): the words may
// have changed since, but a slightly old reading beats none. Failing that,
// the page is published without audio, and the listen button, when enabled,
// still reads it with the browser's voice.
func (g *Generator) audioFallback(p *models.Page, cause error, stats *audioStats) error {
	mode := strings.ToLower(strings.TrimSpace(g.config.Audio.OnFailure))
	if mode == "fail" || g.config.Strict {
		return fmt.Errorf("audio for %s: %w", p.GetURL(), cause)
	}
	switch {
	case !errors.Is(cause, tts.ErrCircuitOpen):
		g.log(fmt.Sprintf("   ⚠️  audio for %s: %v", p.GetURL(), cause))
	case !stats.breakerNoted:
		stats.breakerNoted = true
		g.log("   ⚠️  " + cause.Error())
	}
	if mode != "skip" {
		last := filepath.Join(g.audioCacheDir(), "last", audioName(*p))
		if data, err := os.ReadFile(last); err == nil { // #nosec G304 -- our own cache file
			stats.stale++
			return g.publishAudio(p, data)
		}
	}
	stats.missing++
	return nil
}

// publishAudio writes the MP3 into the output and records it on the page.
func (g *Generator) publishAudio(p *models.Page, data []byte) error {
	dir := strings.Trim(g.config.Audio.Dir, "/")
	if dir == "" {
		dir = "audio"
	}
	name := audioName(*p)
	outDir := filepath.Join(g.config.OutputDir, filepath.FromSlash(dir))
	if err := g.ensureWithinOutput(filepath.Join(outDir, name)); err != nil {
		return err
	}
	if err := cache.WriteAtomicBytes(outDir, name, 0o644, data); err != nil {
		return fmt.Errorf("writing audio for %s: %w", p.GetURL(), err)
	}
	p.AudioURL = "/" + dir + "/" + name
	p.AudioLength = int64(len(data))
	return nil
}

// audioText is what is read: the title, then the article without its code
// blocks and markup.
func (g *Generator) audioText(p models.Page) string {
	body := audioPreBlockRe.ReplaceAllString(g.convertMarkdownToHTML(p.Content), " ")
	body = strings.TrimSpace(tmplStripHTML(body))
	if body == "" {
		return ""
	}
	return strings.TrimSpace(p.Title + ".\n\n" + stdhtml.UnescapeString(body))
}

// audioKey is the cache key: everything that changes the sound.
func audioKey(describe, lang string, jingle []byte, text string) string {
	k := cache.NewKeyer("tts-v1", 32)
	k.WriteDelim(describe)
	k.WriteDelim(lang)
	k.Write(jingle)
	k.WriteDelim("")
	k.WriteString(text)
	return k.Sum()
}

// audioName is the page's MP3 file name, derived from its URL:
// /2026/09/30/hello/ → 2026-09-30-hello.mp3, the front page → index.mp3.
func audioName(p models.Page) string {
	name := strings.ToLower(strings.Trim(p.GetURL(), "/"))
	name = strings.TrimSuffix(name, ".html")
	name = strings.Trim(audioNameRe.ReplaceAllString(strings.ReplaceAll(name, "/", "-"), "-"), "-.")
	if name == "" {
		name = "index"
	}
	return name + ".mp3"
}
