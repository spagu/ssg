package generator

// The og:image fallback existed in the code and was reachable only by a site
// migrated from WordPress, because Marketing came from metadata.json alone
// (#264).

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestConfiguredMarketingWinsOverAMigration: configuration wins field by field,
// the precedence .Site.Title already follows (#128).
func TestConfiguredMarketingWinsOverAMigration(t *testing.T) {
	migrated := models.Marketing{
		OGSiteName:   "Old Name",
		OGImage:      "/old-card.png",
		TwitterSite:  "@old",
		Favicon:      "/favicon.ico",
		Verification: map[string]string{"google-site-verification": "g1"},
	}
	declared := models.Marketing{
		OGSiteName:   "New Name",
		OGImage:      "/new-card.png",
		Verification: map[string]string{"msvalidate.01": "m1"},
	}

	got := mergeMarketing(migrated, declared)

	if got.OGSiteName != "New Name" || got.OGImage != "/new-card.png" {
		t.Errorf("configuration did not win: %+v", got)
	}
	// What the config does not mention survives, rather than being blanked.
	if got.TwitterSite != "@old" || got.Favicon != "/favicon.ico" {
		t.Errorf("an unmentioned field was dropped: %+v", got)
	}
	// Maps merge per key: adding one token must not drop the migration's.
	if got.Verification["google-site-verification"] != "g1" || got.Verification["msvalidate.01"] != "m1" {
		t.Errorf("verification tokens did not merge: %+v", got.Verification)
	}
	// The migration's map is left alone.
	if len(migrated.Verification) != 1 {
		t.Error("the base map was mutated")
	}
}

// TestMarketingReachesASiteWithNoMigration: the case the issue is about — a
// site built from scratch, where the code had the fallback and no way in.
func TestMarketingReachesASiteWithNoMigration(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Marketing = models.Marketing{}
	g.config.Marketing = models.Marketing{OGImage: "https://example.com/card.png", OGSiteName: "SSG"}

	g.applyConfiguredMarketing()

	if g.siteData.Marketing.OGImage != "https://example.com/card.png" {
		t.Fatalf("the declared card never reached the site: %+v", g.siteData.Marketing)
	}
	// And the head injector, which was already able to emit it, now does.
	head := g.buildMarketingHead("")
	if !strings.Contains(head, `property="og:image"`) || !strings.Contains(head, "card.png") {
		t.Errorf("og:image is still missing from the head:\n%s", head)
	}
}

// TestMarketingMergeOnAnEmptyConfig: a site that declares nothing keeps exactly
// what it had.
func TestMarketingMergeOnAnEmptyConfig(t *testing.T) {
	base := models.Marketing{OGImage: "/card.png", Colors: map[string]string{"primary": "#000"}}
	got := mergeMarketing(base, models.Marketing{})

	if got.OGImage != "/card.png" || got.Colors["primary"] != "#000" {
		t.Errorf("an empty config changed the site: %+v", got)
	}
	// A nil generator site data is not a crash.
	g := &Generator{}
	g.applyConfiguredMarketing()
}
