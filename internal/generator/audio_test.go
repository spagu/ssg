package generator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/tts"
)

// ttsServer answers with fixed MP3 bytes, or with an error while down is set.
type ttsServer struct {
	*httptest.Server
	calls atomic.Int32
	down  atomic.Bool
}

func newTTSServer(t *testing.T) *ttsServer {
	t.Helper()
	s := &ttsServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		if s.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/jingle.mp3" {
			_, _ = w.Write([]byte("JINGLE"))
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("SPEECH"))
	}))
	t.Cleanup(s.Close)
	return s
}

// writeAudioSite writes one post and a theme whose post template places the
// block itself, and returns the source root.
func writeAudioSite(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "posts", "news", "hello.md"),
		"---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\ndate: 2026-09-30\n---\n\nSpoken words.\n\n```go\nfunc skipped() {}\n```\n")
	mustWrite(t, filepath.Join(content, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout text.\n")
	dir := filepath.Join(tmp, "templates", "simple")
	for _, name := range []string{"base.html", "index.html", "category.html"} {
		mustWrite(t, filepath.Join(dir, name), `{{define "`+name+`"}}<html><body>x</body></html>{{end}}`)
	}
	mustWrite(t, filepath.Join(dir, "post.html"), `{{define "post.html"}}<html><body><article><h1>{{.Post.Title}}</h1>{{listen .Post}}{{.Post.Content | safeHTML}}</article></body></html>{{end}}`)
	mustWrite(t, filepath.Join(dir, "page.html"), `{{define "page.html"}}<html><body><article><h1>{{.Page.Title}}</h1>{{.Page.Content | safeHTML}}</article></body></html>{{end}}`)
	return tmp
}

// buildAudioSite builds the site with the given options and returns its output.
func buildAudioSite(t *testing.T, tmp string, audio AudioOptions, listen ListenOptions, strict bool) (string, error) {
	t.Helper()
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: out, Quiet: true, Strict: strict, Audio: audio, Listen: listen})
	if err != nil {
		t.Fatal(err)
	}
	return out, gen.Generate()
}

func readAudioOut(t *testing.T, out, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func audioClient(t *testing.T, url string) *tts.Client {
	t.Helper()
	c, err := tts.New(tts.Config{APIURL: url, Retries: -1, Breaker: 1, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestAudioBuildCacheAndFeed: the post gets an MP3 with the jingle in front,
// a player, and a podcast item; the second build reads the cache instead of
// calling the API.
func TestAudioBuildCacheAndFeed(t *testing.T) {
	srv := newTTSServer(t)
	tmp := writeAudioSite(t)
	opts := AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: filepath.Join(tmp, "cache"),
		JingleURL: srv.URL + "/jingle.mp3", Feed: true, FeedTitle: "Spoken", FeedImage: "/cover.png"}
	out, err := buildAudioSite(t, tmp, opts, ListenOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if mp3 := readAudioOut(t, out, "audio/2026-09-30-hello.mp3"); mp3 != "JINGLESPEECH" {
		t.Errorf("mp3 = %q, want the jingle then the speech", mp3)
	}
	post := readAudioOut(t, out, "2026/09/30/hello/index.html")
	if !strings.Contains(post, `<audio class="ssg-audio" data-ssg-listen controls preload="none" src="/audio/2026-09-30-hello.mp3">`) {
		t.Errorf("post lacks the player:\n%s", post)
	}
	feed := readAudioOut(t, out, "podcast.xml")
	for _, want := range []string{`<title>Spoken</title>`, `url="https://example.com/audio/2026-09-30-hello.mp3"`,
		`length="12"`, `type="audio/mpeg"`, `href="https://example.com/cover.png"`, "<pubDate>"} {
		if !strings.Contains(feed, want) {
			t.Errorf("feed lacks %s:\n%s", want, feed)
		}
	}
	calls := srv.calls.Load()
	srv.down.Store(true)
	if _, err := buildAudioSite(t, tmp, AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: opts.CacheRoot,
		JingleURL: opts.JingleURL}, ListenOptions{}, false); err != nil {
		t.Fatal(err)
	}
	if srv.calls.Load() != calls {
		t.Errorf("the second build called the API %d more times; the cache must answer", srv.calls.Load()-calls)
	}
}

// TestAudioWhenTheAPIIsDown covers on_failure: stale reuses the page's last
// MP3 after its text changed, skip leaves the page to the browser button, and
// fail — or strict — stops the build.
func TestAudioWhenTheAPIIsDown(t *testing.T) {
	srv := newTTSServer(t)
	tmp := writeAudioSite(t)
	cacheRoot := filepath.Join(tmp, "cache")
	if _, err := buildAudioSite(t, tmp, AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: cacheRoot}, ListenOptions{}, false); err != nil {
		t.Fatal(err)
	}
	// The text changes, so the cache misses, and the API is down.
	post := filepath.Join(tmp, "content", "site", "posts", "news", "hello.md")
	mustWrite(t, post, strings.Replace(readFileString(t, post), "Spoken words.", "Edited words.", 1))
	srv.down.Store(true)

	out, err := buildAudioSite(t, tmp, AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: cacheRoot}, ListenOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if mp3 := readAudioOut(t, out, "audio/2026-09-30-hello.mp3"); mp3 != "SPEECH" {
		t.Errorf("stale: mp3 = %q, want the last good one", mp3)
	}

	out, err = buildAudioSite(t, tmp, AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: cacheRoot, OnFailure: "skip"},
		ListenOptions{Enabled: true, Label: "Read <aloud>"}, false)
	if err != nil {
		t.Fatal(err)
	}
	page := readAudioOut(t, out, "2026/09/30/hello/index.html")
	if strings.Contains(page, "<audio") || !strings.Contains(page, ">Read &lt;aloud&gt;</button>") || !strings.Contains(page, "data-ssg-listen-script") {
		t.Errorf("skip must leave the browser button and its script:\n%s", page)
	}

	for _, tc := range []struct {
		mode   string
		strict bool
	}{{"fail", false}, {"stale", true}} {
		_, err := buildAudioSite(t, tmp, AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: cacheRoot, OnFailure: tc.mode}, ListenOptions{}, tc.strict)
		if err == nil || !strings.Contains(err.Error(), "audio for") {
			t.Errorf("%s strict=%v: err = %v, want the build stopped", tc.mode, tc.strict, err)
		}
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAudioPagesAndJingleFailure: pages can be read too, and a jingle that
// cannot be fetched costs the intro, not the build.
func TestAudioPagesAndJingleFailure(t *testing.T) {
	srv := newTTSServer(t)
	tmp := writeAudioSite(t)
	out, err := buildAudioSite(t, tmp, AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: filepath.Join(tmp, "cache"),
		Sections: []string{"pages"}, Dir: "/listen/", JingleURL: "ftp://nope"}, ListenOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if mp3 := readAudioOut(t, out, "listen/about.mp3"); mp3 != "SPEECH" {
		t.Errorf("page mp3 = %q", mp3)
	}
	page := readAudioOut(t, out, "about/index.html")
	if !strings.Contains(page, `</h1>`+"\n"+`<audio class="ssg-audio"`) {
		t.Errorf("a theme without the helper gets the player after the title:\n%s", page)
	}
	if _, err := os.Stat(filepath.Join(out, "audio")); !os.IsNotExist(err) {
		t.Error("posts were not selected and must have no audio")
	}
}

func TestAudioHelpers(t *testing.T) {
	for url, want := range map[string]string{
		"/2026/09/30/Hello/": "2026-09-30-hello.mp3",
		"/":                  "index.mp3",
		"/docs/a b.html":     "docs-a-b.mp3",
	} {
		if got := audioName(models.Page{Link: url}); got != want {
			t.Errorf("audioName(%q) = %q, want %q", url, got, want)
		}
	}
	g, err := New(Config{Domain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	text := g.audioText(models.Page{Title: "T", Content: "A &amp; B.\n\n```\ncode()\n```\n"})
	if text != "T.\n\nA & B." {
		t.Errorf("audioText = %q", text)
	}
	if g.audioText(models.Page{Title: "T", Content: "```\nonly code\n```"}) != "" {
		t.Error("a page of only code has nothing to read")
	}
	if audioKey("a", "en", nil, "x") == audioKey("a", "pl", nil, "x") {
		t.Error("the language must change the key")
	}
	if g.generateAudio() != nil || g.generatePodcastFeed() != nil {
		t.Error("no client means no audio and no feed")
	}
}

func TestListenBlock(t *testing.T) {
	g := &Generator{config: Config{Listen: ListenOptions{Enabled: true}}, siteData: &models.SiteData{
		Posts: []models.Page{{Link: "/p/"}}}}
	post, page := models.Page{Link: "/p/"}, models.Page{Link: "/about/"}
	if got := string(g.listenBlock(post)); !strings.Contains(got, `aria-pressed="false" hidden>Listen</button>`) {
		t.Errorf("post button = %s", got)
	}
	if g.listenBlock(page) != "" {
		t.Error("pages are not in the default sections")
	}
	g.config.Listen.Sections = []string{"pages"}
	if g.listenBlock(page) == "" || g.listenBlock(post) != "" {
		t.Error("sections: pages selects pages only")
	}
	g.config.Listen.Sections = []string{"posts", "pages"}
	if g.listenBlock(page) == "" || g.listenBlock(post) == "" {
		t.Error("both sections select both")
	}
	html := g.listenHTMLString("<html><body><h1>T</h1><p>x</p></body></html>", &post)
	if !strings.Contains(html, "</h1>\n<button") || !strings.Contains(html, "data-ssg-listen-script") {
		t.Errorf("auto placement:\n%s", html)
	}
	if again := g.listenHTMLString(html, &post); again != html {
		t.Error("placement and script must not repeat")
	}
	g.config.Listen.Voice = `Google "UK"`
	if got := string(g.listenBlock(post)); !strings.Contains(got, `data-voice="Google &#34;UK&#34;"`) {
		t.Errorf("voice preference = %s", got)
	}
	g.config.Listen.NoAuto = true
	if got := g.listenHTMLString("<html><body><h1>T</h1></body></html>", &post); strings.Contains(got, "<button") {
		t.Errorf("auto: false must not place it: %s", got)
	}
	g.config.Listen.Enabled = false
	if g.listenBlock(post) != "" {
		t.Error("disabled means nothing")
	}
}
