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
