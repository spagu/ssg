package generator

// Content dimensions: named relations, versions, and outputs per page
// (GO-096).
//
// Most of what the proposal behind this ticket asked for already existed —
// type, language, taxonomy, source — because a custom taxonomy is a dimension
// and this build has had those for a while. Three things did not.
//
// **Relations were hard-coded.** A page could be part of a series, have a
// translation, or be "related" by a heuristic. It could not say
// `supersedes: [api-auth-v3]` and have that mean anything, even though the
// author is the only one who knows it.
//
// **A version was a number in Extra.** `version: 4` reached templates and
// meant nothing to the build, so the fourth revision of a document and its
// three predecessors competed with each other in search results — which is a
// real cost, paid quietly, by exactly the kind of site that versions its
// documentation.
//
// **Outputs were a site-wide list.** One reference page publishing JSON meant
// every page publishing JSON.
//
// All three are additive: frontmatter without the new keys behaves exactly as
// it did, which the golden corpora check.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// VersionsConfig tunes what a chain of versions means for search engines.
//
// There is no separate sitemap switch, deliberately. A superseded version
// carries a canonical to the current one, which is what tells an engine which
// URL is authoritative; and turning NoindexOld on makes the existing rule (#78)
// drop those pages from the sitemap, because a page that says "do not index
// this" has already answered the question. A second, parallel exclusion rule
// would only be able to disagree with the first.
type VersionsConfig struct {
	// NoindexOld adds `robots: noindex, follow` to superseded versions. Off by
	// default: a superseded page is still a page someone may have linked, and
	// removing it from an index is a decision, not a default.
	NoindexOld bool `yaml:"noindex_old" toml:"noindex_old" json:"noindex_old"`
}

// applyDimensions resolves relations and versions once the whole site is
// loaded, because both are about pages knowing each other.
func (g *Generator) applyDimensions() {
	// This build's findings only: a watch rebuild reports the relations as
	// they are now.
	g.relationMu.Lock()
	g.relationFailures = nil
	g.relationMu.Unlock()

	index := g.pagesBySlug()
	g.resolveRelations(index)
	g.resolveVersions()
}

// pagesBySlug indexes every page and post by slug, per language.
//
// A relation names a slug, and a slug is unique within a language rather than
// across the site: `see_also: [pricing]` from an English page means the English
// pricing page. A site with no i18n has one bucket and the question does not
// arise.
func (g *Generator) pagesBySlug() map[string]map[string]*models.Page {
	index := map[string]map[string]*models.Page{}
	add := func(p *models.Page) {
		lang := p.Lang
		if index[lang] == nil {
			index[lang] = map[string]*models.Page{}
		}
		if _, taken := index[lang][p.Slug]; !taken {
			index[lang][p.Slug] = p
		}
	}
	for i := range g.siteData.Pages {
		add(&g.siteData.Pages[i])
	}
	for i := range g.siteData.Posts {
		add(&g.siteData.Posts[i])
	}
	return index
}

// resolveRelations turns declared slugs into pages, and reports the ones that
// name nothing.
//
// A relation to a page that does not exist is a broken link by another name,
// so it follows `check_links`: silent when link checking is off, a warning
// when it is on, and a failure under strict.
func (g *Generator) resolveRelations(index map[string]map[string]*models.Page) {
	each := func(p *models.Page) {
		if len(p.Relations) == 0 {
			return
		}
		resolved := make(map[string][]*models.Page, len(p.Relations))
		for _, name := range sortedKeys(p.Relations) {
			for _, slug := range p.Relations[name] {
				target := lookupRelated(index, p.Lang, slug)
				if target == nil {
					g.noteBrokenRelation(p, name, slug)
					continue
				}
				resolved[name] = append(resolved[name], target)
			}
		}
		if len(resolved) > 0 {
			p.RelatedPages = resolved
		}
	}
	for i := range g.siteData.Pages {
		each(&g.siteData.Pages[i])
	}
	for i := range g.siteData.Posts {
		each(&g.siteData.Posts[i])
	}
}

// lookupRelated finds a slug in the page's own language, then falls back to
// the default bucket — a site that translates some of its pages should still
// be able to relate to one it has not translated yet.
func lookupRelated(index map[string]map[string]*models.Page, lang, slug string) *models.Page {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if p, ok := index[lang][slug]; ok {
		return p
	}
	if lang != "" {
		if p, ok := index[""][slug]; ok {
			return p
		}
	}
	return nil
}

// noteBrokenRelation records a relation naming a page the site does not have.
//
// It is recorded whatever the link-checking mode and reported by the link
// check, so one setting decides how loudly the site complains about references
// that go nowhere — whether they were written as links or as relations.
func (g *Generator) noteBrokenRelation(p *models.Page, name, slug string) {
	msg := fmt.Sprintf("relation %q on %q → %q: no page with that slug", name, p.Slug, slug)
	g.relationMu.Lock()
	defer g.relationMu.Unlock()
	g.relationFailures = append(g.relationFailures, msg)
}

// relationErrors returns the strict-mode failures, if any.
func (g *Generator) relationErrors() []string {
	g.relationMu.Lock()
	defer g.relationMu.Unlock()
	return append([]string(nil), g.relationFailures...)
}

// resolveVersions groups pages into version chains and decides which is the
// one search engines should see.
func (g *Generator) resolveVersions() {
	groups := map[string][]*models.Page{}
	collect := func(p *models.Page) {
		key := p.VersionOf
		if key == "" {
			return
		}
		groups[key] = append(groups[key], p)
	}
	for i := range g.siteData.Pages {
		collect(&g.siteData.Pages[i])
	}
	for i := range g.siteData.Posts {
		collect(&g.siteData.Posts[i])
	}
	for _, key := range sortedKeys(groups) {
		chain := groups[key]
		sort.SliceStable(chain, func(i, j int) bool {
			return compareVersions(chain[i].Version, chain[j].Version) > 0 // newest first
		})
		latest := chain[0]
		for _, p := range chain {
			p.Versions = chain
			p.IsLatest = p == latest
			if p.IsLatest {
				continue
			}
			// A superseded version points at the current one. Without this the
			// old revisions compete with the new for the same query, and the
			// site quietly ranks its own out-of-date documentation.
			if p.Canonical == "" {
				p.Canonical = "https://" + g.config.Domain + latest.GetURL()
			}
			if g.config.Versions.NoindexOld && p.Robots == "" {
				p.Robots = "noindex, follow"
			}
		}
	}
}

// compareVersions orders two version labels: numerically when both are
// numbers, and by text otherwise — so 2 sorts before 10, and "beta" still
// sorts against "alpha" predictably.
func compareVersions(a, b string) int {
	na, aerr := strconv.ParseFloat(strings.TrimSpace(a), 64)
	nb, berr := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if aerr == nil && berr == nil {
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		}
		return 0
	}
	return strings.Compare(a, b)
}

// pageWantsOutput is wantsOutput with the page's own `outputs:` taken into
// account: a page overrides the site, and a site with no list of its own still
// gets whatever the page asks for.
func (g *Generator) pageWantsOutput(p models.Page, format string) bool {
	if len(p.Outputs) == 0 {
		return g.wantsOutput(format)
	}
	for _, o := range p.Outputs {
		if strings.EqualFold(o, format) {
			return true
		}
	}
	return false
}

// relationFuncs are the template helpers for declared relations.
func (g *Generator) relationFuncs() map[string]interface{} {
	return map[string]interface{}{
		// relationsOf . "see_also" → the pages that relation names.
		"relationsOf": func(p models.Page, name string) []*models.Page {
			return p.RelatedPages[name]
		},
		// relatedBy . "supersedes" → the pages whose relation points HERE,
		// which is the half an author cannot write down: the new version does
		// not know which old ones replaced it until the site is loaded.
		"relatedBy": func(p models.Page, name string) []*models.Page {
			return g.inverseRelations(p, name)
		},
	}
}

// inverseRelations finds every page whose named relation points at this one.
func (g *Generator) inverseRelations(target models.Page, name string) []*models.Page {
	var out []*models.Page
	scan := func(p *models.Page) {
		for _, related := range p.RelatedPages[name] {
			if related.Slug == target.Slug && related.Lang == target.Lang {
				out = append(out, p)
				return
			}
		}
	}
	for i := range g.siteData.Pages {
		scan(&g.siteData.Pages[i])
	}
	for i := range g.siteData.Posts {
		scan(&g.siteData.Posts[i])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}
