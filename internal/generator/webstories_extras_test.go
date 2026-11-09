package generator

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/tts"
)

// storyMP3 is ten MPEG-1 Layer III frames, about 0.26 s.
var storyMP3 = func() []byte {
	f := append([]byte{0xFF, 0xFB, 0x90, 0x00}, make([]byte, 413)...)
	return bytes.Repeat(f, 10)
}()

// storyTTS is a speech service answering every request with storyMP3, and
// the texts it was asked to read.
func storyTTS(t *testing.T, fail bool) (*tts.Client, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var said []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Text string }
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		mu.Lock()
		said = append(said, body.Text)
		mu.Unlock()
		if fail {
			http.Error(w, "down", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(storyMP3)
	}))
	t.Cleanup(srv.Close)
	c, err := tts.New(tts.Config{APIURL: srv.URL, Retries: -1, Breaker: -1, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c, &said
}

// storyExtrasSettings is a story: with everything a Web Story takes from it.
func storyExtrasSettings() StoryOptions {
	return StoryOptions{Enabled: true, Settings: map[string]any{
		"narrator": map[string]any{"pronounce": map[string]any{"Chocolate": "Czekoladowe"}},
		"widgets": []any{
			map[string]any{"type": "stat", "stat": "180°C", "title": "Oven"},
			map[string]any{"text": "Line the tin."},
			map[string]any{"type": "quote", "text": "Best on day two."},
			map[string]any{"type": "recipe"},
			map[string]any{"html": "<b>only markup</b>"},
		},
		"audio": []any{
			map[string]any{"from": "12-01", "to": "12-30", "src": "/music/carol.mp3"},
			map[string]any{"dates": []any{map[string]any{"from": "2026-03-01", "to": "2026-03-31"}}, "src": "/music/spring.mp3"},
			map[string]any{"playlist": []any{"https://cdn.example.com/a.mp3"}},
		},
		"slideshow": map[string]any{
			"header": map[string]any{"logo": "/media/logo.png", "text": "Cake Co", "logo_height": 40},
			"footer": map[string]any{"label": "Sponsored", "items": []any{
				map[string]any{"link": "/shop/", "text": "Tins", "image": "/media/logo.png"},
				map[string]any{"link": "https://example.org/x", "text": "Elsewhere"},
				map[string]any{"text": "no link"},
			}},
		},
	}}
}

// TestWebStoryExtras: header, widgets, footer strip and page, music and the
// ingredients panel from a post's story: and recipe.
func TestWebStoryExtras(t *testing.T) {
	defer func(f func() time.Time) { storyNow = f }(storyNow)
	storyNow = func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) }
	cfg, _ := buildStorySite(t, func(c *Config) {
		c.Story = storyExtrasSettings()
		site := filepath.Join(c.ContentDir, c.Source)
		post := strings.Replace(storyCakePost, "  recipeYield: \"16\"\n", "  recipeYield: \"16\"\n  recipeIngredient: [\"200 g flour\", \"3 eggs\"]\n", 1)
		if err := os.WriteFile(filepath.Join(site, "posts", "recipes", "cake.md"), []byte(post), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	story := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html"))
	assertAMPStory(t, story)
	for _, want := range []string{
		`background-audio="https://example.com/music/spring.mp3"`,
		`<amp-img src="https://example.com/media/logo.png" height="40" width="auto" layout="fixed-height" alt="Cake Co">`,
		`<b class="stat">180°C</b><h3>Oven</h3>`, `<p>Line the tin.</p>`, `widget-quote`, `animate-in-delay="3.7s"`,
		`<small>Sponsored</small>`, `href="https://example.com/shop/"`, `href="https://example.org/x"`,
		`<amp-story-page id="recommended"`, `cta-text="Ingredients"`, `<li>200 g flour</li>`,
		`class="has-footer"`, `@keyframes story-rot`, `.strip .slot:nth-of-type(2){animation-delay:4s}`,
	} {
		if !strings.Contains(story, want) {
			t.Errorf("story lacks %q", want)
		}
	}
	if strings.Contains(story, "only markup") || strings.Contains(story, "no link") {
		t.Error("a widget with nothing to show, or a promo without a link, is left out")
	}
	cta := story[strings.Index(story, `<amp-story-page id="read-more"`):]
	if strings.Contains(cta, "amp-story-page-attachment") {
		t.Error("the last page carries the outlink, not the panel")
	}
}

// TestWebStoryNarration: each page is recorded through tts: with the
// pronunciation rules, written beside the story and timed to its length;
// the music steps aside; a second build reads the cache.
func TestWebStoryNarration(t *testing.T) {
	client, said := storyTTS(t, false)
	cacheRoot := t.TempDir()
	build := func() string {
		cfg, _ := buildStorySite(t, func(c *Config) {
			c.Story = storyExtrasSettings()
			c.WebStories.Narration = true
			c.Audio.Client = client
			c.Audio.CacheRoot = cacheRoot
		})
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "narration", "cover.mp3")); err != nil {
			t.Errorf("cover narration not written: %v", err)
		}
		return mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html"))
	}
	story := build()
	for _, want := range []string{
		`<amp-story-page id="cover" background-audio="https://example.com/web-stories/chocolate-cake/narration/cover.mp3" auto-advance-after="0.9s"`,
		`<amp-story-page id="read-more" background-audio=`,
	} {
		if !strings.Contains(story, want) {
			t.Errorf("story lacks %q", want)
		}
	}
	if strings.Contains(story, `<amp-story standalone supports-landscape title="Chocolate Cake" publisher="`) &&
		strings.Contains(strings.SplitN(story, "<amp-story-page", 2)[0], "background-audio") {
		t.Error("a narrated story plays no music under it")
	}
	if !strings.Contains(strings.Join(*said, "|"), "|Czekoladowe Cake|") {
		t.Errorf("the cover is read with the pronunciation rules: %q", *said)
	}
	asked := len(*said)
	build()
	if len(*said) != asked {
		t.Errorf("a rebuild asked again: %d → %d", asked, len(*said))
	}
}

// TestWebStoryNarrationFails: without tts: or with a failing one the story
// is made silent and the build goes on.
func TestWebStoryNarrationFails(t *testing.T) {
	_, log := buildStorySite(t, func(c *Config) {
		c.Quiet = false
		c.WebStories.Narration = true
	})
	if !strings.Contains(log, "web_stories.narration needs tts:") {
		t.Error("a missing tts: is reported")
	}
	client, _ := storyTTS(t, true)
	cfg, log := buildStorySite(t, func(c *Config) {
		c.Quiet = false
		c.WebStories.Narration = true
		c.Audio.Client = client
		c.Audio.CacheRoot = t.TempDir()
	})
	if !strings.Contains(log, "narration failed") {
		t.Error("a failed recording is reported")
	}
	if story := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html")); strings.Contains(story, "narration/") {
		t.Error("a failed page has no audio")
	}
}

// TestStoryMusicWindows: the date rule of story.js.
func TestStoryMusicWindows(t *testing.T) {
	at := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	for _, tt := range []struct {
		a    map[string]any
		day  string
		want bool
	}{
		{map[string]any{}, "2026-05-05", true},
		{map[string]any{"from": "12-20", "to": "01-06"}, "2026-01-02", true},
		{map[string]any{"from": "12-20", "to": "01-06"}, "2026-02-02", false},
		{map[string]any{"from": "05-01"}, "2026-06-01", true},
		{map[string]any{"to": "2026-04-01"}, "2026-03-01", true},
		{map[string]any{"dates": []any{map[string]any{"from": "2027-03-01", "to": "2027-03-31"}}}, "2026-03-10", false},
	} {
		if got := storyToday(tt.a, at(tt.day)); got != tt.want {
			t.Errorf("%v on %s = %v", tt.a, tt.day, got)
		}
	}
	g := &Generator{config: Config{Domain: "example.com"}, siteData: nil}
	if got := g.storyMusic(map[string]any{"src": "/a.mp3"}); got != "https://example.com/a.mp3" {
		t.Errorf("one entry = %q", got)
	}
	if got := g.storyMusic([]any{"x", map[string]any{"from": "01-01", "to": "01-01", "src": "/b.mp3"}}); got != "" && storyNow().Format("01-02") != "01-01" {
		t.Errorf("no entry for today = %q", got)
	}
	if storyString(3) != "3" || storyString(nil) != "" {
		t.Error("storyString")
	}
	if g.storyHeader(map[string]any{}, "P") != nil || g.storyFooter(map[string]any{}) != nil ||
		g.storyFooter(map[string]any{"footer": map[string]any{"text": "x"}}) != nil {
		t.Error("no header or footer without a logo or a link")
	}
	one := g.storyFooter(map[string]any{"footer": map[string]any{"link": "/x/", "text": "One"}})
	if one == nil || one.Items[0].URL != "https://example.com/x/" || storyFooterCSS(one) != ".strip .slot{opacity:1;visibility:visible;animation:none}" {
		t.Errorf("a single promo stands still: %+v", one)
	}
	if h := g.storyHeader(map[string]any{"header": map[string]any{"logo": "/l.png"}}, "Pub"); h.Alt != "Pub" || h.Height != 32 {
		t.Errorf("header defaults = %+v", h)
	}
}

// TestWebStoryNarrationEdges: a page with nothing to say stays silent; an
// output that cannot take the file reports the page.
func TestWebStoryNarrationEdges(t *testing.T) {
	client, said := storyTTS(t, false)
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Quiet: true, Audio: AudioOptions{Client: client, CacheRoot: t.TempDir()}},
		siteData: nil}
	b := &storyBuild{g: g, opts: WebStoriesOptions{Narration: true}}
	ctx := StoryContext{Title: "T", CTALabel: "Read", Pages: []StoryPage{{ID: "page-1"}}, Music: "/m.mp3"}
	b.narrate(&ctx, "/web-stories/t/", "en")
	if ctx.Pages[0].Audio != "" || ctx.Cover.Audio == "" || ctx.Music != "" || len(*said) != 2 {
		t.Errorf("silent caption page = %+v, said %q", ctx, *said)
	}
	if err := os.WriteFile(filepath.Join(out, "blocked"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.record(client, "/../../escape/", "cover", "T", "en"); err == nil {
		t.Error("a path out of the output: no error")
	}
	if _, _, err := b.record(client, "/blocked/", "cover", "T", "en"); err == nil {
		t.Error("a file where the directory should be: no error")
	}
	if err := os.MkdirAll(filepath.Join(out, "dir", "narration", "cover.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.record(client, "/dir/", "cover", "T", "en"); err == nil {
		t.Error("a directory where the file should be: no error")
	}
}

// TestWebStoryPagesCap: never more photo pages than web_stories.pages.
func TestWebStoryPagesCap(t *testing.T) {
	cfg, _ := buildStorySite(t, func(c *Config) { c.WebStories.Pages = 2 })
	story := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html"))
	if strings.Contains(story, `id="page-3"`) || !strings.Contains(story, `id="page-2"`) {
		t.Error("pages: 2 makes two photo pages")
	}
}

// TestWebStoryLogoFromProjectRoot: publisher_logo may be written from the
// project root, naming the directory it sits in (#402).
func TestWebStoryLogoFromProjectRoot(t *testing.T) {
	cfg, _ := buildStorySite(t, func(c *Config) {
		c.WebStories.PublisherLogo = filepath.Join(c.ContentDir, c.Source, "media", "logo.png")
	})
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html")); err != nil {
		t.Errorf("a logo path from the project root makes stories: %v", err)
	}
}

// TestWebStoryBackgroundAudio: web_stories.background_audio for every story,
// a post's story_audio over it, story_audio: false for silence (#403).
func TestWebStoryBackgroundAudio(t *testing.T) {
	cfg, _ := buildStorySite(t, func(c *Config) { c.WebStories.BackgroundAudio = "audio/theme.mp3" })
	story := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html"))
	if !strings.Contains(story, `background-audio="https://example.com/audio/theme.mp3"`) {
		t.Error("the site-wide soundtrack is set")
	}
	g := &Generator{config: Config{Domain: "example.com"}}
	b := &storyBuild{g: g, opts: WebStoriesOptions{BackgroundAudio: "/site.mp3"}}
	for _, tt := range []struct {
		name  string
		extra map[string]any
		today string
		want  string
	}{
		{"site", nil, "", "https://example.com/site.mp3"},
		{"today's track wins", nil, "https://x/t.mp3", "https://x/t.mp3"},
		{"the post's own", map[string]any{"story_audio": "/own.mp3"}, "https://x/t.mp3", "https://example.com/own.mp3"},
		{"off", map[string]any{"story_audio": false}, "https://x/t.mp3", ""},
		{"true keeps the default", map[string]any{"story_audio": true}, "", "https://example.com/site.mp3"},
		{"blank keeps the default", map[string]any{"story_audio": " "}, "", "https://example.com/site.mp3"},
	} {
		if got := b.backgroundAudio(models.Page{Extra: tt.extra}, tt.today); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
		}
	}
	b.opts.BackgroundAudio = ""
	if got := b.backgroundAudio(models.Page{}, ""); got != "" {
		t.Errorf("nothing set = %q", got)
	}
}
