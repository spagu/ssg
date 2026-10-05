package generator

// A content page as the site's front page (#129).
//
// Every CMS worth migrating from can put a real page at `/` — WordPress calls it
// a static front page — and the export says so plainly: `link: "/"`. ssg used to
// print a warning and DROP that page, because the generated post listing already
// owns index.html. The page a site leads with is not a collision to be skipped;
// it is the most important document there is.
//
// So a page whose URL resolves to the site root becomes the front page, and the
// post listing moves to `posts_page:` (the second half of WordPress's own
// arrangement) or is not generated at all when no home is configured for it.

import (
	"fmt"
	"path"
	"strings"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// rootPage returns the page that claims a language's root, or nil when
// nothing does. The root is the language's prefix: "" for the site root
// (`link: "/"`), "en" for a language served under /en/ (`link: "/en/"`, #319)
// — so every language can lead with a page of its own, and its post listing
// moves to posts_page exactly as the default language's does.
//
// An empty lang means a single-language build, where a page's own `lang:` is
// documentation rather than routing — every page belongs to the one site, so
// none is filtered out.
func rootPage(pages []models.Page, lang, langPrefix string) *models.Page {
	for i := range pages {
		if lang != "" && pages[i].Lang != lang {
			continue
		}
		if isLanguageRoot(pages[i].GetOutputPath(), langPrefix) {
			return &pages[i]
		}
	}
	return nil
}

// isLanguageRoot reports whether an output path is the root of the language
// served under prefix ("" for the site root).
func isLanguageRoot(p, prefix string) bool {
	if prefix == "" {
		return isRootOutputPath(p)
	}
	return strings.Trim(p, "/") == strings.Trim(prefix, "/")
}

// isRootOutputPath reports whether an output path addresses index.html at the
// site root.
func isRootOutputPath(p string) bool {
	return p == "" || p == "."
}

// isDesignatedFrontPage reports whether this page is the one rootPage picked —
// the page the build's front-page report names. Exactly one document may write
// the root: #129 removed the page-side GO-023 guard so a page could BE the
// front page, and with it went the protection against a SECOND page landing
// there, where render order silently decided what the site leads with (#234).
//
// The comparison mirrors rootPage's own selection, so the page that renders
// the root is always the page the report claimed.
func (g *Generator) isDesignatedFrontPage(page models.Page) bool {
	lang, prefix := "", ""
	if g.config.I18n.Enabled {
		lang, prefix = page.Lang, g.languagePrefix(page.Lang)
	}
	front := rootPage(g.siteData.Pages, lang, prefix)
	return front != nil && front.SourceFile == page.SourceFile &&
		front.Slug == page.Slug && front.Title == page.Title
}

// languagePrefix is the URL prefix a language is served under ("" for the
// default language unless prefix_default_language is set).
func (g *Generator) languagePrefix(lang string) string {
	return ssgi18n.Prefix(lang, g.config.DefaultLanguage, g.config.I18n)
}

// isLanguageRootPage reports whether a page writes its language's root:
// the site root, or /en/ for a page of the language served there.
func (g *Generator) isLanguageRootPage(page models.Page) bool {
	prefix := ""
	if g.config.I18n.Enabled {
		prefix = g.languagePrefix(page.Lang)
	}
	return isLanguageRoot(page.GetOutputPath(), prefix)
}

// postsListingPrefix resolves where the post listing goes for one language.
// Empty means the site root; ok is false when the root is taken by a page and
// no posts_page is configured, so the listing has nowhere to live.
func (g *Generator) postsListingPrefix(langPrefix string, rootTaken bool) (prefix string, ok bool) {
	postsPage := strings.Trim(strings.TrimSpace(g.config.PostsPage), "/")
	if !rootTaken {
		// Without a front-page document the listing keeps the root, and
		// posts_page (when set) is where it goes instead.
		if postsPage == "" {
			return langPrefix, true
		}
		return path.Join(langPrefix, postsPage), true
	}
	if postsPage == "" {
		return "", false
	}
	return path.Join(langPrefix, postsPage), true
}

// reportFrontPage explains, once per build, that a page took the root and where
// the listing went. Silence would leave the operator wondering why /
// stopped listing posts.
func (g *Generator) reportFrontPage(page *models.Page, prefix string, listed bool) {
	if page == nil || g.config.Quiet {
		return
	}
	fmt.Printf("   🏠 Front page: %s\n", frontPageLabel(*page))
	switch {
	case listed:
		fmt.Printf("      Post listing: /%s/\n", strings.Trim(prefix, "/"))
	case len(g.siteData.Posts) > 0:
		fmt.Printf("      %d post(s) are not listed anywhere — set posts_page: \"blog\" to publish the listing\n",
			len(g.siteData.Posts))
	}
}

// frontPageLabel names the front page by its source file, falling back to its
// title, so the line points at something the operator can open.
func frontPageLabel(page models.Page) string {
	if page.SourceFile != "" {
		return page.SourceFile
	}
	return page.Title
}

// postsPageOwner reports whether a page document occupies the address
// posts_page names (#150).
//
// WordPress's "Posts page" IS a page: the admin assigns an existing page to it,
// WordPress ignores that page's content and renders the loop in its place. An
// export carries both faithfully — a "Blog" page with empty content, and
// posts_page: blog — and ssg used to write the page there and the listing's
// SECOND page under it, so /blog/ served an empty document while /blog/page/2/
// served the listing. Two of six sites in one batch hit it.
//
// The setting wins, matching the source CMS and what the operator asked for by
// setting the key, and the build says so rather than silently choosing.
func (g *Generator) postsPageOwner(pages []models.Page, lang string) *models.Page {
	listing := strings.Trim(strings.TrimSpace(g.config.PostsPage), "/")
	if listing == "" {
		return nil
	}
	for i := range pages {
		if lang != "" && pages[i].Lang != lang {
			continue
		}
		if strings.Trim(pages[i].GetOutputPath(), "/") == listing {
			return &pages[i]
		}
	}
	return nil
}

// reportPostsPageCollision names both documents once per build, so the operator
// can see which one is being served without diffing the output tree.
func (g *Generator) reportPostsPageCollision(page *models.Page) {
	if page == nil || g.config.Quiet {
		return
	}
	fmt.Printf("   📰 /%s/: the post listing replaces the page of the same address (posts_page)\n",
		strings.Trim(g.config.PostsPage, "/"))
	fmt.Printf("      the page %s is not written; rename it or change posts_page to keep both\n",
		frontPageLabel(*page))
}

// withoutPage returns the pages except the one given, compared by output path
// so a page is removed regardless of which copy of the value is held.
func withoutPage(pages []models.Page, drop models.Page) []models.Page {
	out := make([]models.Page, 0, len(pages))
	for _, p := range pages {
		if p.GetOutputPath() == drop.GetOutputPath() {
			continue
		}
		out = append(out, p)
	}
	return out
}
