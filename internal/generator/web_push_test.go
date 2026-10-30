package generator

// The static half of Web Push (#368).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func webPushGen(t *testing.T, cfg Config) *Generator {
	t.Helper()
	cfg.Domain = "example.com"
	if cfg.OutputDir == "" {
		cfg.OutputDir = t.TempDir()
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	g.siteData = &models.SiteData{}
	return g
}

const pushDoc = "<html><head><title>x</title></head><body></body></html>"

func TestInjectWebPush(t *testing.T) {
	off := webPushGen(t, Config{})
	if got := off.injectWebPush(pushDoc); got != pushDoc {
		t.Errorf("off must not inject: %s", got)
	}
	g := webPushGen(t, Config{WebPush: WebPushOptions{Enabled: true}})
	got := g.injectWebPush(pushDoc)
	for _, want := range []string{
		`<link rel="manifest" href="/manifest.webmanifest">`,
		`<script src="/push.js" defer data-endpoint="/api/push"></script>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if !strings.HasSuffix(got, "</head><body></body></html>") {
		t.Errorf("must go before </head>: %s", got)
	}
	if g.injectWebPush("<p>fragment</p>") != "<p>fragment</p>" {
		t.Error("no <head>, nothing injected")
	}
	// What the theme already has is left alone.
	theme := `<html><head><link rel="manifest" href="/app.webmanifest"><script src="/js/push.js"></script></head></html>`
	if g.injectWebPush(theme) != theme {
		t.Errorf("theme's own manifest and script must win: %s", g.injectWebPush(theme))
	}
}

func TestInjectWebPushBasePathAndEndpoint(t *testing.T) {
	g := webPushGen(t, Config{BasePath: "/docs", WebPush: WebPushOptions{Enabled: true, Endpoint: "/hooks/push"}})
	got := g.injectWebPush(pushDoc)
	for _, want := range []string{`href="/docs/manifest.webmanifest"`, `src="/docs/push.js"`, `data-endpoint="/hooks/push"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestWriteWebPushManifest(t *testing.T) {
	out := t.TempDir()
	g := webPushGen(t, Config{OutputDir: out, BasePath: "/docs", WebPush: WebPushOptions{Enabled: true}})
	g.siteData.Title = "Example Blog"
	g.siteData.Marketing = models.Marketing{ThemeColor: "#0b57d0", AppleTouchIcon: "/apple.png", Favicon: "/favicon.ico"}
	mustWrite(t, filepath.Join(out, "push.js"), "x")
	if err := g.writeWebPush(); err != nil {
		t.Fatal(err)
	}
	var m webManifest
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(out, webManifestName))), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "Example Blog" || m.StartURL != "/docs/" || m.Scope != "/docs/" || m.Display != "standalone" ||
		m.ThemeColor != "#0b57d0" || len(m.Icons) != 2 || m.Icons[0].Src != "/apple.png" {
		t.Errorf("manifest = %+v", m)
	}
}

func TestWriteWebPushKeepsSiteManifest(t *testing.T) {
	out := t.TempDir()
	g := webPushGen(t, Config{OutputDir: out, WebPush: WebPushOptions{Enabled: true}})
	mustWrite(t, filepath.Join(out, webManifestName), "own")
	if err := g.writeWebPush(); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, filepath.Join(out, webManifestName)); got != "own" {
		t.Errorf("site's own manifest overwritten: %s", got)
	}
}

func TestWriteWebPushOffAndDomainName(t *testing.T) {
	out := t.TempDir()
	if err := webPushGen(t, Config{OutputDir: out}).writeWebPush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, webManifestName)); err == nil {
		t.Error("off must write nothing")
	}
	g := webPushGen(t, Config{OutputDir: out, WebPush: WebPushOptions{Enabled: true}})
	if m := g.webManifest(); m.Name != "example.com" || m.StartURL != "/" || m.Icons != nil {
		t.Errorf("untitled site: %+v", m)
	}
}

func TestWriteWebPushUnwritable(t *testing.T) {
	g := webPushGen(t, Config{OutputDir: filepath.Join(t.TempDir(), "missing"), WebPush: WebPushOptions{Enabled: true}})
	if err := g.writeWebPush(); err == nil {
		t.Error("a missing output directory must fail")
	}
}

// TestWebPushFullBuild: a build with web_push writes the manifest and puts the
// link and script in the pages it renders.
func TestWebPushFullBuild(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	cfg.WebPush = WebPushOptions{Enabled: true}
	mustWrite(t, filepath.Join(tmp, "templates", "simple", "taxonomy.html"),
		`{{define "taxonomy.html"}}<html><head><title>{{.Taxonomy.Label}}</title></head><body></body></html>{{end}}`)
	generateSite(t, cfg)
	out := filepath.Join(tmp, "output")
	wantFiles(t, filepath.Join(out, webManifestName))
	wantContains(t, "taxonomy index", mustRead(t, filepath.Join(out, "technology", "index.html")),
		`<link rel="manifest" href="/manifest.webmanifest">`, `<script src="/push.js" defer`)
}
