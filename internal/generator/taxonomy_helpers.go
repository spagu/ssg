package generator

import (
	"html/template"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// TaxonomyInfo is the template view of one taxonomy definition.
type TaxonomyInfo struct {
	Name     string
	Label    string
	Singular string
	Path     string
	URL      string // archive root, e.g. /technology/ (empty for taxonomies without archives)
}

// TaxonomyTerm is the template view of one term.
type TaxonomyTerm struct {
	Name        string
	Slug        string
	URL         string
	Description string
	Count       int
	Weight      int
	Data        map[string]interface{}
}

// termView converts a registry term to its template view under an archive base URL.
func termView(t *taxonomy.Term, base string) TaxonomyTerm {
	url := ""
	if base != "" {
		url = base + t.Slug + "/"
	}
	return TaxonomyTerm{Name: t.Name, Slug: t.Slug, URL: url,
		Description: t.Description, Count: t.Count, Weight: t.Weight, Data: t.Data}
}

// termViews maps a sorted term list to template views.
func termViews(terms []*taxonomy.Term, base string) []TaxonomyTerm {
	out := make([]TaxonomyTerm, len(terms))
	for i, t := range terms {
		out[i] = termView(t, base)
	}
	return out
}

// taxonomyInfo builds the template view of a definition for the current language.
func (g *Generator) taxonomyInfo(def taxonomy.Definition, lang string) TaxonomyInfo {
	info := TaxonomyInfo{Name: def.Name, Label: def.Label, Singular: def.Singular, Path: def.Path}
	if def.Archive {
		if def.Legacy {
			info.URL = "/" + def.Path + "/" // legacy archives are never language-prefixed
		} else {
			info.URL = g.taxonomyBaseURL(def, lang)
		}
	}
	return info
}

// termBase is the URL prefix terms of a taxonomy live under ("" = no archive).
func (g *Generator) termBase(def taxonomy.Definition, lang string) string {
	return g.taxonomyInfo(def, lang).URL
}

// taxonomyFuncs exposes the taxonomy template helpers (taxonomies-feature.md).
// All helpers read the live registry and the current render language, so they
// return empty values gracefully when taxonomies are not built (e.g. unit
// tests driving renderTemplate directly).
func (g *Generator) taxonomyFuncs() template.FuncMap {
	return template.FuncMap{
		"taxonomies":    g.tmplTaxonomies,    // every definition, stable order
		"taxonomy":      g.tmplTaxonomy,      // one definition view (nil when undefined)
		"taxonomyTerms": g.tmplTaxonomyTerms, // sorted terms for the current language
		"pageTerms":     g.tmplPageTerms,     // a page's terms as full views
		"termURL":       g.tmplTermURL,       // /technology/go/ ("" when unknown)
		"hasTerm":       g.tmplHasTerm,       // normalized membership test
		"pagesByTerm":   g.tmplPagesByTerm,   // a term's posts, newest first
	}
}

// tmplTaxonomies lists every taxonomy definition (legacy + custom) in stable order.
func (g *Generator) tmplTaxonomies() []TaxonomyInfo {
	if g.taxonomies == nil {
		return nil
	}
	lang := g.taxonomyLang(g.currentLang)
	out := make([]TaxonomyInfo, 0, len(g.taxonomies.Names))
	for _, name := range g.taxonomies.Names {
		out = append(out, g.taxonomyInfo(g.taxonomies.Definitions[name], lang))
	}
	return out
}

// tmplTaxonomy returns one taxonomy's definition view, or nil when undefined.
func (g *Generator) tmplTaxonomy(name string) interface{} {
	if g.taxonomies == nil {
		return nil
	}
	def, ok := g.taxonomies.Definitions[name]
	if !ok {
		return nil
	}
	return g.taxonomyInfo(def, g.taxonomyLang(g.currentLang))
}

// tmplTaxonomyTerms returns a taxonomy's sorted terms for the current language.
func (g *Generator) tmplTaxonomyTerms(name string) []TaxonomyTerm {
	if g.taxonomies == nil {
		return nil
	}
	def, ok := g.taxonomies.Definitions[name]
	if !ok {
		return nil
	}
	lang := g.taxonomyLang(g.currentLang)
	return termViews(g.taxonomies.Terms(name, lang), g.termBase(def, lang))
}

// tmplTermURL resolves a term's archive URL ("" when the term is unknown).
func (g *Generator) tmplTermURL(name, term string) string {
	lang := g.taxonomyLang(g.currentLang)
	t := g.lookupTerm(name, lang, term)
	if t == nil {
		return ""
	}
	return termView(t, g.termBase(g.taxonomies.Definitions[name], lang)).URL
}

// tmplHasTerm reports whether a page carries a term (normalized comparison).
func (g *Generator) tmplHasTerm(name, term string, page any) bool {
	p, ok := pageFromAny(page)
	if !ok || g.taxonomies == nil {
		return false
	}
	def, ok := g.taxonomies.Definitions[name]
	if !ok {
		return false
	}
	want, _ := taxonomy.NormalizeKey(term, def.CaseSensitive)
	for _, v := range p.Taxonomies[name] {
		if key, _ := taxonomy.NormalizeKey(v, def.CaseSensitive); key == want {
			return true
		}
	}
	return false
}

// tmplPagesByTerm returns a term's posts, newest first.
func (g *Generator) tmplPagesByTerm(name, term string) []models.Page {
	lang := g.taxonomyLang(g.currentLang)
	t := g.lookupTerm(name, lang, term)
	if t == nil {
		return nil
	}
	return sortPostsByDate(g.taxonomies.Pages(name, lang, t.Key))
}

// tmplPageTerms resolves a page's terms for one taxonomy into template views.
func (g *Generator) tmplPageTerms(name string, page any) []TaxonomyTerm {
	p, ok := pageFromAny(page)
	if !ok || g.taxonomies == nil {
		return nil
	}
	def, ok := g.taxonomies.Definitions[name]
	if !ok {
		return nil
	}
	lang := g.taxonomyLang(p.Lang)
	base := g.termBase(def, lang)
	out := make([]TaxonomyTerm, 0, len(p.Taxonomies[name]))
	for _, v := range p.Taxonomies[name] {
		if t := g.taxonomies.Term(name, lang, v); t != nil {
			out = append(out, termView(t, base))
		} else {
			out = append(out, TaxonomyTerm{Name: v, Slug: slugify(v)})
		}
	}
	return out
}

// lookupTerm fetches a registry term by raw value, nil-safe on every level.
func (g *Generator) lookupTerm(name, lang, term string) *taxonomy.Term {
	if g.taxonomies == nil {
		return nil
	}
	if _, ok := g.taxonomies.Definitions[name]; !ok {
		return nil
	}
	return g.taxonomies.Term(name, lang, term)
}
