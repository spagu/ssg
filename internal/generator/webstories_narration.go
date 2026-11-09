package generator

// Narrated Web Stories (1.9.4): `web_stories.narration: true` records what
// each page says — the title on the cover, a photograph's caption, the link
// text on the last page — through the site's `tts:` service, and the page
// moves on when its recording ends. AMP allows no script, so the browser's
// voice is out; this is the same narrator as story mode's, recorded at build
// time, reading the same pronunciation rules (story_say.go). Recordings are
// cached by text and voice, so a rebuild costs nothing. A page whose
// recording fails keeps AMP's default timing; the build goes on.
//
// AMP plays page audio and story audio at full volume over each other, so a
// narrated story leaves today's music out.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/cache"
	"github.com/spagu/ssg/internal/tts"
)

// storyNarrationTail is the pause after a page's last word.
const storyNarrationTail = 600 * time.Millisecond

// narrate records every page that has something to say.
func (b *storyBuild) narrate(ctx *StoryContext, path, lang string) {
	if !b.opts.Narration {
		return
	}
	client := b.g.config.Audio.Client
	if client == nil {
		if !b.narrationNoted {
			b.narrationNoted = true
			b.g.log("   ⚠️  web_stories.narration needs tts: (a speech service) — stories are not narrated")
		}
		return
	}
	var rules map[string]any
	if settings, ok := b.g.storySettings(ctx.Post, true); ok {
		narrator, _ := settings["narrator"].(map[string]any)
		rules = storyPronounce(narrator, lang)
	} else {
		rules = storyPronounce(nil, lang)
	}
	plain := make(map[string]string, len(rules))
	for k, v := range rules {
		plain[k] = storyString(v)
	}
	say := newSayer(plain, lang)
	narrated := false
	page := func(p *StoryPage, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		url, length, err := b.record(client, path, p.ID, say.Say(text), lang)
		if err != nil {
			b.g.log(fmt.Sprintf("   ⚠️  web story %s, page %s: narration failed: %v", path, p.ID, err))
			return
		}
		p.Audio, p.Advance = url, fmt.Sprintf("%.1fs", (length+storyNarrationTail).Seconds())
		narrated = true
	}
	page(&ctx.Cover, ctx.Title)
	for i := range ctx.Pages {
		page(&ctx.Pages[i], ctx.Pages[i].Caption)
	}
	page(&ctx.CTA, ctx.CTALabel)
	if narrated {
		ctx.Music = ""
	}
}

// record makes (or takes from the cache) one page's MP3 and writes it beside
// the story: its URL and length.
func (b *storyBuild) record(client *tts.Client, path, id, text, lang string) (string, time.Duration, error) {
	sum := sha256.Sum256([]byte(client.Describe() + "\x00" + lang + "\x00" + text))
	key := hex.EncodeToString(sum[:12]) + ".mp3"
	dir := cache.Dir(b.g.config.Audio.CacheRoot, "story-narration")
	data, err := os.ReadFile(filepath.Join(dir, key)) // #nosec G304 -- our own cache file
	if err != nil {
		data, err = client.Synthesize(context.Background(), text, lang)
		if err != nil {
			return "", 0, err
		}
		_ = cache.WriteAtomicBytes(dir, key, 0o644, data)
	}
	rel := path + "narration/" + id + ".mp3"
	file := filepath.Join(b.g.config.OutputDir, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
	if err := b.g.ensureWithinOutput(file); err != nil {
		return "", 0, err
	}
	if err := b.g.ensureParent(file); err != nil {
		return "", 0, err
	}
	// #nosec G306,G703 -- a public web file, checked to be inside the output directory
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return "", 0, err
	}
	return b.g.storyHref(rel), tts.Duration(data), nil
}
