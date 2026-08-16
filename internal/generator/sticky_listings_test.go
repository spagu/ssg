package generator

// The reporter's site, as a regression (#155). Ten posts, one pinned, neither
// the newest nor the oldest, so a listing that ignores the flag puts it sixth
// and the difference is visible rather than accidental.
//
// The 1.8.38 fix went into sortPostsByDate, and the front page never called it:
// it read g.siteData.Posts, ordered once at load time by a date-only comparator.
// Same site, two orders. These tests assert every listing at once, so the two
// paths cannot drift apart again.

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// reportedSite builds ten dated posts with the sixth-by-date one pinned.
func reportedSite() []models.Page {
	posts := make([]models.Page, 0, 10)
	for i := 1; i <= 10; i++ {
		posts = append(posts, models.Page{
			Title: "post-" + string(rune('0'+i%10)), Slug: "p", Status: "publish", Type: "post",
			Date:       time.Date(2024, time.May, 21-i, 0, 0, 0, 0, time.UTC),
			Sticky:     i == 6,
			Categories: []int{1}, Tags: []string{"news"}, Content: "Body.",
		})
	}
	return posts
}

// pinnedPosition returns where the pinned post sits in an ordered listing,
// 1-based, or 0 when it is absent.
func pinnedPosition(posts []models.Page) int {
	for i, p := range posts {
		if p.Sticky {
			return i + 1
		}
	}
	return 0
}

// TestLoadedPostsArePinnedFirst: the one sort every listing inherits. The front
// page and .Site.Posts read this slice directly, which is why it — and not each
// renderer — is where pinning has to happen.
func TestLoadedPostsArePinnedFirst(t *testing.T) {
	if got := pinnedPosition(sortPostsByDate(reportedSite())); got != 1 {
		t.Fatalf("the pinned post is %d of 10, want 1", got)
	}
}

// TestFrontPageAndSitePostsAgree: the two listings that disagreed. The front
// page renders .Posts; a theme's "recent posts" block ranges over .Site.Posts.
// Both come from the loaded set, so both must lead with the pinned post.
func TestFrontPageAndSitePostsAgree(t *testing.T) {
	g := newTestGen(t, `{{define "index.html"}}POSTS:{{range .Posts}}{{if .Sticky}}[P]{{end}}{{.Title}} {{end}}
SITE:{{range .Site.Posts}}{{if .Sticky}}[P]{{end}}{{.Title}} {{end}}{{end}}`)
	g.siteData.Posts = sortPostsByDate(reportedSite())

	if err := g.generateIndex(); err != nil {
		t.Fatal(err)
	}
	body := mustReadOutput(t, g, "index.html")
	for _, section := range []string{"POSTS:", "SITE:"} {
		part := body[strings.Index(body, section)+len(section):]
		if i := strings.Index(part, "\n"); i > 0 {
			part = part[:i]
		}
		if !strings.HasPrefix(strings.TrimSpace(part), "[P]") {
			t.Errorf("%s must lead with the pinned post: %q", section, strings.TrimSpace(part))
		}
	}
}

// TestArchivesAndFrontPageShareOneOrder: the archives were already right; this
// pins them to the front page so a future change cannot fix one and break the
// other, which is exactly how the reopened report happened.
func TestArchivesAndFrontPageShareOneOrder(t *testing.T) {
	posts := sortPostsByDate(reportedSite())
	if got := pinnedPosition(posts); got != 1 {
		t.Fatalf("front-page order puts the pinned post %d", got)
	}
	// A category or tag archive filters the loaded set and sorts it the same
	// way: the pinned post leads there too, however few of its neighbours the
	// filter kept.
	subset := append([]models.Page(nil), reportedSite()[4:]...)
	if got := pinnedPosition(sortPostsByDate(subset)); got != 1 {
		t.Errorf("an archive subset puts the pinned post %d, want 1", got)
	}
	// Dates below the pinned post keep their own order.
	titles := titlesOf(t, posts)
	if strings.Count(titles, ",") != 9 {
		t.Fatalf("every post must survive the sort: %q", titles)
	}
}

// TestFeedsStayChronological: a feed is a record of what was published when.
// WordPress applies stickiness to the blog query, not to /feed/, and a pinned
// older post at the top of an Atom feed resurfaces in subscribers' readers as
// though it were new — an unintended side effect of the 1.8.38 fix.
func TestFeedsStayChronological(t *testing.T) {
	ordered := sortPostsChronologically(reportedSite())
	if got := pinnedPosition(ordered); got != 6 {
		t.Fatalf("a feed must ignore pinning: pinned post is %d, want 6 (its date)", got)
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].Date.Before(ordered[i].Date) {
			t.Fatalf("a feed must be newest-first at %d: %s before %s",
				i, ordered[i-1].Date, ordered[i].Date)
		}
	}
}

// TestSortsAgreeWhenNothingIsPinned: a site that pins nothing gets the order it
// has always had from either sort — the flag is additive, not a new sort.
func TestSortsAgreeWhenNothingIsPinned(t *testing.T) {
	plain := reportedSite()
	for i := range plain {
		plain[i].Sticky = false
	}
	if a, b := titlesOf(t, sortPostsByDate(plain)), titlesOf(t, sortPostsChronologically(plain)); a != b {
		t.Fatalf("without pinning the two sorts must agree:\n  %s\n  %s", a, b)
	}
}
