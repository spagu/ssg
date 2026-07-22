package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// This file wires the generic taxonomy system (audit/taxonomies-feature.md)
// into the build: registry construction from frontmatter, custom archive
// rendering with template fallback chains, and sitemap/feed integration.
// Legacy category/tag/series archives stay on their original pipelines.

// buildTaxonomies resolves the configured taxonomy definitions, extracts every
// page's assignments (priority: taxonomies map > direct field > legacy fields),
// syncs generic values back onto the legacy tag/series fields, and validates
// slug and output-path collisions. Runs after loadContent+loadData so language
// codes, resolved categories and data/taxonomies/* metadata are available.
func (g *Generator) buildTaxonomies() error {
	reserved := []string{"author", "page"}
	for _, lang := range g.siteData.Languages {
		reserved = append(reserved, lang.Code)
	}
	defs, names, err := taxonomy.Resolve(g.config.Taxonomies, reserved)
	if err != nil {
		return err
	}
	reg := taxonomy.NewRegistry(defs, names, slugify)
	g.taxonomies = reg

	// Posts drive archives; pages only get .Taxonomies for template helpers.
	for i := range g.siteData.Posts {
		if err := g.assignPageTaxonomies(&g.siteData.Posts[i], true); err != nil {
			return err
		}
	}
	for i := range g.siteData.Pages {
		if err := g.assignPageTaxonomies(&g.siteData.Pages[i], false); err != nil {
			return err
		}
	}

	g.applyCategorySlugs()
	g.applyTermMetadata()
	if err := reg.ValidateSlugs(); err != nil {
		return err
	}
	return g.checkTaxonomyCollisions()
}

// assignPageTaxonomies resolves one page's taxonomy values and, for posts,
// registers them in the registry's language bucket.
func (g *Generator) assignPageTaxonomies(p *models.Page, isPost bool) error {
	src := taxonomy.PageSources{
		TaxonomiesFM:  p.TaxonomiesFM,
		Extra:         p.Extra,
		CategoryNames: g.categoryNames(p),
		Tags:          p.Tags,
		Series:        p.Series,
	}
	source := p.SourceFile
	if source == "" {
		source = p.Slug
	}
	assigned, err := taxonomy.ExtractAssignments(g.taxonomies.Definitions, g.taxonomies.Names, src, source)
	if err != nil {
		return err
	}
	p.Taxonomies = assigned
	// Generic → legacy sync: `taxonomies:` values reach the legacy tag/series
	// pipelines so their archives include them (category stays ID-based).
	if tags, ok := assigned["tag"]; ok {
		p.Tags = tags
	}
	if series, ok := assigned["series"]; ok && len(series) > 0 {
		p.Series = series[0]
	}
	if !isPost {
		return nil
	}
	lang := g.taxonomyLang(p.Lang)
	for _, name := range g.taxonomies.Names {
		if vals := assigned[name]; len(vals) > 0 {
			if err := g.taxonomies.Assign(name, lang, vals, *p); err != nil {
				return err
			}
		}
	}
	return nil
}

// categoryNames resolves a page's legacy category assignments to display names
// (WordPress IDs via SiteData.Categories plus the single md `category:` field).
func (g *Generator) categoryNames(p *models.Page) []string {
	var names []string
	for _, id := range p.Categories {
		if cat, ok := g.siteData.Categories[id]; ok && cat.Name != "" {
			names = append(names, cat.Name)
		}
	}
	if p.Category != "" {
		names = append(names, p.Category)
	}
	return names
}

// taxonomyLang maps a page language onto a registry bucket: multilingual i18n
// builds scope terms per language, everything else shares the "" bucket.
func (g *Generator) taxonomyLang(pageLang string) string {
	if !g.config.I18n.Enabled {
		return ""
	}
	return pageLang
}

// taxonomyLangs lists the registry buckets the build renders archives for.
func (g *Generator) taxonomyLangs() []string {
	if !g.config.I18n.Enabled {
		return []string{""}
	}
	langs := make([]string, 0, len(g.siteData.Languages))
	for _, l := range g.siteData.Languages {
		langs = append(langs, l.Code)
	}
	return langs
}

// applyCategorySlugs overrides registry category slugs with the WordPress-style
// slugs from SiteData so helper URLs match the legacy /category/{slug}/ output.
func (g *Generator) applyCategorySlugs() {
	for _, cat := range g.siteData.Categories {
		if cat.Name == "" || cat.Slug == "" {
			continue
		}
		for _, lang := range g.taxonomyLangs() {
			if g.taxonomies.Term("category", lang, cat.Name) != nil {
				g.taxonomies.ApplyTermMeta("category", cat.Name,
					map[string]interface{}{"slug": models.SanitizeRelPath(cat.Slug)}, lang)
			}
		}
	}
}

// applyTermMetadata overlays data/taxonomies/<name>.yaml onto registry terms
// (display name, slug, description, weight, free-form data).
func (g *Generator) applyTermMetadata() {
	meta, ok := g.data["taxonomies"].(map[string]interface{})
	if !ok {
		return
	}
	for _, name := range g.taxonomies.Names {
		sub, ok := meta[name].(map[string]interface{})
		if !ok {
			continue
		}
		for _, termKey := range sortedKeys(sub) {
			m, ok := sub[termKey].(map[string]interface{})
			if !ok {
				continue
			}
			for _, lang := range g.taxonomyLangs() {
				g.taxonomies.ApplyTermMeta(name, termKey, m, lang)
			}
		}
	}
}

// taxonomyBaseURL is the archive root for a taxonomy in one language,
// e.g. /technology/ or /en/technology/ (always with trailing slash).
func (g *Generator) taxonomyBaseURL(def taxonomy.Definition, lang string) string {
	url := "/"
	if g.config.I18n.Enabled {
		if prefix := ssgi18n.Prefix(lang, g.config.DefaultLanguage, g.config.I18n); prefix != "" {
			url += prefix + "/"
		}
	}
	return url + def.Path + "/"
}

// checkTaxonomyCollisions fails the build when a custom taxonomy index or term
// URL would overwrite a page, post or alias output (always checked, i18n or not).
func (g *Generator) checkTaxonomyCollisions() error {
	taken := g.takenContentURLs()
	for _, name := range g.taxonomies.Names {
		def := g.taxonomies.Definitions[name]
		if def.Legacy || !def.Archive {
			continue
		}
		if err := g.checkTaxonomyURLs(def, taken); err != nil {
			return err
		}
	}
	return nil
}

// takenContentURLs maps every post, page and alias URL to a human-readable owner.
func (g *Generator) takenContentURLs() map[string]string {
	taken := map[string]string{}
	claim := func(url, owner string) {
		if url != "" {
			taken[url] = owner
		}
	}
	for _, p := range g.siteData.Posts {
		claim(p.GetURL(), "post "+p.Slug)
	}
	for _, p := range g.siteData.Pages {
		claim(p.GetURL(), "page "+p.Slug)
		for _, a := range p.Aliases {
			claim("/"+strings.Trim(a, "/")+"/", "alias of page "+p.Slug)
		}
	}
	return taken
}

// archiveURLOwner reports which content page/post/alias (if any) already owns
// a legacy archive URL (/kind/slug/). Explicit content always wins over an
// auto-generated archive: the archive is skipped instead of silently
// overwriting the page (GO-050). The map is built once per generator.
func (g *Generator) archiveURLOwner(kind, slug string) (string, bool) {
	if g.ownedURLs == nil {
		g.ownedURLs = g.takenContentURLs()
	}
	owner, taken := g.ownedURLs["/"+kind+"/"+slug+"/"]
	return owner, taken
}

// checkTaxonomyURLs verifies one taxonomy's index and term URLs against taken output URLs.
func (g *Generator) checkTaxonomyURLs(def taxonomy.Definition, taken map[string]string) error {
	for _, lang := range g.taxonomyLangs() {
		base := g.taxonomyBaseURL(def, lang)
		if owner, hit := taken[base]; hit {
			return fmt.Errorf("taxonomy %q index URL %s collides with %s", def.Name, base, owner)
		}
		for _, t := range g.taxonomies.Terms(def.Name, lang) {
			if owner, hit := taken[base+t.Slug+"/"]; hit {
				return fmt.Errorf("taxonomy %q term %q URL %s collides with %s",
					def.Name, t.Name, base+t.Slug+"/", owner)
			}
		}
	}
	return nil
}

// generateTaxonomies renders every custom taxonomy's index and term archives
// (legacy category/tag/series stay on their original pipelines).
func (g *Generator) generateTaxonomies() error {
	if g.taxonomies == nil {
		return nil
	}
	for _, name := range g.taxonomies.Names {
		def := g.taxonomies.Definitions[name]
		if def.Legacy || !def.Archive {
			continue
		}
		for _, lang := range g.taxonomyLangs() {
			if err := g.generateTaxonomyArchives(def, lang); err != nil {
				return err
			}
		}
	}
	return nil
}

// generateTaxonomyArchives writes one language's taxonomy index page plus a
// (paginated) archive per term.
func (g *Generator) generateTaxonomyArchives(def taxonomy.Definition, lang string) error {
	terms := g.taxonomies.Terms(def.Name, lang)
	if len(terms) == 0 {
		return nil
	}
	// Align the mutable render language with this archive's bucket so template
	// helpers (taxonomyTerms/termURL/…) resolve against the right language.
	g.currentLang = lang
	base := g.taxonomyBaseURL(def, lang)
	info := TaxonomyInfo{Name: def.Name, Label: def.Label, Singular: def.Singular, Path: def.Path, URL: base}
	views := termViews(terms, base)

	indexOut := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.Trim(base, "/")), indexHTMLName)
	indexData := struct {
		Site         *models.SiteData
		Taxonomy     TaxonomyInfo
		Terms        []TaxonomyTerm
		Lang         string
		Domain       string
		Vars         map[string]interface{}
		Data         map[string]interface{}
		ExternalData map[string]interface{}
	}{g.siteData, info, views, lang, g.config.Domain, g.config.Variables, g.data, g.externalData}
	if err := g.renderTaxonomyPage(g.taxonomyIndexChain(def), indexOut, indexData); err != nil {
		return err
	}

	for i, t := range terms {
		posts := sortPostsByDate(g.taxonomies.Pages(def.Name, lang, t.Key))
		if err := g.renderTermArchive(def, lang, info, views[i], posts, base); err != nil {
			return err
		}
	}
	return nil
}

// termChunk is one paginated slice of a term archive.
type termChunk struct {
	Posts []models.Page
	Pager Pager
}

// paginateTerm splits a term's posts into pages with prev/next URLs rooted at
// termURL. paginate <= 0 (or few posts) yields a single un-paginated chunk.
func paginateTerm(posts []models.Page, per int, termURL string) []termChunk {
	if per <= 0 || len(posts) <= per {
		return []termChunk{{Posts: posts, Pager: Pager{Current: 1, Total: 1, PerPage: per}}}
	}
	total := (len(posts) + per - 1) / per
	chunks := make([]termChunk, 0, total)
	for page := 1; page <= total; page++ {
		start, end := (page-1)*per, page*per
		if end > len(posts) {
			end = len(posts)
		}
		pager := Pager{Current: page, Total: total, PerPage: per}
		if page > 1 {
			pager.PrevURL = termPageURL(termURL, page-1)
		}
		if page < total {
			pager.NextURL = termPageURL(termURL, page+1)
		}
		chunks = append(chunks, termChunk{Posts: posts[start:end], Pager: pager})
	}
	return chunks
}

// renderTermArchive writes /{base}/{slug}/ and, when paginate is on and needed,
// /{base}/{slug}/page/N/. The context keeps Category/Kind/Name/Posts so the
// category.html fallback used by legacy archives renders unchanged.
func (g *Generator) renderTermArchive(def taxonomy.Definition, lang string, info TaxonomyInfo, term TaxonomyTerm, posts []models.Page, base string) error {
	slug := models.SanitizeRelPath(term.Slug)
	if slug == "" {
		return nil
	}
	root := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.Trim(base, "/")), slug)
	chain := g.taxonomyTermChain(def)
	// A per-taxonomy paginate overrides the global page size; 0 falls back to
	// the site-wide paginate, preserving existing behaviour (#44).
	perPage := g.config.Paginate
	if def.Paginate > 0 {
		perPage = def.Paginate
	}
	for _, chunk := range paginateTerm(posts, perPage, term.URL) {
		outPath := filepath.Join(root, indexHTMLName)
		if chunk.Pager.Current > 1 {
			outPath = filepath.Join(root, "page", fmt.Sprintf("%d", chunk.Pager.Current), indexHTMLName)
		}
		data := struct {
			Site         *models.SiteData
			Taxonomy     TaxonomyInfo
			Term         TaxonomyTerm
			Category     models.Category
			Kind         string
			Name         string
			Series       string
			Posts        []models.Page
			Pager        Pager
			Lang         string
			Domain       string
			Vars         map[string]interface{}
			Data         map[string]interface{}
			ExternalData map[string]interface{}
		}{g.siteData, info, term, models.Category{Name: term.Name, Slug: slug}, def.Name,
			term.Name, term.Name, chunk.Posts, chunk.Pager, lang, g.config.Domain, g.config.Variables, g.data, g.externalData}
		if err := g.renderTaxonomyPage(chain, outPath, data); err != nil {
			return err
		}
	}
	return nil
}

// termPageURL returns the URL of page n of a term archive (page 1 is the term root).
func termPageURL(termURL string, n int) string {
	if n <= 1 {
		return termURL
	}
	return fmt.Sprintf("%spage/%d/", termURL, n)
}

// renderTaxonomyPage renders data with the first template of the chain that
// exists in the loaded template set. A chain with no match is a warning, not an
// error, mirroring the legacy archive renderer's behaviour.
func (g *Generator) renderTaxonomyPage(chain []string, outPath string, data interface{}) error {
	if err := g.ensureWithinOutput(outPath); err != nil {
		fmt.Printf("   ⚠️  Skipping taxonomy page with unsafe path: %v\n", err)
		return nil
	}
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	for _, name := range chain {
		if !g.hasTemplate(name) {
			continue
		}
		return g.renderTemplate(name, outPath, data)
	}
	fmt.Printf("   ⚠️  No template found for %s (tried %s)\n", outPath, strings.Join(chain, ", "))
	return nil
}

// hasTemplate reports whether a template name exists in the active engine's
// set WITH a non-whitespace body. ParseGlob names every file by its basename
// even when the file only holds {{define}} blocks for other names — executing
// such a shell writes a whitespace-only page, which is never what a theme
// author meant (GO-051), so shells count as absent and fallbacks apply.
func (g *Generator) hasTemplate(name string) bool {
	if g.engine != nil {
		_, ok := g.engineTmpls[name]
		return ok
	}
	if g.tmpl == nil {
		return false
	}
	t := g.tmpl.Lookup(name)
	return t != nil && t.Tree != nil && t.Tree.Root != nil &&
		strings.TrimSpace(t.Tree.Root.String()) != ""
}

// taxonomyIndexChain is the index-page template fallback order: explicit
// config override, then taxonomy-<name>.html → taxonomy.html → archive.html →
// category.html (the shared archive fallback every theme has).
func (g *Generator) taxonomyIndexChain(def taxonomy.Definition) []string {
	var chain []string
	if def.Template != "" {
		chain = append(chain, def.Template)
	}
	return append(chain, "taxonomy-"+def.Name+".html", "taxonomy.html", "archive.html", categoryHTMLName)
}

// taxonomyTermChain is the term-archive fallback order (same tail as the index).
func (g *Generator) taxonomyTermChain(def taxonomy.Definition) []string {
	var chain []string
	if def.TermTemplate != "" {
		chain = append(chain, def.TermTemplate)
	}
	return append(chain, "taxonomy-"+def.Name+"-term.html", "taxonomy-term.html", "archive.html", categoryHTMLName)
}

// writeTaxonomySitemap appends sitemap entries for custom taxonomy indexes and
// term archives (sitemap: true, the default).
func (g *Generator) writeTaxonomySitemap(sb *strings.Builder) {
	if g.taxonomies == nil {
		return
	}
	for _, name := range g.taxonomies.Names {
		def := g.taxonomies.Definitions[name]
		if def.Legacy || !def.Archive || !def.Sitemap {
			continue
		}
		for _, lang := range g.taxonomyLangs() {
			terms := g.taxonomies.Terms(name, lang)
			if len(terms) == 0 {
				continue
			}
			base := g.taxonomyBaseURL(def, lang)
			g.writeSitemapRelURL(sb, base)
			for _, t := range terms {
				g.writeSitemapRelURL(sb, base+t.Slug+"/")
			}
		}
	}
}

// writeSitemapRelURL appends one archive-style sitemap entry for a site-relative URL.
func (g *Generator) writeSitemapRelURL(sb *strings.Builder, rel string) {
	sb.WriteString(sitemapURLOpen)
	fmt.Fprintf(sb, "    <loc>%s%s%s</loc>\n", httpsScheme, g.config.Domain, rel)
	sb.WriteString("    <changefreq>weekly</changefreq>\n")
	sb.WriteString("    <priority>0.5</priority>\n")
	sb.WriteString(sitemapURLClose)
}

// generateTaxonomyFeeds writes one Atom feed per term for custom taxonomies
// with feed: true, in every language bucket.
func (g *Generator) generateTaxonomyFeeds(limit int) error {
	if g.taxonomies == nil {
		return nil
	}
	for _, name := range g.taxonomies.Names {
		def := g.taxonomies.Definitions[name]
		if def.Legacy || !def.Archive || !def.Feed {
			continue
		}
		if err := g.writeTaxonomyTermFeeds(def, limit); err != nil {
			return err
		}
	}
	return nil
}

// writeTaxonomyTermFeeds writes the per-term feeds of one taxonomy.
func (g *Generator) writeTaxonomyTermFeeds(def taxonomy.Definition, limit int) error {
	baseURL := httpsScheme + g.config.Domain
	for _, lang := range g.taxonomyLangs() {
		base := g.taxonomyBaseURL(def, lang)
		for _, t := range g.taxonomies.Terms(def.Name, lang) {
			if t.Count == 0 {
				continue
			}
			rel := strings.TrimPrefix(base, "/") + t.Slug + "/" + feedFileName
			posts := g.taxonomies.Pages(def.Name, lang, t.Key)
			if err := g.writeFeed(rel, t.Name, baseURL+base+t.Slug+"/", posts, limit); err != nil {
				return err
			}
		}
	}
	return nil
}
