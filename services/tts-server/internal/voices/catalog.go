package voices

import (
	"cmp"
	"slices"
)

// Catalog is an immutable snapshot of the available voices.
type Catalog struct {
	byID     map[string]Voice
	defaults map[string]string // normalised lang -> voice id (voices.yaml)
}

func newCatalog() Catalog {
	return Catalog{byID: map[string]Voice{}, defaults: map[string]string{}}
}

// add inserts or replaces a voice.
func (c Catalog) add(v Voice) { c.byID[v.ID] = v }

// Len is the number of voices.
func (c Catalog) Len() int { return len(c.byID) }

// Get looks a voice up by id.
func (c Catalog) Get(id string) (Voice, bool) {
	v, ok := c.byID[id]
	return v, ok
}

// List returns every voice, Piper first, then by id, with Default set on the
// voice Resolve would pick for that voice's own language.
func (c Catalog) List() []Voice {
	out := make([]Voice, 0, len(c.byID))
	for _, v := range c.byID {
		if d, ok := c.Resolve(v.Lang); ok && d.ID == v.ID {
			v.Default = true
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b Voice) int {
		return cmp.Or(cmp.Compare(engineRank(a), engineRank(b)), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// Resolve picks the default voice for a language tag. A voices.yaml default
// for the exact tag or its primary subtag wins; otherwise Piper beats espeak,
// an exact language beats a related one, and espeak's own alias priorities
// (en -> en-gb before en-029) break the remaining ties.
func (c Catalog) Resolve(lang string) (Voice, bool) {
	n := NormalizeLang(lang)
	for _, key := range []string{n, primary(n)} {
		if v, ok := c.byID[c.defaults[key]]; ok {
			return v, true
		}
	}
	var best Voice
	var bestScore []int
	for _, v := range c.byID {
		s, ok := score(v, n)
		if !ok {
			continue
		}
		if bestScore == nil || slices.Compare(s, bestScore) < 0 || (slices.Equal(s, bestScore) && v.ID < best.ID) {
			best, bestScore = v, s
		}
	}
	return best, bestScore != nil
}

// noAlias ranks a related-language match that espeak did not prioritise.
const noAlias = 99

// score orders candidate voices for lang; ok is false when v cannot speak it.
func score(v Voice, lang string) ([]int, bool) {
	vl := NormalizeLang(v.Lang)
	if vl == lang {
		return []int{engineRank(v), 0, 0}, true
	}
	for _, key := range []string{lang, primary(lang)} {
		if p, ok := v.Aliases[key]; ok {
			return []int{engineRank(v), 1, p}, true
		}
	}
	if primary(vl) == primary(lang) {
		return []int{engineRank(v), 1, noAlias}, true
	}
	return nil, false
}

func engineRank(v Voice) int {
	if v.Engine == EnginePiper {
		return 0
	}
	return 1
}

// CatalogOf builds a catalog from a fixed list of voices (used by tests and
// by embedders that do not read a voices directory).
func CatalogOf(vs ...Voice) Catalog {
	c := newCatalog()
	for _, v := range vs {
		c.add(v)
	}
	return c
}
