package generator

// What a page carries in edit mode, and what it does not (GO-102).

import (
	"strings"
	"testing"
)

// TestEditAttrsAreStrippedFromAPublishedPage: a theme marks its editable
// regions once, and a published build takes the marks back out — byte for
// byte, which is what lets the themes carry them at all.
func TestEditAttrsAreStrippedFromAPublishedPage(t *testing.T) {
	plain := `<h1 class="post-title">Hello</h1><time datetime="2026-01-15">x</time>`
	marked := `<h1 class="post-title" data-ssg-edit="frontmatter:title">Hello</h1>` +
		`<time data-ssg-edit="frontmatter:date" datetime="2026-01-15">x</time>`
	if got := stripEditAttrs(marked); got != plain {
		t.Errorf("stripping did not restore the original:\n got %q\nwant %q", got, plain)
	}
	// A page with no marks is returned as it came, without a rewrite.
	if got := stripEditAttrs(plain); got != plain {
		t.Errorf("an unmarked page changed: %q", got)
	}
}

// TestEditSourceMeta names the document, and says nothing when there is none —
// a listing has no single source file, which is how the editor knows not to
// offer a form for it.
func TestEditSourceMeta(t *testing.T) {
	got := editSourceMeta("content/site/posts/news", "hello.md", "post")
	if !strings.Contains(got, `content="content/site/posts/news/hello.md"`) {
		t.Errorf("got %q", got)
	}
	if !strings.Contains(got, `name="ssg:type" content="post"`) {
		t.Errorf("type missing: %q", got)
	}
	if editSourceMeta("", "", "post") != "" {
		t.Error("a page with no source file must carry no marker")
	}
	if got := editSourceMeta("", "hello.md", "page"); !strings.Contains(got, `content="hello.md"`) {
		t.Errorf("a file with no directory: %q", got)
	}
	// A path that could close the attribute is escaped.
	if got := editSourceMeta(`a"><script>`, "x.md", "post"); strings.Contains(got, "<script>") {
		t.Errorf("unescaped path: %q", got)
	}
}

// TestInjectEditSource places the marker in the head, or appends it when the
// template has no head to speak of.
func TestInjectEditSource(t *testing.T) {
	meta := editSourceMeta("content", "x.md", "page")
	page := "<html><head><title>x</title></head><body>y</body></html>"
	got := injectEditSource(page, meta)
	if !strings.Contains(got, "ssg:source") || strings.Index(got, "ssg:source") > strings.Index(got, "</head>") {
		t.Errorf("marker not in the head:\n%s", got)
	}
	if injectEditSource(page, "") != page {
		t.Error("no marker, no change")
	}
	if got := injectEditSource("<p>bare</p>", meta); got != "<p>bare</p>" {
		t.Errorf("a document with no head is left alone: %q", got)
	}
}

// TestEditModeChangesNothingWhenOff: a build without edit mode carries neither
// the marker nor the theme's attributes, which is the promise the golden
// corpora check at a larger scale.
func TestEditModeChangesNothingWhenOff(t *testing.T) {
	files := map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}
	marked := func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>{{ .Title }}</title></head><body>` +
				`<h1 data-ssg-edit="frontmatter:title">{{ .Title }}</h1>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	}
	off := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, marked)
	buildSiteFixture(t, off)
	published := mustRead(t, off.OutputDir+"/about/index.html")
	if strings.Contains(published, "data-ssg-edit") || strings.Contains(published, "ssg:source") {
		t.Errorf("editing scaffolding reached a published page:\n%s", published)
	}

	on := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, marked)
	on.EditMode = true
	buildSiteFixture(t, on)
	editable := mustRead(t, on.OutputDir+"/about/index.html")
	if !strings.Contains(editable, "data-ssg-edit") {
		t.Errorf("edit mode dropped the theme's attributes:\n%s", editable)
	}
	if !strings.Contains(editable, `name="ssg:source"`) {
		t.Errorf("edit mode did not name the source document:\n%s", editable)
	}
	if !strings.Contains(editable, "about.md") {
		t.Errorf("the marker does not name the file:\n%s", editable)
	}
}
