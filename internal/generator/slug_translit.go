package generator

// slug_transliterate (#426): the slug rule a build uses for labels — terms,
// series, authors, categories an extra source names, story pages — and the
// 301s that keep the URLs the plain rule made working once the switch is on.
//
// Only archive roots are redirected (/tag/<old>/ → /tag/<new>/); their
// /page/N/ and feed URLs are not, because a splat per term would spend the
// host's small budget of dynamic rules. Pages built from external-source
// records get an alias instead, which also writes a stub where the host does
// not read _redirects.

import (
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// labelSlug is models.Slugify, or models.SlugifyTranslit when
// slug_transliterate is on.
func (g *Generator) labelSlug(label string) string {
	return models.SlugFunc(g.config.SlugTransliterate)(label)
}

// redirectMovedSlugs records a 301 from base+<plain slug> to base+<slug> for
// each label whose archive moved because of the switch. labels maps a label
// to the slug its archive is written at; a slug that did not come from the
// label (an export's canonical slug, term metadata) moved for another reason
// and is left alone. An old slug two labels shared, or one that is still a
// live slug under base, is not redirected: either answer would be a guess.
func (g *Generator) redirectMovedSlugs(base string, labels map[string]string) {
	if !g.config.SlugTransliterate {
		return
	}
	live := make(map[string]bool, len(labels))
	for _, slug := range labels {
		live[slug] = true
	}
	to := map[string]string{}
	shared := map[string]bool{}
	for label, slug := range labels {
		old := models.Slugify(label)
		if old == "" || old == slug || live[old] || g.labelSlug(label) != slug {
			continue
		}
		if prev, seen := to[old]; seen && prev != slug {
			shared[old] = true
		}
		to[old] = slug
	}
	olds := make([]string, 0, len(to))
	for old := range to {
		olds = append(olds, old)
	}
	sort.Strings(olds)
	base = strings.Trim(base, "/") + "/"
	for _, old := range olds {
		if !shared[old] {
			g.addArchiveRedirect(base+old, base+to[old])
		}
	}
}

// redirectMovedAuthors does it for author archives, whose map is keyed by
// slug: the label is the author's own slug when it has one, the name otherwise
// (authorSlug).
func (g *Generator) redirectMovedAuthors(written map[string]string) {
	labels := map[string]string{}
	for _, a := range g.siteData.Authors {
		label := a.Slug
		if label == "" {
			label = a.Name
		}
		if slug := g.labelSlug(label); written[slug] != "" {
			labels[label] = slug
		}
	}
	g.redirectMovedSlugs("author", labels)
}

// derivedTagSlugs is the tag→slug map without the tags whose slug is an
// export's canonical one (issue #27): their URL never came from the label.
func (g *Generator) derivedTagSlugs(slugs map[string]string) map[string]string {
	out := make(map[string]string, len(slugs))
	for name, slug := range slugs {
		if g.siteData.TagSlugs[strings.ToLower(name)] == "" {
			out[name] = slug
		}
	}
	return out
}

// termSlugs maps each term's key to its slug. The key is the label lowercased
// and with its spaces collapsed, which slugs the same as the label itself;
// the display name is not used because term metadata may rename it.
func termSlugs(terms []*taxonomy.Term) map[string]string {
	out := make(map[string]string, len(terms))
	for _, t := range terms {
		out[t.Key] = t.Slug
	}
	return out
}

// aliasLegacySlug gives a record page whose slug the switch changed an alias
// at the URL the plain slug made (models.Page.LegacySlug), with the same
// permalink pattern. Aliases are written without the language prefix when
// i18n adds it back (writeAliasStubs).
func (g *Generator) aliasLegacySlug(p *models.Page, pattern string) {
	if p.LegacySlug == "" {
		return
	}
	old, cur := *p, *p
	old.Slug = p.LegacySlug
	if pattern != "" {
		old.PermalinkPath = g.expandPermalink(pattern, old)
	}
	if g.config.I18n.Enabled {
		old.LangPrefix, cur.LangPrefix = "", ""
	}
	if from := old.GetURL(); from != cur.GetURL() {
		p.Aliases = append(p.Aliases, from)
	}
}
