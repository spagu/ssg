package generator

// The site's declared social identity, over whatever a migration recorded.
//
// `Marketing` reached the build from one place only: the metadata.json an
// `ssg migrate` crawl writes. So the og:image fallback existed in the code and
// was unreachable for any site not migrated from WordPress — including a
// documentation site, where no page has a hero photograph and every page wants
// the project's card. An Ahrefs crawl reported "Open Graph tags incomplete" on
// every page of this project's own site for exactly that reason (#264).
//
// Configuration wins field by field, the precedence `.Site.Title` already
// follows (#128): a migration fills in what the config has not said yet, and
// saying it later overrides the export rather than being ignored.

import "github.com/spagu/ssg/internal/models"

// applyConfiguredMarketing overlays the configured marketing block on whatever
// the site already carries.
func (g *Generator) applyConfiguredMarketing() {
	if g.siteData == nil {
		return
	}
	g.siteData.Marketing = mergeMarketing(g.siteData.Marketing, g.config.Marketing)
}

// mergeMarketing returns base with every non-empty field of over applied.
func mergeMarketing(base, over models.Marketing) models.Marketing {
	overlayString(&base.OGSiteName, over.OGSiteName)
	overlayString(&base.OGImage, over.OGImage)
	overlayString(&base.TwitterSite, over.TwitterSite)
	overlayString(&base.Favicon, over.Favicon)
	overlayString(&base.AppleTouchIcon, over.AppleTouchIcon)
	overlayString(&base.Logo, over.Logo)
	overlayString(&base.ThemeColor, over.ThemeColor)
	// Maps merge per key rather than replacing wholesale: a config adding one
	// verification token should not drop the three a migration found.
	base.Verification = overlayMap(base.Verification, over.Verification)
	base.SocialProfiles = overlayMap(base.SocialProfiles, over.SocialProfiles)
	base.Colors = overlayMap(base.Colors, over.Colors)
	return base
}

// overlayString writes over onto dst unless over says nothing.
func overlayString(dst *string, over string) {
	if over != "" {
		*dst = over
	}
}

// overlayMap merges over onto base, leaving base untouched.
func overlayMap(base, over map[string]string) map[string]string {
	if len(over) == 0 {
		return base
	}
	merged := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range over {
		if v != "" {
			merged[k] = v
		}
	}
	return merged
}
