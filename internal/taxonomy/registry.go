package taxonomy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// Term is one taxonomy value with its display name, slug and page count.
type Term struct {
	Taxonomy    string
	Key         string // normalized identity (lowercased unless case_sensitive)
	Name        string // display name (term metadata > first occurrence)
	Slug        string
	URL         string
	Description string
	Lang        string
	Count       int
	Weight      int
	Data        map[string]interface{}
}

// Registry holds every taxonomy definition, term and page assignment. Terms
// are tracked per (taxonomy, language, key) so multilingual builds get
// language-scoped archives; single-language builds use lang "".
type Registry struct {
	Definitions map[string]Definition
	Names       []string // deterministic ordering
	SlugFunc    func(string) string

	terms map[string]map[string]*Term         // taxonomy → lang\x00key → term
	pages map[string]map[string][]models.Page // taxonomy → lang\x00key → pages
}

// NewRegistry builds an empty registry over resolved definitions. slugFunc
// supplies the project's canonical slugifier so legacy URLs stay identical.
func NewRegistry(defs map[string]Definition, names []string, slugFunc func(string) string) *Registry {
	return &Registry{
		Definitions: defs,
		Names:       names,
		SlugFunc:    slugFunc,
		terms:       map[string]map[string]*Term{},
		pages:       map[string]map[string][]models.Page{},
	}
}

// NormalizeKey derives a term's identity: trimmed, inner whitespace collapsed,
// lowercased unless the taxonomy is case-sensitive. The display name is never
// altered beyond trimming.
func NormalizeKey(value string, caseSensitive bool) (key, display string) {
	display = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	key = display
	if !caseSensitive {
		key = strings.ToLower(display)
	}
	return key, display
}

func composite(lang, key string) string { return lang + "\x00" + key }

// Assign registers page under the given taxonomy values (already extracted from
// frontmatter) in the lang bucket ("" for single-language builds). Empty values
// are skipped; case-variants merge into one term.
func (r *Registry) Assign(taxonomy, lang string, values []string, page models.Page) error {
	def, ok := r.Definitions[taxonomy]
	if !ok {
		return fmt.Errorf("unknown taxonomy %q", taxonomy)
	}
	seen := map[string]bool{}
	for _, raw := range values {
		key, display := NormalizeKey(raw, def.CaseSensitive)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		ck := composite(lang, key)
		if r.terms[taxonomy] == nil {
			r.terms[taxonomy] = map[string]*Term{}
			r.pages[taxonomy] = map[string][]models.Page{}
		}
		term := r.terms[taxonomy][ck]
		if term == nil {
			slug := display
			if def.Slugify && r.SlugFunc != nil {
				slug = r.SlugFunc(display)
			}
			term = &Term{Taxonomy: taxonomy, Key: key, Name: display, Slug: slug, Lang: lang}
			r.terms[taxonomy][ck] = term
		}
		term.Count++
		r.pages[taxonomy][ck] = append(r.pages[taxonomy][ck], page)
	}
	return nil
}

// ApplyTermMeta overlays data/taxonomies/<name>.yaml metadata onto a term
// (creating it when generate_empty semantics need it later): name, slug,
// description, weight and free-form data win over content-derived values.
func (r *Registry) ApplyTermMeta(taxonomy, rawKey string, meta map[string]interface{}, lang string) {
	def, ok := r.Definitions[taxonomy]
	if !ok {
		return
	}
	key, display := NormalizeKey(rawKey, def.CaseSensitive)
	ck := composite(lang, key)
	if r.terms[taxonomy] == nil {
		r.terms[taxonomy] = map[string]*Term{}
		r.pages[taxonomy] = map[string][]models.Page{}
	}
	term := r.terms[taxonomy][ck]
	if term == nil {
		slug := display
		if def.Slugify && r.SlugFunc != nil {
			slug = r.SlugFunc(display)
		}
		term = &Term{Taxonomy: taxonomy, Key: key, Name: display, Slug: slug, Lang: lang}
		r.terms[taxonomy][ck] = term
	}
	if v, ok := meta["name"].(string); ok && v != "" {
		term.Name = v
	}
	if v, ok := meta["slug"].(string); ok && v != "" {
		term.Slug = v
	}
	if v, ok := meta["description"].(string); ok {
		term.Description = v
	}
	switch w := meta["weight"].(type) {
	case int:
		term.Weight = w
	case float64:
		term.Weight = int(w)
	}
	if d, ok := meta["data"].(map[string]interface{}); ok {
		term.Data = d
	}
}

// Terms returns a taxonomy's terms for one language, deterministically ordered
// by the definition's sort (name asc | count desc | weight desc; name breaks
// ties). Empty terms are excluded unless generate_empty is set.
func (r *Registry) Terms(taxonomy, lang string) []*Term {
	def := r.Definitions[taxonomy]
	out := make([]*Term, 0, len(r.terms[taxonomy]))
	for ck, t := range r.terms[taxonomy] {
		if !strings.HasPrefix(ck, lang+"\x00") {
			continue
		}
		if t.Count == 0 && !def.GenerateEmpty {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch def.Sort {
		case "count":
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
		case "weight":
			if out[i].Weight != out[j].Weight {
				return out[i].Weight > out[j].Weight
			}
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Pages returns the pages assigned to a term (registration order).
func (r *Registry) Pages(taxonomy, lang, key string) []models.Page {
	return r.pages[taxonomy][composite(lang, key)]
}

// Term looks a term up by raw value (normalized internally).
func (r *Registry) Term(taxonomy, lang, raw string) *Term {
	def, ok := r.Definitions[taxonomy]
	if !ok {
		return nil
	}
	key, _ := NormalizeKey(raw, def.CaseSensitive)
	return r.terms[taxonomy][composite(lang, key)]
}

// ValidateSlugs fails on two distinct terms of one taxonomy+language sharing a
// slug (e.g. "C++" and "C#" both slugifying to "c"): ask for a manual override.
func (r *Registry) ValidateSlugs() error {
	for _, name := range r.Names {
		byLangSlug := map[string]string{}
		for _, t := range r.terms[name] {
			sk := t.Lang + "\x00" + t.Slug
			if prev, dup := byLangSlug[sk]; dup && prev != t.Name {
				return fmt.Errorf("taxonomy %q: terms %q and %q collide on slug %q — set an explicit slug in data/taxonomies/%s.yaml",
					name, prev, t.Name, t.Slug, name)
			}
			byLangSlug[sk] = t.Name
		}
	}
	return nil
}
