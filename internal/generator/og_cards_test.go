package generator

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/ogcard"
)

// writeOGCardSite writes a post without an image, a page with one, and a
// theme that leaves the social tags to SEO injection.
func writeOGCardSite(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "posts", "news", "hello.md"),
		"---\ntitle: Zażółć gęślą jaźń\nslug: hello\nstatus: publish\ntype: post\ndate: 2026-09-30\ncategory: Poradniki\n---\n\nText.\n")
	mustWrite(t, filepath.Join(content, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\nfeatured_image: /media/about.jpg\n---\n\nAbout.\n")
	dir := filepath.Join(tmp, "templates", "simple")
	for _, name := range []string{"base.html", "index.html", "category.html"} {
		mustWrite(t, filepath.Join(dir, name), `{{define "`+name+`"}}<html><head><title>x</title></head><body>x</body></html>{{end}}`)
	}
	mustWrite(t, filepath.Join(dir, "post.html"), `{{define "post.html"}}<html><head><title>{{.Post.Title}}</title></head><body data-card="{{.Post.OGCard}}">{{.Post.Content | safeHTML}}</body></html>{{end}}`)
	mustWrite(t, filepath.Join(dir, "page.html"), `{{define "page.html"}}<html><head><title>{{.Page.Title}}</title></head><body>{{.Page.Content | safeHTML}}</body></html>{{end}}`)
	return tmp
}

func buildOGCardSite(t *testing.T, tmp string, opts OGCardOptions) (string, error) {
	t.Helper()
	out := filepath.Join(tmp, "output")
	if opts.CacheRoot == "" {
		opts.CacheRoot = filepath.Join(tmp, "cache")
	}
	// The colours a site must give; a test about them sets its own.
	if opts.Spec.Text == "" {
		opts.Spec.Text = "#FFFFFF"
	}
	if opts.Spec.Background == "" {
		opts.Spec.Background = "#202124"
	}
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com", SEO: true,
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: out, Quiet: true, OGCards: opts})
	if err != nil {
		t.Fatal(err)
	}
	return out, gen.Generate()
}

var ogCardURLRe = regexp.MustCompile(`<meta property="og:image" content="https://example\.com(/og/[^"]+\.png)">`)

func TestOGCardsBuild(t *testing.T) {
	tmp := writeOGCardSite(t)
	out, err := buildOGCardSite(t, tmp, OGCardOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	post := readAudioOut(t, out, "2026/09/30/hello/index.html")
	m := ogCardURLRe.FindStringSubmatch(post)
	if m == nil {
		t.Fatalf("no card og:image in post:\n%s", post)
	}
	card := m[1]
	for _, want := range []string{
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta name="twitter:image" content="https://example.com` + card + `">`,
		`"image":"https://example.com` + card + `"`,
		`data-card="` + card + `"`, // templates see .Post.OGCard
	} {
		if !strings.Contains(post, want) {
			t.Errorf("post lacks %s", want)
		}
	}
	if !strings.HasPrefix(card, "/og/2026-09-30-hello-") {
		t.Errorf("card name %q", card)
	}
	data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(card)))
	if err != nil {
		t.Fatal(err)
	}
	if img, err := png.Decode(bytes.NewReader(data)); err != nil || img.Bounds().Dx() != 1200 {
		t.Fatalf("card is not a 1200 px PNG: %v", err)
	}
	// A page with its own image keeps it, and gets no card.
	page := readAudioOut(t, out, "about/index.html")
	if !strings.Contains(page, `og:image" content="https://example.com/media/about.jpg"`) || strings.Contains(page, "og:image:width") {
		t.Errorf("page image changed:\n%s", page)
	}

	// Second build: the card in the output is left as it is.
	stat := func() int64 {
		fi, err := os.Stat(filepath.Join(out, filepath.FromSlash(card)))
		if err != nil {
			t.Fatal(err)
		}
		return fi.ModTime().UnixNano()
	}
	before := stat()
	if _, err := buildOGCardSite(t, tmp, OGCardOptions{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if stat() != before {
		t.Error("unchanged card was rewritten")
	}
	// A clean output takes the card from the cache, byte for byte.
	if err := os.RemoveAll(filepath.Join(out, "og")); err != nil {
		t.Fatal(err)
	}
	if _, err := buildOGCardSite(t, tmp, OGCardOptions{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(card)))
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("cached card differs: %v", err)
	}
}

func TestOGCardsOptionsAndFailures(t *testing.T) {
	tmp := writeOGCardSite(t)
	// JPEG in a custom directory, pages only: the post has no card.
	out, err := buildOGCardSite(t, tmp, OGCardOptions{Enabled: true, Dir: "/cards/", Sections: []string{"pages"},
		Spec: ogcard.Spec{Format: "jpeg", Width: 600, Height: 315, Background: "#777", Text: "#888"}})
	if err != nil {
		t.Fatal(err)
	}
	if post := readAudioOut(t, out, "2026/09/30/hello/index.html"); strings.Contains(post, "og:image") {
		t.Errorf("post outside sections got a card:\n%s", post)
	}

	for name, opts := range map[string]OGCardOptions{
		"bad colour":   {Enabled: true, Spec: ogcard.Spec{Background: "red"}},
		"missing font": {Enabled: true, Spec: ogcard.Spec{Font: filepath.Join(tmp, "none.ttf")}},
		"escapes":      {Enabled: true, Dir: "../outside"},
	} {
		if _, err := buildOGCardSite(t, writeOGCardSite(t), opts); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	// The card directory cannot be made: a file stands in its place.
	blocked := writeOGCardSite(t)
	mustWrite(t, filepath.Join(blocked, "output", "og"), "a file")
	if _, err := buildOGCardSite(t, blocked, OGCardOptions{Enabled: true}); err == nil {
		t.Error("unwritable card directory: no error")
	}
}

// TestOGCardsNeedTheSiteColours: `og_cards: true` alone draws nothing; the
// build stops and says which values to add.
func TestOGCardsNeedTheSiteColours(t *testing.T) {
	g := &Generator{config: Config{Quiet: true, OGCards: OGCardOptions{Enabled: true}}, siteData: &models.SiteData{}}
	err := g.generateOGCards()
	if err == nil || !strings.Contains(err.Error(), "text_color") {
		t.Fatalf("err = %v", err)
	}
}

func TestOGCardHelpers(t *testing.T) {
	g := &Generator{siteData: &models.SiteData{Categories: map[int]models.Category{7: {Name: "News"}}}}
	g.config.Domain = "example.com"
	if c := g.ogCardOf(models.Page{Title: "T", Categories: []int{7}}); c.Eyebrow != "News" || c.Footer != "example.com" {
		t.Errorf("card %+v", c)
	}
	g.siteData.Marketing.OGSiteName = "Brand"
	if f := g.ogCardFooter(); f != "Brand" {
		t.Errorf("footer %q", f)
	}
	g.siteData.Title = "Site"
	if f := g.ogCardFooter(); f != "Site" {
		t.Errorf("footer %q", f)
	}
	if f := (&Generator{siteData: &models.SiteData{}}).ogCardFooter(); f != "" {
		t.Errorf("empty footer %q", f)
	}
	if pageImage(models.Page{FeaturedImage: "/a.jpg", OGCard: "/og/b.png"}) != "/a.jpg" || pageImage(models.Page{OGCard: "/og/b.png"}) != "/og/b.png" {
		t.Error("pageImage precedence")
	}
	if ogCardKey("k", ogcard.Card{Title: "a"}) == ogCardKey("k", ogcard.Card{Title: "b"}) {
		t.Error("key ignores the title")
	}
	g.config.OGCards.Spec = ogcard.Spec{Width: 800, Height: 418}
	if m := g.ogCardSizeMeta(models.Page{OGCard: "/og/x.png"}); !strings.Contains(m, `"800"`) || !strings.Contains(m, `"418"`) {
		t.Errorf("size meta %q", m)
	}
	if g.ogCardSizeMeta(models.Page{FeaturedImage: "/a.jpg", OGCard: "/og/x.png"}) != "" || g.ogCardSizeMeta(models.Page{}) != "" {
		t.Error("size meta without a card")
	}
	if (OGCardOptions{}).OGCardDir() != "og" || (OGCardOptions{Dir: "/x/y/"}).OGCardDir() != "x/y" {
		t.Error("card dir")
	}
}

// TestOGCardEncodeFailure: a JPEG wider than the format allows fails the
// build with the page named, instead of publishing a broken card.
func TestOGCardEncodeFailure(t *testing.T) {
	tmp := writeOGCardSite(t)
	_, err := buildOGCardSite(t, tmp, OGCardOptions{Enabled: true, Sections: []string{"posts"},
		Spec: ogcard.Spec{Format: "jpeg", Width: 70000, Height: 1}})
	if err == nil || !strings.Contains(err.Error(), "drawing the card of") {
		t.Fatalf("err = %v", err)
	}
}
