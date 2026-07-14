package externalsource

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/spagu/ssg/internal/models"
)

// wordpressAdapter imports a WordPress database (plan phase 4): wp_posts,
// wp_users, wp_terms/wp_term_taxonomy/wp_term_relationships, wp_postmeta and
// attachments, mapped onto SSG's Page/Author models. Custom post types map to
// posts; the built-in "page" type maps to pages. Category and post_tag feed
// the legacy fields; other taxonomies land in the page's taxonomies map so
// dynamic taxonomies pick them up.
type wordpressAdapter struct{}

// wpTerm is one term with its taxonomy.
type wpTerm struct{ taxonomy, name string }

// Import loads and maps the WordPress content.
func (wordpressAdapter) Import(ctx context.Context, db *sql.DB, src Source) (*CMSImportResult, error) {
	opt := src.WordPress
	prefix := opt.TablePrefix
	if prefix == "" {
		prefix = "wp_"
	}
	if !identRe.MatchString(prefix) {
		return nil, fmt.Errorf("invalid table_prefix %q", prefix)
	}
	postTypes := opt.PostTypes
	if len(postTypes) == 0 {
		postTypes = []string{"post", "page"}
	}
	statuses := opt.Statuses
	if len(statuses) == 0 {
		statuses = []string{"publish"}
	}
	driver, _ := driverAndDSN(src)

	result := &CMSImportResult{Authors: map[int]models.Author{}, Taxonomies: map[string][]string{},
		Metadata: map[string]interface{}{"adapter": "wordpress", "table_prefix": prefix}}
	if err := wpAuthors(ctx, db, prefix, result); err != nil {
		return nil, err
	}
	postTerms, err := wpTerms(ctx, db, prefix, boolLayer(true, opt.IncludeTaxonomies), result)
	if err != nil {
		return nil, err
	}
	meta, err := wpPostMeta(ctx, db, prefix, boolLayer(true, opt.IncludeCustomFields))
	if err != nil {
		return nil, err
	}
	imp := wpImport{db: db, driver: driver, prefix: prefix, postTypes: postTypes,
		statuses: statuses, postTerms: postTerms, meta: meta}
	if err := wpPosts(ctx, imp, result); err != nil {
		return nil, err
	}
	if boolLayer(true, opt.IncludeMedia) {
		if err := wpMedia(ctx, db, prefix, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// wpAuthors loads wp_users into the unified author map.
func wpAuthors(ctx context.Context, db *sql.DB, prefix string, result *CMSImportResult) error {
	rows, err := queryMaps(ctx, db, "SELECT ID, display_name, user_nicename FROM "+prefix+"users")
	if err != nil {
		return err
	}
	for _, r := range rows {
		id := asInt(r["ID"])
		result.Authors[id] = models.Author{ID: id, Name: asString(r["display_name"]), Slug: asString(r["user_nicename"])}
	}
	return nil
}

// wpTerms loads the taxonomy tables and returns the object_id → terms
// assignment map.
func wpTerms(ctx context.Context, db *sql.DB, prefix string, include bool, result *CMSImportResult) (map[int][]wpTerm, error) {
	if !include {
		return map[int][]wpTerm{}, nil
	}
	rows, err := queryMaps(ctx, db, "SELECT tt.term_taxonomy_id, tt.taxonomy, t.name FROM "+
		prefix+"term_taxonomy tt JOIN "+prefix+"terms t ON t.term_id = tt.term_id")
	if err != nil {
		return nil, err
	}
	terms := make(map[int]wpTerm, len(rows))
	for _, r := range rows {
		term := wpTerm{taxonomy: asString(r["taxonomy"]), name: asString(r["name"])}
		terms[asInt(r["term_taxonomy_id"])] = term
		result.Taxonomies[term.taxonomy] = append(result.Taxonomies[term.taxonomy], term.name)
	}
	rels, err := queryMaps(ctx, db, "SELECT object_id, term_taxonomy_id FROM "+prefix+"term_relationships")
	if err != nil {
		return nil, err
	}
	postTerms := map[int][]wpTerm{}
	for _, r := range rels {
		if term, ok := terms[asInt(r["term_taxonomy_id"])]; ok {
			id := asInt(r["object_id"])
			postTerms[id] = append(postTerms[id], term)
		}
	}
	return postTerms, nil
}

// wpPostMeta loads user-facing custom fields (keys not starting with "_").
func wpPostMeta(ctx context.Context, db *sql.DB, prefix string, include bool) (map[int]map[string]interface{}, error) {
	if !include {
		return map[int]map[string]interface{}{}, nil
	}
	rows, err := queryMaps(ctx, db, "SELECT post_id, meta_key, meta_value FROM "+
		prefix+"postmeta WHERE substr(meta_key, 1, 1) <> '_'")
	if err != nil {
		return nil, err
	}
	meta := map[int]map[string]interface{}{}
	for _, r := range rows {
		id := asInt(r["post_id"])
		if meta[id] == nil {
			meta[id] = map[string]interface{}{}
		}
		meta[id][asString(r["meta_key"])] = asString(r["meta_value"])
	}
	return meta, nil
}

// wpImport bundles the loaded lookup tables for the post-mapping pass (S107).
type wpImport struct {
	db        *sql.DB
	driver    string
	prefix    string
	postTypes []string
	statuses  []string
	postTerms map[int][]wpTerm
	meta      map[int]map[string]interface{}
}

// wpPosts loads the selected post types/statuses and maps them to pages/posts.
func wpPosts(ctx context.Context, imp wpImport, result *CMSImportResult) error {
	query := "SELECT ID, post_title, post_name, post_content, post_excerpt, post_date, post_modified," +
		" post_status, post_type, post_author FROM " + imp.prefix + "posts WHERE post_type IN (" +
		inPlaceholders(imp.driver, 1, len(imp.postTypes)) + ") AND post_status IN (" +
		inPlaceholders(imp.driver, 1+len(imp.postTypes), len(imp.statuses)) + ")"
	args := make([]interface{}, 0, len(imp.postTypes)+len(imp.statuses))
	for _, t := range imp.postTypes {
		args = append(args, t)
	}
	for _, s := range imp.statuses {
		args = append(args, s)
	}
	rows, err := queryMaps(ctx, imp.db, query, args...)
	if err != nil {
		return err
	}
	for _, r := range rows {
		id := asInt(r["ID"])
		page := models.Page{
			ID: id, Title: asString(r["post_title"]), Slug: fallbackSlug(asString(r["post_name"]), asString(r["post_title"])),
			Content: asString(r["post_content"]), Excerpt: asString(r["post_excerpt"]),
			Date: cmsTime(r["post_date"]), Modified: cmsTime(r["post_modified"]),
			Status: asString(r["post_status"]), Type: asString(r["post_type"]),
			Author: asInt(r["post_author"]), Extra: imp.meta[id],
		}
		applyWPTerms(&page, imp.postTerms[id])
		if page.Type == "page" {
			result.Pages = append(result.Pages, page)
		} else {
			page.Type = "post" // custom post types render on the post pipeline
			result.Posts = append(result.Posts, page)
		}
	}
	return nil
}

// applyWPTerms routes term assignments: category/post_tag to the legacy
// fields, everything else into the generic taxonomies map.
func applyWPTerms(page *models.Page, terms []wpTerm) {
	for _, term := range terms {
		switch term.taxonomy {
		case "category":
			if page.Category == "" {
				page.Category = term.name
			}
			page.CategoriesRaw = append(page.CategoriesRaw, term.name)
		case "post_tag":
			page.Tags = append(page.Tags, term.name)
		default:
			if page.TaxonomiesFM == nil {
				page.TaxonomiesFM = map[string]interface{}{}
			}
			existing, _ := page.TaxonomiesFM[term.taxonomy].([]interface{})
			page.TaxonomiesFM[term.taxonomy] = append(existing, term.name)
		}
	}
}

// wpMedia loads attachments into the media collection.
func wpMedia(ctx context.Context, db *sql.DB, prefix string, result *CMSImportResult) error {
	rows, err := queryMaps(ctx, db, "SELECT ID, post_title, guid, post_mime_type FROM "+
		prefix+"posts WHERE post_type = 'attachment'")
	if err != nil {
		return err
	}
	for _, r := range rows {
		result.Media = append(result.Media, map[string]interface{}{
			"id": asInt(r["ID"]), "title": asString(r["post_title"]),
			"url": asString(r["guid"]), "mime_type": asString(r["post_mime_type"]),
		})
	}
	return nil
}
