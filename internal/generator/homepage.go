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

	"github.com/spagu/ssg/internal/models"
)

// rootPage returns the page that claims the site root for a language, or nil
// when nothing does. A page claims it by resolving to an empty output path,
// which is what `link: "/"` produces.
//
// An empty lang means a single-language build, where a page's own `lang:` is
// documentation rather than routing — every page belongs to the one site, so
// none is filtered out.
func rootPage(pages []models.Page, lang string) *models.Page {
	for i := range pages {
		if lang != "" && pages[i].Lang != lang {
			continue
		}
		if isRootOutputPath(pages[i].GetOutputPath()) {
			return &pages[i]
		}
	}
	return nil
}

// isRootOutputPath reports whether an output path addresses index.html at the
// site root.
func isRootOutputPath(p string) bool {
	return p == "" || p == "."
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
