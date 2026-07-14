package externalsource

import (
	"context"
	"database/sql"

	"github.com/spagu/ssg/internal/models"
)

// movableTypeAdapter imports a Movable Type database (plan phase 6):
// mt_entry (entries and pages), mt_author, mt_category/mt_placement,
// mt_tag/mt_objecttag and mt_asset. Only released entries (status 2) are
// imported; comments are deferred.
type movableTypeAdapter struct{}

// mtStatusRelease is Movable Type's "published" entry status.
const mtStatusRelease = 2

// Import loads and maps the Movable Type content.
func (movableTypeAdapter) Import(ctx context.Context, db *sql.DB, src Source) (*CMSImportResult, error) {
	opt := src.MovableType
	result := &CMSImportResult{Authors: map[int]models.Author{}, Taxonomies: map[string][]string{},
		Metadata: map[string]interface{}{"adapter": "movable_type"}}

	if err := mtAuthors(ctx, db, result); err != nil {
		return nil, err
	}
	entryCats, err := mtCategories(ctx, db, result)
	if err != nil {
		return nil, err
	}
	entryTags, err := mtTags(ctx, db, result)
	if err != nil {
		return nil, err
	}
	classes := make([]string, 0, 2)
	if boolLayer(true, opt.IncludeEntries) {
		classes = append(classes, "entry")
	}
	if boolLayer(true, opt.IncludePages) {
		classes = append(classes, "page")
	}
	if len(classes) > 0 {
		if err := mtEntries(ctx, db, src, classes, entryCats, entryTags, result); err != nil {
			return nil, err
		}
	}
	if boolLayer(true, opt.IncludeAssets) {
		if err := mtAssets(ctx, db, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// mtAuthors loads mt_author.
func mtAuthors(ctx context.Context, db *sql.DB, result *CMSImportResult) error {
	rows, err := queryMaps(ctx, db, "SELECT author_id, author_name, author_nickname FROM mt_author")
	if err != nil {
		return err
	}
	for _, r := range rows {
		id := asInt(r["author_id"])
		name := asString(r["author_nickname"])
		if name == "" {
			name = asString(r["author_name"])
		}
		result.Authors[id] = models.Author{ID: id, Name: name, Slug: asString(r["author_name"])}
	}
	return nil
}

// mtCategories loads categories plus the entry placements (entry → labels).
func mtCategories(ctx context.Context, db *sql.DB, result *CMSImportResult) (map[int][]string, error) {
	rows, err := queryMaps(ctx, db, "SELECT category_id, category_label FROM mt_category")
	if err != nil {
		return nil, err
	}
	categories := map[int]string{}
	for _, r := range rows {
		label := asString(r["category_label"])
		categories[asInt(r["category_id"])] = label
		result.Taxonomies["category"] = append(result.Taxonomies["category"], label)
	}
	placements, err := queryMaps(ctx, db, "SELECT placement_entry_id, placement_category_id FROM mt_placement")
	if err != nil {
		return nil, err
	}
	entryCats := map[int][]string{}
	for _, r := range placements {
		if label, ok := categories[asInt(r["placement_category_id"])]; ok {
			id := asInt(r["placement_entry_id"])
			entryCats[id] = append(entryCats[id], label)
		}
	}
	return entryCats, nil
}

// mtTags loads entry tag assignments.
func mtTags(ctx context.Context, db *sql.DB, result *CMSImportResult) (map[int][]string, error) {
	rows, err := queryMaps(ctx, db, "SELECT ot.objecttag_object_id, t.tag_name FROM mt_objecttag ot "+
		"JOIN mt_tag t ON t.tag_id = ot.objecttag_tag_id WHERE ot.objecttag_object_datasource = 'entry'")
	if err != nil {
		return nil, err
	}
	out := map[int][]string{}
	seen := map[string]bool{}
	for _, r := range rows {
		id, tag := asInt(r["objecttag_object_id"]), asString(r["tag_name"])
		out[id] = append(out[id], tag)
		if !seen[tag] {
			seen[tag] = true
			result.Taxonomies["tag"] = append(result.Taxonomies["tag"], tag)
		}
	}
	return out, nil
}

// mtEntries loads released entries/pages and maps them onto Page models.
func mtEntries(ctx context.Context, db *sql.DB, src Source, classes []string,
	entryCats, entryTags map[int][]string, result *CMSImportResult) error {
	driver, _ := driverAndDSN(src)
	query := "SELECT entry_id, entry_title, entry_basename, entry_text, entry_text_more, entry_excerpt," +
		" entry_authored_on, entry_modified_on, entry_class, entry_author_id FROM mt_entry" +
		" WHERE entry_status = " + inPlaceholders(driver, 1, 1) +
		" AND entry_class IN (" + inPlaceholders(driver, 2, len(classes)) + ")"
	args := []interface{}{mtStatusRelease}
	for _, c := range classes {
		args = append(args, c)
	}
	rows, err := queryMaps(ctx, db, query, args...)
	if err != nil {
		return err
	}
	for _, r := range rows {
		id := asInt(r["entry_id"])
		content := asString(r["entry_text"])
		if more := asString(r["entry_text_more"]); more != "" {
			content += "\n\n" + more
		}
		page := models.Page{
			ID: id, Title: asString(r["entry_title"]), Slug: fallbackSlug(asString(r["entry_basename"]), asString(r["entry_title"])),
			Content: content, Excerpt: asString(r["entry_excerpt"]),
			Date: cmsTime(r["entry_authored_on"]), Modified: cmsTime(r["entry_modified_on"]),
			Status: "publish", Author: asInt(r["entry_author_id"]), Tags: entryTags[id],
		}
		if cats := entryCats[id]; len(cats) > 0 {
			page.Category = cats[0]
			for _, c := range cats {
				page.CategoriesRaw = append(page.CategoriesRaw, c)
			}
		}
		if asString(r["entry_class"]) == "page" {
			page.Type = "page"
			result.Pages = append(result.Pages, page)
		} else {
			page.Type = "post"
			result.Posts = append(result.Posts, page)
		}
	}
	return nil
}

// mtAssets loads mt_asset into the media collection.
func mtAssets(ctx context.Context, db *sql.DB, result *CMSImportResult) error {
	rows, err := queryMaps(ctx, db, "SELECT asset_id, asset_label, asset_url, asset_file_path FROM mt_asset")
	if err != nil {
		return err
	}
	for _, r := range rows {
		result.Media = append(result.Media, map[string]interface{}{
			"id": asInt(r["asset_id"]), "title": asString(r["asset_label"]),
			"url": asString(r["asset_url"]), "file_path": asString(r["asset_file_path"]),
		})
	}
	return nil
}
