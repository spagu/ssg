package externalsource

import (
	"testing"
	"time"
)

// adapterDDL lists each adapter's required tables in query order, so building
// fixtures from prefixes of the list exercises every per-table error branch.
var adapterDDL = map[string][]string{
	"wordpress": {
		`CREATE TABLE wp_users (ID INTEGER, display_name TEXT, user_nicename TEXT)`,
		`CREATE TABLE wp_term_taxonomy (term_taxonomy_id INTEGER, term_id INTEGER, taxonomy TEXT)`,
		`CREATE TABLE wp_terms (term_id INTEGER, name TEXT, slug TEXT)`,
		`CREATE TABLE wp_term_relationships (object_id INTEGER, term_taxonomy_id INTEGER)`,
		`CREATE TABLE wp_postmeta (post_id INTEGER, meta_key TEXT, meta_value TEXT)`,
		`CREATE TABLE wp_posts (ID INTEGER, post_title TEXT, post_name TEXT, post_content TEXT,
		 post_excerpt TEXT, post_date TEXT, post_modified TEXT, post_status TEXT, post_type TEXT,
		 post_author INTEGER, guid TEXT, post_mime_type TEXT)`,
	},
	"drupal": {
		`CREATE TABLE users_field_data (uid INTEGER, name TEXT)`,
		`CREATE TABLE node__body (entity_id INTEGER, body_value TEXT, body_summary TEXT)`,
		`CREATE TABLE taxonomy_term_field_data (tid INTEGER, name TEXT, vid TEXT)`,
		`CREATE TABLE taxonomy_index (nid INTEGER, tid INTEGER)`,
		`CREATE TABLE path_alias (path TEXT, alias TEXT)`,
		`CREATE TABLE node_field_data (nid INTEGER, type TEXT, title TEXT, status INTEGER,
		 created INTEGER, changed INTEGER, uid INTEGER)`,
	},
	"movable_type": {
		`CREATE TABLE mt_author (author_id INTEGER, author_name TEXT, author_nickname TEXT)`,
		`CREATE TABLE mt_category (category_id INTEGER, category_label TEXT, category_basename TEXT)`,
		`CREATE TABLE mt_placement (placement_entry_id INTEGER, placement_category_id INTEGER)`,
		`CREATE TABLE mt_objecttag (objecttag_object_id INTEGER, objecttag_tag_id INTEGER, objecttag_object_datasource TEXT)`,
		`CREATE TABLE mt_tag (tag_id INTEGER, tag_name TEXT)`,
		`CREATE TABLE mt_entry (entry_id INTEGER, entry_title TEXT, entry_basename TEXT, entry_text TEXT,
		 entry_text_more TEXT, entry_excerpt TEXT, entry_authored_on TEXT, entry_modified_on TEXT,
		 entry_status INTEGER, entry_class TEXT, entry_author_id INTEGER)`,
		`CREATE TABLE mt_asset (asset_id INTEGER, asset_label TEXT, asset_url TEXT, asset_file_path TEXT)`,
	},
}

// TestCMSAdapterMissingTables walks every prefix of each adapter's schema:
// incomplete databases fail with the unified error; the full (empty) schema
// imports zero content cleanly.
func TestCMSAdapterMissingTables(t *testing.T) {
	for adapter, ddl := range adapterDDL {
		for i := 0; i <= len(ddl); i++ {
			src := cmsSource(adapter, buildFixtureDB(t, ddl[:i]), "content")
			_, err := CMSConnector{}.Load(src)
			if i < len(ddl) && err == nil {
				t.Errorf("%s with %d/%d tables: expected error", adapter, i, len(ddl))
			}
			if i == len(ddl) && err != nil {
				t.Errorf("%s with full schema: %v", adapter, err)
			}
		}
	}
}

func TestRegistryCMSImports(t *testing.T) {
	src := cmsSource("movable_type", buildFixtureDB(t, adapterDDL["movable_type"]), "content")
	res, err := CMSConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	reg := &Registry{Order: []string{"mt", "plain"}, Results: map[string]*Result{
		"mt":    res,
		"plain": {Name: "plain", Type: "file"},
	}}
	imports := reg.CMSImports()
	if len(imports) != 1 || imports[0] != res.CMS {
		t.Fatalf("imports = %+v", imports)
	}
}

func TestNormalizeSQLValueTime(t *testing.T) {
	ts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if normalizeSQLValue(ts) != ts || normalizeSQLValue([]byte("x")) != "x" || normalizeSQLValue(5) != 5 {
		t.Fatal("normalizeSQLValue")
	}
}
