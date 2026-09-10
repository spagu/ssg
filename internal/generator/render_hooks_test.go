package generator

// Render hooks: a template decides the markup for one kind of node (GO-099).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hookFiles are the six templates, each doing the thing its hook exists for.
var hookFiles = map[string]string{
	"image.html": `<figure class="md-image"><img src="{{ .Src }}" alt="{{ .Alt }}" loading="lazy"` +
		`{{ if .IsExternal }} data-external="1"{{ end }}>{{ with .Title }}<figcaption>{{ . }}</figcaption>{{ end }}</figure>`,
	"link.html": `{{ if .IsExternal }}<a href="{{ .Href }}" rel="noopener noreferrer">{{ .Text }}</a>` +
		`{{ else }}<a href="{{ .Href }}">{{ .Text }}</a>{{ end }}`,
	"heading.html":    `<h{{ .Level }} id="{{ .ID }}">{{ .Text }} <a class="anchor" href="#{{ .ID }}">¶</a></h{{ .Level }}>`,
	"code.html":       `<div class="code" data-lang="{{ .Lang }}" data-info="{{ .Info }}">{{ .Rendered }}</div>`,
	"table.html":      `<div class="table-scroll">{{ .Inner }}</div>`,
	"blockquote.html": `<blockquote class="quote">{{ .Inner }}</blockquote>`,
}

// hookSite builds a site with the named hooks wired up.
func hookSite(t *testing.T, body string, hooks ...string) string {
	t.Helper()
	dir := t.TempDir()
	config := map[string]string{}
	for _, name := range hooks {
		file := name + ".html"
		path := filepath.Join(dir, file)
		mustWrite(t, path, hookFiles[file])
		config[name] = path
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/demo.md": "---\ntitle: Demo\nslug: demo\nstatus: publish\ntype: page\n---\n\n" + body,
	}, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.RenderHooks = config
	buildSiteFixture(t, cfg)
	return mustRead(t, filepath.Join(cfg.OutputDir, "demo", "index.html"))
}

// TestImageHookReachesContentImages: the whole point. An image written in
// Markdown could not get lazy loading, a caption or a srcset, because the
// responsive pipeline was reachable only from templates.
func TestImageHookReachesContentImages(t *testing.T) {
	got := hookSite(t, "![A blue widget](/img/widget.jpg \"The caption\")\n", "image")
	if !strings.Contains(got, `<img src="/img/widget.jpg" alt="A blue widget" loading="lazy">`) {
		t.Errorf("the hook did not render the image:\n%s", got)
	}
	if !strings.Contains(got, "<figcaption>The caption</figcaption>") {
		t.Errorf("the Markdown title did not reach the hook:\n%s", got)
	}
	// A lone image is not left inside a paragraph, because a <figure> in a <p>
	// is markup no browser agrees about.
	if strings.Contains(got, "<p><figure") {
		t.Errorf("the figure is still wrapped in a paragraph:\n%s", got)
	}
}

// TestImageInProseKeepsItsParagraph: only a paragraph that is nothing but an
// image is unwrapped.
func TestImageInProseKeepsItsParagraph(t *testing.T) {
	got := hookSite(t, "Words around ![alt](/a.jpg) an image.\n", "image")
	if !strings.Contains(got, "<p>Words around <figure") {
		t.Errorf("a paragraph with prose in it must survive:\n%s", got)
	}
}

// TestLinkHookSeesWhereALinkGoes, and keeps the markup inside it.
func TestLinkHookSeesWhereALinkGoes(t *testing.T) {
	got := hookSite(t,
		"A [local](/about/) and an [external](https://example.org/x) and a [**bold** one](/b/).\n", "link")
	if !strings.Contains(got, `<a href="https://example.org/x" rel="noopener noreferrer">external</a>`) {
		t.Errorf("the external link got no rel:\n%s", got)
	}
	if !strings.Contains(got, `<a href="/about/">local</a>`) {
		t.Errorf("the internal link was changed:\n%s", got)
	}
	if !strings.Contains(got, `<a href="/b/"><strong>bold</strong> one</a>`) {
		t.Errorf("markup inside the link was flattened:\n%s", got)
	}
}

// TestOwnDomainIsNotExternal: an absolute URL on the site's own domain is a
// link home, whatever it looks like.
func TestOwnDomainIsNotExternal(t *testing.T) {
	h := &hookSet{domain: "example.com"}
	internal := []string{"", "#top", "/about/", "about/", "mailto:a@example.com", "tel:+1",
		"https://example.com/x", "http://example.com", "//example.com/y"}
	for _, dest := range internal {
		if h.isExternal(dest) {
			t.Errorf("%q should be internal", dest)
		}
	}
	for _, dest := range []string{"https://example.org/x", "//other.example/y", "http://sub.example.com/"} {
		if !h.isExternal(dest) {
			t.Errorf("%q should be external", dest)
		}
	}
	// With no domain configured, an absolute URL is external — the safe reading.
	bare := &hookSet{}
	if !bare.isExternal("https://example.com/x") || bare.isExternal("/x") {
		t.Error("a site with no domain should treat absolute URLs as external and paths as not")
	}
}

// TestHeadingHookGetsTheIDTheBuildComputed, so an anchor a hook writes and the
// table of contents cannot disagree.
func TestHeadingHookGetsTheIDTheBuildComputed(t *testing.T) {
	got := hookSite(t, "## A heading with **markup**\n", "heading")
	if !strings.Contains(got, `<h2 id="a-heading-with-markup">A heading with <strong>markup</strong>`) {
		t.Errorf("heading hook:\n%s", got)
	}
	if !strings.Contains(got, `<a class="anchor" href="#a-heading-with-markup">`) {
		t.Errorf("the anchor did not get the id:\n%s", got)
	}
}

// TestCodeHookWrapsRatherThanReplaces: re-implementing the highlighter in a
// template is not what anyone wants from this.
func TestCodeHookWrapsRatherThanReplaces(t *testing.T) {
	got := hookSite(t, "```go title=\"main.go\"\nfunc main() {}\n```\n", "code")
	if !strings.Contains(got, `<div class="code" data-lang="go"`) {
		t.Errorf("the wrapper is missing:\n%s", got)
	}
	if !strings.Contains(got, `data-info="go title=&#34;main.go&#34;"`) {
		t.Errorf("the info string did not reach the hook:\n%s", got)
	}
	if !strings.Contains(got, "<pre><code") || !strings.Contains(got, "func main() {}") {
		t.Errorf("the block itself was replaced rather than wrapped:\n%s", got)
	}
}

// TestContainerHooksKeepTheirContent.
func TestContainerHooksKeepTheirContent(t *testing.T) {
	got := hookSite(t, "| a | b |\n|---|---|\n| 1 | 2 |\n\n> Quoted **text**.\n", "table", "blockquote")
	if !strings.Contains(got, `<div class="table-scroll"><table>`) || !strings.Contains(got, "<td>1</td>") {
		t.Errorf("table hook:\n%s", got)
	}
	if !strings.Contains(got, `<blockquote class="quote"><p>Quoted <strong>text</strong>.</p>`) {
		t.Errorf("blockquote hook:\n%s", got)
	}
}

// TestNoHooksChangesNothing: the hard back-compat condition. A build without
// hooks registers no renderer and gets goldmark's own markup.
func TestNoHooksChangesNothing(t *testing.T) {
	body := "## Heading\n\n![alt](/a.jpg)\n\n[link](/x) and [out](https://example.org)\n\n" +
		"```go\nx := 1\n```\n\n> quote\n\n| a |\n|---|\n| 1 |\n"
	plain := hookSite(t, body)
	for _, want := range []string{"<h2 id=\"heading\">Heading</h2>", `<p><img src="/a.jpg" alt="alt"></p>`,
		`<a href="/x">link</a>`, "<blockquote>", "<table>"} {
		if !strings.Contains(plain, want) {
			t.Errorf("goldmark's own markup changed — missing %q:\n%s", want, plain)
		}
	}
	for _, unwanted := range []string{"md-image", "table-scroll", "noopener", "anchor"} {
		if strings.Contains(plain, unwanted) {
			t.Errorf("a hook ran without being configured: %q", unwanted)
		}
	}
}

// TestOneHookLeavesTheOthersAlone: a set claims only the kinds it has
// templates for.
func TestOneHookLeavesTheOthersAlone(t *testing.T) {
	got := hookSite(t, "## Heading\n\n![alt](/a.jpg)\n\n[link](/x)\n", "image")
	if !strings.Contains(got, `class="md-image"`) {
		t.Errorf("the image hook did not run:\n%s", got)
	}
	if !strings.Contains(got, `<h2 id="heading">Heading</h2>`) || !strings.Contains(got, `<a href="/x">link</a>`) {
		t.Errorf("an unhooked kind changed:\n%s", got)
	}
}

// TestHookConfigErrors: a hook the site asked for by name and cannot have is
// an error at load time, not a silent no-op.
func TestHookConfigErrors(t *testing.T) {
	base := func(hooks map[string]string) error {
		cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
			"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
		}, nil)
		cfg.RenderHooks = hooks
		return mustBuild(t, cfg)
	}
	if err := base(map[string]string{"image": filepath.Join(t.TempDir(), "gone.html")}); err == nil ||
		!strings.Contains(err.Error(), "render_hooks.image") {
		t.Errorf("a missing template: %v", err)
	}
	broken := filepath.Join(t.TempDir(), "broken.html")
	mustWrite(t, broken, "{{ .Src")
	if err := base(map[string]string{"image": broken}); err == nil || !strings.Contains(err.Error(), "render_hooks.image") {
		t.Errorf("an unparsable template: %v", err)
	}
	ok := filepath.Join(t.TempDir(), "ok.html")
	mustWrite(t, ok, "<img>")
	if err := base(map[string]string{"footnote": ok}); err == nil || !strings.Contains(err.Error(), "is not a hook") {
		t.Errorf("an unknown hook name: %v", err)
	}
	// An empty path is not a hook and is not an error either.
	if err := base(map[string]string{"image": ""}); err != nil {
		t.Errorf("an empty path should be ignored: %v", err)
	}
}

// TestHookThatFailsAtRenderTimeIsReported rather than quietly writing markup
// that is almost right.
func TestHookThatFailsAtRenderTimeIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.html")
	mustWrite(t, path, "{{ len .Missing }}")
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\n![x](/a.jpg)\n",
	}, nil)
	cfg.RenderHooks = map[string]string{"image": path}
	// The conversion fails; the build reports it rather than shipping the page.
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		return // reported, which is the point
	}
	got := mustRead(t, filepath.Join(cfg.OutputDir, "a", "index.html"))
	if strings.Contains(got, "<img") {
		t.Errorf("a broken hook fell back to goldmark's markup:\n%s", got)
	}
}

// TestKnownHookNames keeps the list and the docs honest.
func TestKnownHookNames(t *testing.T) {
	for _, name := range []string{"image", "link", "heading", "code", "table", "blockquote"} {
		if !knownHook(name) {
			t.Errorf("%q should be a hook", name)
		}
	}
	for _, name := range []string{"", "images", "paragraph", "emphasis"} {
		if knownHook(name) {
			t.Errorf("%q should not be a hook", name)
		}
	}
	if len(hookNames) != 6 {
		t.Errorf("hookNames = %v", hookNames)
	}
}

// TestHookSetIsNilSafe: every path a build takes without hooks.
func TestHookSetIsNilSafe(t *testing.T) {
	var h *hookSet
	if h.hasImageHook() {
		t.Error("a nil set has no image hook")
	}
	h.RegisterFuncs(nil) // must not panic
	if got := hookTransformers(nil); len(got) != 1 {
		t.Errorf("transformers = %v", got)
	}
	if got := hookRendererOption(nil); got != nil {
		t.Errorf("options = %v", got)
	}
	if got := hookRendererOption(&hookSet{}); got != nil {
		t.Error("an empty set registers nothing")
	}
}

// TestHooksSurviveARebuild, which is what a watch loop does.
func TestHooksSurviveARebuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.html")
	mustWrite(t, path, hookFiles["image.html"])
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\n![x](/a.jpg)\n",
	}, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>x</title></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.RenderHooks = map[string]string{"image": path}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := gen.Generate(); err != nil {
			t.Fatalf("build %d: %v", i+1, err)
		}
		if got := mustRead(t, filepath.Join(cfg.OutputDir, "a", "index.html")); !strings.Contains(got, "md-image") {
			t.Errorf("build %d lost the hook:\n%s", i+1, got)
		}
	}
	_ = os.Remove(path)
}

// TestImageWithSiblingsStaysInItsParagraph, and an empty paragraph is left
// alone: the unwrap is for a lone image, and nothing else.
func TestImageWithSiblingsStaysInItsParagraph(t *testing.T) {
	got := hookSite(t, "![a](/a.jpg) ![b](/b.jpg)\n", "image")
	if !strings.Contains(got, "<p><figure") {
		t.Errorf("two images in one paragraph should keep it:\n%s", got)
	}
	if strings.Count(got, "md-image") != 2 {
		t.Errorf("both images should render through the hook:\n%s", got)
	}
}

// TestExternalImageIsRecognised: a hook can treat a remote image differently,
// which is the point of putting IsExternal in its context.
func TestExternalImageIsRecognised(t *testing.T) {
	got := hookSite(t, "![remote](https://example.org/x.jpg)\n", "image")
	if !strings.Contains(got, `data-external="1"`) {
		t.Errorf("a remote image was not marked external:\n%s", got)
	}
}

// TestHookOutputIsNotEscaped: a hook writes markup, and .Text and .Inner carry
// markup into it. Neither may arrive as escaped text.
func TestHookOutputIsNotEscaped(t *testing.T) {
	got := hookSite(t, "> A quote with a [link](/x) in it.\n", "blockquote")
	if strings.Contains(got, "&lt;a href") {
		t.Errorf("the rendered inner content was escaped:\n%s", got)
	}
	if !strings.Contains(got, `<blockquote class="quote"><p>A quote with a <a href="/x">link</a>`) {
		t.Errorf("blockquote:\n%s", got)
	}
}

// TestNestedHooksDoNotApplyInsideRenderedContent, which is the documented
// price of not recursing forever.
func TestNestedHooksDoNotApplyInsideRenderedContent(t *testing.T) {
	got := hookSite(t, "> A quote with an ![image](/a.jpg) in it.\n", "blockquote", "image")
	if !strings.Contains(got, `<blockquote class="quote">`) {
		t.Errorf("blockquote hook did not run:\n%s", got)
	}
	// The image inside is rendered by goldmark, not by the image hook.
	if strings.Contains(got, "md-image") {
		t.Errorf("a hook ran inside another hook's content:\n%s", got)
	}
}
