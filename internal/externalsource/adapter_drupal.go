package externalsource

import (
	"context"
	"database/sql"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// drupalAdapter imports a Drupal 8-11 database (plan phase 5):
// node_field_data, node__body, users_field_data, taxonomy_term_field_data,
// taxonomy_index and path_alias, plus dynamic node__field_* tables into
// .Extra. Path aliases become explicit Links so Drupal URLs survive the
// migration; taxonomy terms land in the generic taxonomies map. Drupal 7 has
// a different schema and stays a separate, deferred adapter.
type drupalAdapter struct{}

// Import loads and maps the Drupal content.
func (drupalAdapter) Import(ctx context.Context, db *sql.DB, src Source) (*CMSImportResult, error) {
	opt := src.Drupal
	bundles := opt.Bundles
	if len(bundles) == 0 {
		bundles = []string{"article", "page"}
	}
	driver, _ := driverAndDSN(src)
	result := &CMSImportResult{Authors: map[int]models.Author{}, Taxonomies: map[string][]string{},
		Metadata: map[string]interface{}{"adapter": "drupal", "version": opt.Version}}

	if err := drupalAuthors(ctx, db, result); err != nil {
		return nil, err
	}
	bodies, err := drupalBodies(ctx, db)
	if err != nil {
		return nil, err
	}
	nodeTerms, err := drupalTerms(ctx, db, result)
	if err != nil {
		return nil, err
	}
	aliases, err := drupalAliases(ctx, db)
	if err != nil {
		return nil, err
	}
	fields := map[int]map[string]interface{}{}
	if boolLayer(true, opt.IncludeFields) {
		if fields, err = drupalFields(ctx, db, driver); err != nil {
			return nil, err
		}
	}

	query := "SELECT nid, type, title, status, created, changed, uid FROM node_field_data" +
		" WHERE type IN (" + inPlaceholders(driver, 1, len(bundles)) + ")"
	if boolLayer(true, opt.PublishedOnly) {
		query += " AND status = 1"
	}
	args := make([]interface{}, len(bundles))
	for i, b := range bundles {
		args[i] = b
	}
	rows, err := queryMaps(ctx, db, query, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		nid := asInt(r["nid"])
		page := models.Page{
			ID: nid, Title: asString(r["title"]), Slug: slugFromAlias(aliases[nid], asString(r["title"])),
			Link:    aliases[nid],
			Content: asString(bodies[nid]["value"]), Excerpt: asString(bodies[nid]["summary"]),
			Date: cmsTime(r["created"]), Modified: cmsTime(r["changed"]),
			Status: drupalStatus(asInt(r["status"])), Author: asInt(r["uid"]), Extra: fields[nid],
		}
		for vocab, names := range nodeTerms[nid] {
			if page.TaxonomiesFM == nil {
				page.TaxonomiesFM = map[string]interface{}{}
			}
			page.TaxonomiesFM[vocab] = names
		}
		if asString(r["type"]) == "page" {
			page.Type = "page"
			result.Pages = append(result.Pages, page)
		} else {
			page.Type = "post"
			result.Posts = append(result.Posts, page)
		}
	}
	return result, nil
}

// drupalStatus maps Drupal's published flag to SSG statuses.
func drupalStatus(status int) string {
	if status == 1 {
		return "publish"
	}
	return "draft"
}

// slugFromAlias derives a slug from a path alias, falling back to the title.
func slugFromAlias(alias, title string) string {
	trimmed := strings.Trim(alias, "/")
	if trimmed != "" {
		parts := strings.Split(trimmed, "/")
		return parts[len(parts)-1]
	}
	return strings.ToLower(strings.Join(strings.Fields(title), "-"))
}

// drupalAuthors loads users_field_data.
func drupalAuthors(ctx context.Context, db *sql.DB, result *CMSImportResult) error {
	rows, err := queryMaps(ctx, db, "SELECT uid, name FROM users_field_data WHERE uid > 0")
	if err != nil {
		return err
	}
	for _, r := range rows {
		uid := asInt(r["uid"])
		result.Authors[uid] = models.Author{ID: uid, Name: asString(r["name"]), Slug: asString(r["name"])}
	}
	return nil
}

// drupalBodies loads node__body into nid → {value, summary}.
func drupalBodies(ctx context.Context, db *sql.DB) (map[int]map[string]string, error) {
	rows, err := queryMaps(ctx, db, "SELECT entity_id, body_value, body_summary FROM node__body")
	if err != nil {
		return nil, err
	}
	out := map[int]map[string]string{}
	for _, r := range rows {
		out[asInt(r["entity_id"])] = map[string]string{
			"value": asString(r["body_value"]), "summary": asString(r["body_summary"])}
	}
	return out, nil
}

// drupalTerms loads vocabulary terms and the node assignments:
// nid → vocabulary → []term names.
func drupalTerms(ctx context.Context, db *sql.DB, result *CMSImportResult) (map[int]map[string][]interface{}, error) {
	terms, err := queryMaps(ctx, db, "SELECT tid, name, vid FROM taxonomy_term_field_data")
	if err != nil {
		return nil, err
	}
	byTid := map[int]struct{ vocab, name string }{}
	for _, r := range terms {
		vocab, name := asString(r["vid"]), asString(r["name"])
		byTid[asInt(r["tid"])] = struct{ vocab, name string }{vocab, name}
		result.Taxonomies[vocab] = append(result.Taxonomies[vocab], name)
	}
	index, err := queryMaps(ctx, db, "SELECT nid, tid FROM taxonomy_index")
	if err != nil {
		return nil, err
	}
	out := map[int]map[string][]interface{}{}
	for _, r := range index {
		term, ok := byTid[asInt(r["tid"])]
		if !ok {
			continue
		}
		nid := asInt(r["nid"])
		if out[nid] == nil {
			out[nid] = map[string][]interface{}{}
		}
		out[nid][term.vocab] = append(out[nid][term.vocab], term.name)
	}
	return out, nil
}

// drupalAliases loads path_alias into nid → alias.
func drupalAliases(ctx context.Context, db *sql.DB) (map[int]string, error) {
	rows, err := queryMaps(ctx, db, "SELECT path, alias FROM path_alias")
	if err != nil {
		return nil, err
	}
	out := map[int]string{}
	for _, r := range rows {
		path := asString(r["path"])
		if nid := asInt(strings.TrimPrefix(path, "/node/")); nid > 0 && strings.HasPrefix(path, "/node/") {
			out[nid] = asString(r["alias"])
		}
	}
	return out, nil
}

// drupalFields discovers node__field_* tables and loads their values into
// nid → field name → value(s).
func drupalFields(ctx context.Context, db *sql.DB, driver string) (map[int]map[string]interface{}, error) {
	tables, err := listTables(ctx, db, driver, "node__field_%")
	if err != nil {
		return nil, err
	}
	out := map[int]map[string]interface{}{}
	for _, table := range tables {
		if !identRe.MatchString(table) {
			continue
		}
		field := strings.TrimPrefix(table, "node__")
		rows, err := queryMaps(ctx, db, "SELECT entity_id, "+field+"_value AS v FROM "+table)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			nid := asInt(r["entity_id"])
			if out[nid] == nil {
				out[nid] = map[string]interface{}{}
			}
			out[nid][field] = asString(r["v"])
		}
	}
	return out, nil
}

// listTables enumerates tables matching a LIKE pattern per engine.
func listTables(ctx context.Context, db *sql.DB, driver, like string) ([]string, error) {
	var query string
	switch driver {
	case "pgx":
		query = "SELECT table_name AS name FROM information_schema.tables WHERE table_schema = current_schema() AND table_name LIKE $1"
	case "sqlite":
		query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE ?"
	default: // mysql
		query = "SELECT table_name AS name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name LIKE ?"
	}
	rows, err := queryMaps(ctx, db, query, like)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, asString(r["name"]))
	}
	return out, nil
}
