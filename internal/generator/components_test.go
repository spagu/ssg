package generator

// Typed content components in a real build (GO-093).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// componentSite builds a site with a component library beside it.
func componentSite(t *testing.T, files map[string]string, apply func(cfg *Config)) Config {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "youtube", "component.yaml"), `
description: Embed a video.
props:
  id: {type: string, required: true}
  ratio: {type: string, enum: [16x9, 4x3], default: 16x9}
`)
	mustWrite(t, filepath.Join(dir, "youtube", "template.html"),
		`<figure class="yt yt--{{ .Props.ratio }}"><iframe src="/e/{{ .Props.id }}"></iframe></figure>`)
	mustWrite(t, filepath.Join(dir, "youtube", "assets", "youtube.css"), ".yt{margin:0}")
	mustWrite(t, filepath.Join(dir, "youtube", "assets", "youtube.js"), "//yt")
	mustWrite(t, filepath.Join(dir, "note", "template.html"), `<aside>{{ .Props.text }} — {{ .Site.Domain }}</aside>`)

	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files,
		func(name string) string {
			if name == "page.html" || name == "post.html" {
				return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
			}
			return `<html><head><title>x</title></head><body><p>x</p></body></html>`
		})
	cfg.ComponentsDir = dir
	if apply != nil {
		apply(&cfg)
	}
	return cfg
}

// TestComponentsRenderInContent, with typed props and defaults, and the site in
// scope.
func TestComponentsRenderInContent(t *testing.T) {
	cfg := componentSite(t, map[string]string{
		"pages/demo.md": "---\ntitle: Demo\nslug: demo\nstatus: publish\ntype: page\n---\n\n" +
			"{{< youtube id=\"abc\" ratio=\"4x3\" >}}\n\n{{< note text=\"careful\" >}}\n",
		"pages/plain.md": "---\ntitle: Plain\nslug: plain\nstatus: publish\ntype: page\n---\n\nNothing here.\n",
	}, nil)
	buildSiteFixture(t, cfg)

	demo := mustRead(t, filepath.Join(cfg.OutputDir, "demo", "index.html"))
	if !strings.Contains(demo, `class="yt yt--4x3"`) || !strings.Contains(demo, `src="/e/abc"`) {
		t.Errorf("component did not render:\n%s", demo)
	}
	if !strings.Contains(demo, "careful — example.com") {
		t.Errorf("the site was not in scope:\n%s", demo)
	}
	if strings.Contains(demo, "{{<") {
		t.Errorf("a call survived:\n%s", demo)
	}
	// The marker is bookkeeping and does not ship.
	if strings.Contains(demo, componentMarkerAttr) {
		t.Errorf("the marker reached the page:\n%s", demo)
	}
	// Assets: linked on the page that used them, copied once, absent elsewhere.
	if !strings.Contains(demo, `href="/components/youtube/youtube.css"`) ||
		!strings.Contains(demo, `src="/components/youtube/youtube.js"`) {
		t.Errorf("component assets were not linked:\n%s", demo)
	}
	plain := mustRead(t, filepath.Join(cfg.OutputDir, "plain", "index.html"))
	if strings.Contains(plain, "/components/") {
		t.Errorf("a page that used nothing links a component's assets:\n%s", plain)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "components", "youtube", "youtube.css")); err != nil {
		t.Errorf("asset not copied: %v", err)
	}
	// The `note` component has no assets, so nothing was copied for it.
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "components", "note")); err == nil {
		t.Error("a component with no assets should leave no directory")
	}
	// The manifest publishes the contract.
	manifest := mustRead(t, filepath.Join(cfg.OutputDir, "components.json"))
	if !strings.Contains(manifest, `"name": "youtube"`) || !strings.Contains(manifest, `{{< youtube id=`) {
		t.Errorf("manifest:\n%s", manifest)
	}
}

// TestComponentErrorsFollowShortcodePolicy: one setting for "my content is
// wrong", not two.
func TestComponentErrorsFollowShortcodePolicy(t *testing.T) {
	files := map[string]string{
		"pages/bad.md": "---\ntitle: Bad\nslug: bad\nstatus: publish\ntype: page\n---\n\n{{< youtube >}}\n",
	}
	// keep leaves the call in the page for the author to see.
	kept := componentSite(t, files, func(cfg *Config) { cfg.ShortcodeErrors = "keep" })
	buildSiteFixture(t, kept)
	if got := mustRead(t, filepath.Join(kept.OutputDir, "bad", "index.html")); !strings.Contains(got, "youtube") {
		t.Errorf("keep should leave the call visible:\n%s", got)
	}

	// drop takes it out, and an unset shortcode_errors means drop.
	for _, policy := range []string{"drop", ""} {
		dropped := componentSite(t, files, func(cfg *Config) { cfg.ShortcodeErrors = policy })
		buildSiteFixture(t, dropped)
		if got := mustRead(t, filepath.Join(dropped.OutputDir, "bad", "index.html")); strings.Contains(got, "youtube") {
			t.Errorf("%q should remove the call:\n%s", policy, got)
		}
	}

	// A call naming a component the site does not have is text, not a broken
	// call: it survives every policy, because documentation quoting a call is
	// the ordinary case.
	quoting := componentSite(t, map[string]string{
		"pages/docs.md": "---\ntitle: Docs\nslug: docs\nstatus: publish\ntype: page\n---\n\n" +
			"Write {{< gallery src=\"photos/\" >}} to embed one.\n",
	}, func(cfg *Config) { cfg.ShortcodeErrors = "strict" })
	if err := mustBuild(t, quoting); err != nil {
		t.Fatalf("quoting a call must not fail a strict build: %v", err)
	}
	if got := mustRead(t, filepath.Join(quoting.OutputDir, "docs", "index.html")); !strings.Contains(got, "gallery") {
		t.Errorf("the quoted call was eaten:\n%s", got)
	}

	// strict fails the build, naming the component and the call.
	strict := componentSite(t, files, func(cfg *Config) { cfg.ShortcodeErrors = "strict" })
	gen, err := New(strict)
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), "youtube") || !strings.Contains(err.Error(), "{{<") {
		t.Errorf("strict: %v", err)
	}
}

// TestASiteWithoutComponentsIsUnchanged: the feature is opt-in by the presence
// of a directory, and content that merely looks like a call is left alone.
func TestASiteWithoutComponentsIsUnchanged(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/demo.md": "---\ntitle: Demo\nslug: demo\nstatus: publish\ntype: page\n---\n\n" +
			"A page that writes {{< youtube id=\"x\" >}} as an example.\n",
	}, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.ComponentsDir = filepath.Join(t.TempDir(), "no-components-here")
	buildSiteFixture(t, cfg)
	got := mustRead(t, filepath.Join(cfg.OutputDir, "demo", "index.html"))
	if !strings.Contains(got, "youtube") {
		t.Errorf("the text was eaten:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "components.json")); err == nil {
		t.Error("a site with no components should publish no manifest")
	}
}

// TestComponentOutputSurvivesTheSanitizer, while a prop from content does not
// get to bring markup with it.
func TestComponentOutputSurvivesTheSanitizer(t *testing.T) {
	cfg := componentSite(t, map[string]string{
		"pages/demo.md": "---\ntitle: Demo\nslug: demo\nstatus: publish\ntype: page\n---\n\n" +
			"{{< youtube id=\"abc\" >}}\n\n{{< note text=\"<script>alert(1)</script>\" >}}\n",
	}, func(cfg *Config) { cfg.SanitizeHTML = true })
	buildSiteFixture(t, cfg)
	got := mustRead(t, filepath.Join(cfg.OutputDir, "demo", "index.html"))
	if !strings.Contains(got, "<iframe") {
		t.Errorf("the sanitizer removed the component's own markup:\n%s", got)
	}
	if strings.Contains(got, "<script>alert(1)") {
		t.Errorf("a prop carried markup into the page:\n%s", got)
	}
}

// TestMarkerHelpers: what goes on, and what comes back off.
func TestMarkerHelpers(t *testing.T) {
	got := markComponent(`<figure class="yt">x</figure>`, "youtube")
	if got != `<figure data-ssg-component="youtube" class="yt">x</figure>` {
		t.Errorf("marked = %q", got)
	}
	if stripComponentMarkers(got) != `<figure class="yt">x</figure>` {
		t.Errorf("stripping did not restore the original: %q", stripComponentMarkers(got))
	}
	// Output that does not start with an element gets no marker, and therefore
	// no per-page assets — the documented reason to give a component with a
	// stylesheet an element.
	if got := markComponent("just text", "youtube"); got != "just text" {
		t.Errorf("markComponent = %q", got)
	}
	// A page with no markers is returned untouched, without a rewrite.
	plain := "<p>nothing here</p>"
	if stripComponentMarkers(plain) != plain {
		t.Error("an unmarked page changed")
	}
}

// TestComponentBookkeepingResetsBetweenBuilds, so a watch rebuild reports this
// build rather than every one this process has rendered.
func TestComponentBookkeepingResetsBetweenBuilds(t *testing.T) {
	cfg := componentSite(t, map[string]string{
		"pages/demo.md": "---\ntitle: Demo\nslug: demo\nstatus: publish\ntype: page\n---\n\n{{< youtube id=\"a\" >}}\n",
	}, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	if got := gen.usedComponents(); len(got) != 1 || got[0] != "youtube" {
		t.Errorf("used = %v", got)
	}
	gen.resetComponents()
	if len(gen.usedComponents()) != 0 || gen.componentError() != nil {
		t.Error("the record should be empty after a reset")
	}
}

// mustBuild runs one build and returns its error, for a test that expects one
// way or the other.
func mustBuild(t *testing.T, cfg Config) error {
	t.Helper()
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return gen.Generate()
}

// TestComponentLoadErrorsFailTheBuild: a components directory the author meant
// and got wrong is not something to build past.
func TestComponentLoadErrorsFailTheBuild(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "broken", "component.yaml"), "props:\n  a: {type: colour}\n")
	mustWrite(t, filepath.Join(dir, "broken", "template.html"), "<p>x</p>")
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
	}, nil)
	cfg.ComponentsDir = dir
	err := mustBuild(t, cfg)
	if err == nil || !strings.Contains(err.Error(), "colour") {
		t.Errorf("a malformed schema must fail the build: %v", err)
	}
}

// TestComponentAssetsReportAnUnwritableOutput rather than shipping a page that
// links a stylesheet nobody copied.
func TestComponentAssetsReportAnUnwritableOutput(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	cfg := componentSite(t, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\n{{< youtube id=\"x\" >}}\n",
	}, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	// Point the same build at a directory it cannot write into: the copy fails
	// and says which asset it was.
	locked := t.TempDir()
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	gen.config.OutputDir = locked
	err = gen.writeComponentAssets()
	if err == nil || !strings.Contains(err.Error(), "youtube.css") {
		t.Errorf("an asset that cannot be copied must be reported: %v", err)
	}
	// And a manifest that cannot be written is reported too.
	gen.componentsUsed = nil
	if err := gen.writeComponentAssets(); err == nil {
		t.Error("a manifest that cannot be written must be reported")
	}
}

// TestComponentAssetTagsAreNotDuplicated when a theme already links them.
func TestComponentAssetTagsAreNotDuplicated(t *testing.T) {
	cfg := componentSite(t, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
	}, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	marked := `<html><head><link rel="stylesheet" href="/components/youtube/youtube.css"></head>` +
		`<body><figure data-ssg-component="youtube">x</figure></body></html>`
	if got := gen.componentAssetTags(marked); strings.Contains(got, "youtube.css") {
		t.Errorf("a stylesheet the page already links should not be added again: %q", got)
	}
	// A page with no markers needs nothing, and a document with no head still
	// gets its tags rather than losing them.
	if got := gen.componentAssetTags("<p>plain</p>"); got != "" {
		t.Errorf("tags = %q", got)
	}
	headless := gen.injectComponentAssets(`<figure data-ssg-component="youtube">x</figure>`)
	if !strings.Contains(headless, "youtube.css") || strings.Contains(headless, componentMarkerAttr) {
		t.Errorf("headless = %q", headless)
	}
}
