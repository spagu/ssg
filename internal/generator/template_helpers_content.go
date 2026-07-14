// P2 content helpers: latest, published, byTag, byCategory, byAuthor, related
// (v1.8.3, audit/feature.md). These are thin wrappers over the generic helpers;
// byCategory/byAuthor additionally resolve names/slugs via site metadata and are
// therefore Generator methods restricted to []models.Page.
package generator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// tmplLatest sorts by a field descending and keeps the newest count elements:
//
//	{{ .Site.Pages | latest "Modified" 5 }}
func tmplLatest(field string, count int, collection any) (any, error) {
	sorted, err := tmplSortBy(field, "desc", collection)
	if err != nil {
		return nil, fmt.Errorf("latest: %w", err)
	}
	out, err := tmplFirst(count, sorted)
	if err != nil {
		return nil, fmt.Errorf("latest: %w", err)
	}
	return out, nil
}

// tmplPublished keeps elements whose Status field equals "publish":
//
//	{{ .Site.Pages | published }}
func tmplPublished(collection any) (any, error) {
	out, err := tmplWhere("Status", "publish", collection)
	if err != nil {
		return nil, fmt.Errorf("published: %w", err)
	}
	return out, nil
}

// tmplByTag keeps elements whose Tags slice contains tag:
//
//	{{ .Site.Pages | byTag "go" }}
func tmplByTag(tag string, collection any) (any, error) {
	out, err := tmplFilter("Tags", "contains", tag, collection)
	if err != nil {
		return nil, fmt.Errorf("byTag: %w", err)
	}
	return out, nil
}

// asPages coerces a helper collection argument to []models.Page.
func asPages(collection any, helperName string) ([]models.Page, error) {
	pages, ok := collection.([]models.Page)
	if !ok {
		return nil, fmt.Errorf("%s: expected []models.Page, got %T", helperName, collection)
	}
	return pages, nil
}

// tmplByCategory keeps pages matching a category by name or slug
// (case-insensitive), resolving Categories IDs through site metadata:
//
//	{{ .Site.Posts | byCategory "guides" }}
func (g *Generator) tmplByCategory(name string, collection any) (any, error) {
	pages, err := asPages(collection, "byCategory")
	if err != nil {
		return nil, err
	}
	out := make([]models.Page, 0, len(pages))
	for _, p := range pages {
		if g.pageInCategory(p, name) {
			out = append(out, p)
		}
	}
	return out, nil
}

// pageInCategory reports whether a page belongs to the named category, matching
// the frontmatter Category string or any resolved Categories ID's name/slug.
func (g *Generator) pageInCategory(p models.Page, name string) bool {
	if strings.EqualFold(p.Category, name) {
		return true
	}
	for _, id := range p.Categories {
		if cat, ok := g.siteData.Categories[id]; ok &&
			(strings.EqualFold(cat.Name, name) || strings.EqualFold(cat.Slug, name)) {
			return true
		}
	}
	return false
}

// tmplByAuthor keeps pages by author ID, name or slug (case-insensitive):
//
//	{{ .Site.Posts | byAuthor "jan-kowalski" }}
func (g *Generator) tmplByAuthor(author string, collection any) (any, error) {
	pages, err := asPages(collection, "byAuthor")
	if err != nil {
		return nil, err
	}
	wantID, idErr := strconv.Atoi(strings.TrimSpace(author))
	byID := idErr == nil
	out := make([]models.Page, 0, len(pages))
	for _, p := range pages {
		if byID && p.Author == wantID {
			out = append(out, p)
			continue
		}
		if a, ok := g.siteData.Authors[p.Author]; ok &&
			(strings.EqualFold(a.Name, author) || strings.EqualFold(a.Slug, author)) {
			out = append(out, p)
		}
	}
	return out, nil
}

// relatedScore ranks candidate pages against the current one: shared tags weigh
// most, then shared categories, then the same author.
func relatedScore(current, candidate models.Page) int {
	score := 0
	tags := map[string]bool{}
	for _, t := range current.Tags {
		tags[strings.ToLower(t)] = true
	}
	for _, t := range candidate.Tags {
		if tags[strings.ToLower(t)] {
			score += 3
		}
	}
	cats := map[int]bool{}
	for _, c := range current.Categories {
		cats[c] = true
	}
	for _, c := range candidate.Categories {
		if cats[c] {
			score += 2
		}
	}
	if current.Author != 0 && current.Author == candidate.Author {
		score++
	}
	return score
}

// tmplRelated returns up to count pages related to current (shared tags, then
// categories, then author; recency breaks ties). The current page is excluded:
//
//	{{ .Site.Posts | related .Page 3 }}
func tmplRelated(current models.Page, count int, collection any) ([]models.Page, error) {
	if count < 0 {
		return nil, fmt.Errorf("related: count must be greater than or equal to zero")
	}
	pages, err := asPages(collection, "related")
	if err != nil {
		return nil, err
	}
	type scored struct {
		page  models.Page
		score int
	}
	candidates := make([]scored, 0, len(pages))
	for _, p := range pages {
		if p.ID == current.ID && p.Slug == current.Slug {
			continue // never suggest the page itself
		}
		if s := relatedScore(current, p); s > 0 {
			candidates = append(candidates, scored{page: p, score: s})
		}
	}
	sort.SliceStable(candidates, func(a, b int) bool {
		if candidates[a].score != candidates[b].score {
			return candidates[a].score > candidates[b].score
		}
		return candidates[a].page.Date.After(candidates[b].page.Date)
	})
	if count > len(candidates) {
		count = len(candidates)
	}
	out := make([]models.Page, 0, count)
	for _, c := range candidates[:count] {
		out = append(out, c.page)
	}
	return out, nil
}
