package generator

// Feed autodiscovery on a multilingual site (#282).

import (
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
)

// discoveryGen is a three-language generator with the built-in feed on.
func discoveryGen(t *testing.T, prefixDefault bool, title string) *Generator {
	t.Helper()
	g, err := New(Config{
		Domain: "example.com", Feed: true, DefaultLanguage: "en",
		Languages: []string{"en", "pl", "ro"},
		LanguageConfigs: []ssgi18n.LanguageConfig{
			{Code: "en", Name: "English"}, {Code: "pl", Name: "Polski"}, {Code: "ro", Name: "Română"},
		},
		I18n: ssgi18n.Config{Enabled: true, PrefixDefaultLanguage: prefixDefault},
	})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Title = title
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatal(err)
	}
	return g
}

// TestAutodiscoveryFollowsThePageLanguage: a reader on /pl/ who presses
// subscribe gets the Polish feed the build wrote, under the site's own name.
func TestAutodiscoveryFollowsThePageLanguage(t *testing.T) {
	g := discoveryGen(t, false, "Example Site")
	page := `<html><head><title>x</title></head><body></body></html>`
	cases := map[string]string{
		"pl": `title="Example Site (Polski)" href="/pl/feed.xml"`,
		"ro": `title="Example Site (Română)" href="/ro/feed.xml"`,
		"en": `title="Example Site (English)" href="/feed.xml"`,
		// A language the site does not declare has no feed of its own.
		"de": `title="Example Site" href="/feed.xml"`,
	}
	for lang, want := range cases {
		if got := g.injectFeedLinks(page, lang); !strings.Contains(got, want) {
			t.Errorf("%s: want %s in\n%s", lang, want, got)
		}
	}
}

// TestAutodiscoveryWithAPrefixedDefaultLanguage: with prefix_default_language
// there is no /feed.xml at all, so the old link pointed at nothing.
func TestAutodiscoveryWithAPrefixedDefaultLanguage(t *testing.T) {
	g := discoveryGen(t, true, "")
	href, title := g.builtinFeedLink("en")
	if href != "/en/feed.xml" {
		t.Errorf("href = %q, want /en/feed.xml", href)
	}
	// No site title: the domain, as before.
	if title != "example.com (English)" {
		t.Errorf("title = %q", title)
	}
}

// TestAutodiscoveryWithoutI18nIsUnchanged, apart from the site title.
func TestAutodiscoveryWithoutI18nIsUnchanged(t *testing.T) {
	g := feedGen(t)
	g.config.Feed = true
	if href, title := g.builtinFeedLink("pl"); href != "/feed.xml" || title != "ex.com" {
		t.Errorf("got %q %q", href, title)
	}
	g.siteData.Title = "  Named  "
	if _, title := g.builtinFeedLink(""); title != "Named" {
		t.Errorf("title = %q, want the trimmed site title", title)
	}
}
