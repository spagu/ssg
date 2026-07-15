package externalsource

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildFixtureDB executes DDL/DML statements into a fresh sqlite file.
func buildFixtureDB(t *testing.T, stmts []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cms.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

// cmsSource builds a resolved CMS source over a sqlite fixture.
func cmsSource(adapter, dbPath, mode string) Source {
	return Source{Name: adapter, Type: "cms", Adapter: adapter, Mode: mode,
		Driver: "sqlite", Database: dbPath, Required: true, Timeout: 5 * time.Second}
}

// ─── WordPress ───────────────────────────────────────────────────────────────

func wordpressFixture(t *testing.T) string {
	t.Helper()
	return buildFixtureDB(t, []string{
		`CREATE TABLE wp_posts (ID INTEGER, post_title TEXT, post_name TEXT, post_content TEXT,
		 post_excerpt TEXT, post_date TEXT, post_modified TEXT, post_status TEXT, post_type TEXT,
		 post_author INTEGER, guid TEXT, post_mime_type TEXT)`,
		`INSERT INTO wp_posts VALUES (1, 'Hello world', 'hello-world', '<p>Welcome!</p>', 'Hi',
		 '2026-01-05 10:00:00', '2026-01-06 10:00:00', 'publish', 'post', 1, '', '')`,
		`INSERT INTO wp_posts VALUES (2, 'About', 'about', '<p>About us</p>', '',
		 '2026-01-01 08:00:00', '2026-01-01 08:00:00', 'publish', 'page', 1, '', '')`,
		`INSERT INTO wp_posts VALUES (3, 'Draft', 'draft', 'x', '', '2026-01-02 08:00:00',
		 '2026-01-02 08:00:00', 'draft', 'post', 1, '', '')`,
		`INSERT INTO wp_posts VALUES (4, 'Guide one', 'guide-one', 'g', '', '2026-01-03 08:00:00',
		 '2026-01-03 08:00:00', 'publish', 'guide', 1, '', '')`,
		`INSERT INTO wp_posts VALUES (9, 'Logo', '', 'x', '', '2026-01-01 08:00:00',
		 '2026-01-01 08:00:00', 'inherit', 'attachment', 1, 'https://example.com/logo.png', 'image/png')`,
		`CREATE TABLE wp_users (ID INTEGER, display_name TEXT, user_nicename TEXT)`,
		`INSERT INTO wp_users VALUES (1, 'Ed Writer', 'ed')`,
		`CREATE TABLE wp_terms (term_id INTEGER, name TEXT, slug TEXT)`,
		`INSERT INTO wp_terms VALUES (10, 'News', 'news')`,
		`INSERT INTO wp_terms VALUES (11, 'golang', 'golang')`,
		`INSERT INTO wp_terms VALUES (12, 'Poland', 'poland')`,
		`CREATE TABLE wp_term_taxonomy (term_taxonomy_id INTEGER, term_id INTEGER, taxonomy TEXT)`,
		`INSERT INTO wp_term_taxonomy VALUES (100, 10, 'category')`,
		`INSERT INTO wp_term_taxonomy VALUES (101, 11, 'post_tag')`,
		`INSERT INTO wp_term_taxonomy VALUES (102, 12, 'region')`,
		`CREATE TABLE wp_term_relationships (object_id INTEGER, term_taxonomy_id INTEGER)`,
		`INSERT INTO wp_term_relationships VALUES (1, 100)`,
		`INSERT INTO wp_term_relationships VALUES (1, 101)`,
		`INSERT INTO wp_term_relationships VALUES (1, 102)`,
		`CREATE TABLE wp_postmeta (post_id INTEGER, meta_key TEXT, meta_value TEXT)`,
		`INSERT INTO wp_postmeta VALUES (1, 'mood', 'sunny')`,
		`INSERT INTO wp_postmeta VALUES (1, '_edit_lock', 'internal')`,
	})
}

func TestWordPressAdapter(t *testing.T) {
	src := cmsSource("wordpress", wordpressFixture(t), "content")
	src.WordPress.PostTypes = []string{"post", "page", "guide"}
	res, err := CMSConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	imp := res.CMS
	if imp == nil || len(imp.Posts) != 2 || len(imp.Pages) != 1 {
		t.Fatalf("imported = %+v", imp)
	}
	post := imp.Posts[0]
	if post.Title != "Hello world" || post.Slug != "hello-world" || post.Status != "publish" ||
		post.Category != "News" || post.Tags[0] != "golang" || post.Author != 1 {
		t.Fatalf("post = %+v", post)
	}
	if terms, ok := post.TaxonomiesFM["region"].([]interface{}); !ok || terms[0] != "Poland" {
		t.Fatalf("custom taxonomy = %#v", post.TaxonomiesFM)
	}
	if post.Extra["mood"] != "sunny" {
		t.Fatalf("custom fields = %#v", post.Extra)
	}
	if _, leaked := post.Extra["_edit_lock"]; leaked {
		t.Fatal("internal meta keys must be excluded")
	}
	// Custom post type "guide" renders on the post pipeline.
	if imp.Posts[1].Type != "post" || imp.Posts[1].Title != "Guide one" {
		t.Fatalf("custom post type = %+v", imp.Posts[1])
	}
	if imp.Authors[1].Name != "Ed Writer" || imp.Authors[1].Slug != "ed" {
		t.Fatalf("authors = %+v", imp.Authors)
	}
	if len(imp.Media) != 1 || imp.Media[0]["url"] != "https://example.com/logo.png" {
		t.Fatalf("media = %+v", imp.Media)
	}
	if res.Metadata.ContentType != "wordpress" || res.Metadata.RecordCount != 3 {
		t.Fatalf("meta = %+v", res.Metadata)
	}
	// Data view is exposed for templates in both modes.
	view := res.Data.(map[string]interface{})
	if len(view["posts"].([]interface{})) != 2 || len(view["authors"].([]interface{})) != 1 {
		t.Fatalf("data view = %#v", view)
	}
	// mode: data must NOT mark the import for content merging.
	src.Mode = "data"
	res2, err := CMSConnector{}.Load(src)
	if err != nil || res2.CMS != nil {
		t.Fatalf("data mode: %+v %v", res2, err)
	}
	// Invalid table prefix is rejected.
	src.WordPress.TablePrefix = "wp_'; DROP TABLE--"
	if _, err := (CMSConnector{}).Load(src); err == nil || !strings.Contains(err.Error(), "table_prefix") {
		t.Fatalf("prefix err = %v", err)
	}
}

// ─── Drupal ──────────────────────────────────────────────────────────────────

func drupalFixture(t *testing.T) string {
	t.Helper()
	return buildFixtureDB(t, []string{
		`CREATE TABLE node_field_data (nid INTEGER, type TEXT, title TEXT, status INTEGER,
		 created INTEGER, changed INTEGER, uid INTEGER)`,
		`INSERT INTO node_field_data VALUES (1, 'article', 'First article', 1, 1767225600, 1767312000, 5)`,
		`INSERT INTO node_field_data VALUES (2, 'page', 'Company', 1, 1767225600, 1767225600, 5)`,
		`INSERT INTO node_field_data VALUES (3, 'article', 'Unpublished', 0, 1767225600, 1767225600, 5)`,
		`CREATE TABLE node__body (entity_id INTEGER, body_value TEXT, body_summary TEXT)`,
		`INSERT INTO node__body VALUES (1, '<p>Body one</p>', 'Summary one')`,
		`INSERT INTO node__body VALUES (2, '<p>Company page</p>', '')`,
		`CREATE TABLE users_field_data (uid INTEGER, name TEXT)`,
		`INSERT INTO users_field_data VALUES (5, 'editor')`,
		`INSERT INTO users_field_data VALUES (0, 'anonymous')`,
		`CREATE TABLE taxonomy_term_field_data (tid INTEGER, name TEXT, vid TEXT)`,
		`INSERT INTO taxonomy_term_field_data VALUES (7, 'Research', 'topics')`,
		`CREATE TABLE taxonomy_index (nid INTEGER, tid INTEGER)`,
		`INSERT INTO taxonomy_index VALUES (1, 7)`,
		`CREATE TABLE path_alias (path TEXT, alias TEXT)`,
		`INSERT INTO path_alias VALUES ('/node/1', '/blog/first-article')`,
		`CREATE TABLE node__field_subtitle (entity_id INTEGER, field_subtitle_value TEXT)`,
		`INSERT INTO node__field_subtitle VALUES (1, 'A closer look')`,
	})
}

func TestDrupalAdapter(t *testing.T) {
	src := cmsSource("drupal", drupalFixture(t), "content")
	res, err := CMSConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	imp := res.CMS
	if len(imp.Posts) != 1 || len(imp.Pages) != 1 {
		t.Fatalf("imported = %d posts %d pages", len(imp.Posts), len(imp.Pages))
	}
	post := imp.Posts[0]
	if post.Title != "First article" || post.Link != "/blog/first-article" || post.Slug != "first-article" ||
		post.Status != "publish" || post.Content != "<p>Body one</p>" || post.Excerpt != "Summary one" {
		t.Fatalf("post = %+v", post)
	}
	if post.Date.Year() != 2026 {
		t.Fatalf("unix created = %v", post.Date)
	}
	if terms, ok := post.TaxonomiesFM["topics"].([]interface{}); !ok || terms[0] != "Research" {
		t.Fatalf("vocabulary terms = %#v", post.TaxonomiesFM)
	}
	if post.Extra["field_subtitle"] != "A closer look" {
		t.Fatalf("dynamic fields = %#v", post.Extra)
	}
	if imp.Authors[5].Name != "editor" {
		t.Fatalf("authors = %+v", imp.Authors)
	}
	if _, anon := imp.Authors[0]; anon {
		t.Fatal("anonymous user must be excluded")
	}
	// Unpublished nodes excluded by default; included when published_only=false.
	src.Drupal.PublishedOnly = boolPtr(false)
	res2, err := CMSConnector{}.Load(src)
	if err != nil || len(res2.CMS.Posts) != 2 {
		t.Fatalf("published_only=false: %+v %v", res2.CMS, err)
	}
}

// ─── Movable Type ────────────────────────────────────────────────────────────

func movableTypeFixture(t *testing.T) string {
	t.Helper()
	return buildFixtureDB(t, []string{
		`CREATE TABLE mt_entry (entry_id INTEGER, entry_title TEXT, entry_basename TEXT, entry_text TEXT,
		 entry_text_more TEXT, entry_excerpt TEXT, entry_authored_on TEXT, entry_modified_on TEXT,
		 entry_status INTEGER, entry_class TEXT, entry_author_id INTEGER)`,
		`INSERT INTO mt_entry VALUES (1, 'Old times', 'old-times', 'Part one.', 'Part two.', 'Ex',
		 '2026-02-01 09:00:00', '2026-02-02 09:00:00', 2, 'entry', 3)`,
		`INSERT INTO mt_entry VALUES (2, 'Imprint', 'imprint', 'Legal.', '', '',
		 '2026-02-01 09:00:00', '2026-02-01 09:00:00', 2, 'page', 3)`,
		`INSERT INTO mt_entry VALUES (3, 'Unfinished', 'unfinished', 'Draft.', '', '',
		 '2026-02-01 09:00:00', '2026-02-01 09:00:00', 1, 'entry', 3)`,
		`CREATE TABLE mt_author (author_id INTEGER, author_name TEXT, author_nickname TEXT)`,
		`INSERT INTO mt_author VALUES (3, 'melody', 'Melody Nelson')`,
		`CREATE TABLE mt_category (category_id INTEGER, category_label TEXT, category_basename TEXT)`,
		`INSERT INTO mt_category VALUES (20, 'Stories', 'stories')`,
		`CREATE TABLE mt_placement (placement_entry_id INTEGER, placement_category_id INTEGER)`,
		`INSERT INTO mt_placement VALUES (1, 20)`,
		`CREATE TABLE mt_tag (tag_id INTEGER, tag_name TEXT)`,
		`INSERT INTO mt_tag VALUES (30, 'retro')`,
		`CREATE TABLE mt_objecttag (objecttag_object_id INTEGER, objecttag_tag_id INTEGER, objecttag_object_datasource TEXT)`,
		`INSERT INTO mt_objecttag VALUES (1, 30, 'entry')`,
		`CREATE TABLE mt_asset (asset_id INTEGER, asset_label TEXT, asset_url TEXT, asset_file_path TEXT)`,
		`INSERT INTO mt_asset VALUES (40, 'Cover', '%r/cover.jpg', '/site/cover.jpg')`,
		`CREATE TABLE mt_comment (comment_id INTEGER, comment_entry_id INTEGER, comment_author TEXT,
		 comment_email TEXT, comment_url TEXT, comment_created_on TEXT, comment_text TEXT, comment_visible INTEGER)`,
		`INSERT INTO mt_comment VALUES (50, 1, 'Anna', 'anna@example.com', 'https://anna.example.com',
		 '2026-02-03 10:00:00', 'Lovely read.', 1)`,
		`INSERT INTO mt_comment VALUES (51, 1, 'Bert', '', '', '2026-02-02 08:00:00', 'First!', 1)`,
		`INSERT INTO mt_comment VALUES (52, 1, 'Spam Bot', 'spam@spam.example', '', '2026-02-04 10:00:00', 'Buy pills', 0)`,
		`INSERT INTO mt_comment VALUES (53, 2, 'Carol', '', '', '2026-02-05 09:00:00', 'Page comment.', 1)`,
	})
}

func TestMovableTypeAdapter(t *testing.T) {
	src := cmsSource("movable_type", movableTypeFixture(t), "content")
	res, err := CMSConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	imp := res.CMS
	if len(imp.Posts) != 1 || len(imp.Pages) != 1 {
		t.Fatalf("imported = %d posts %d pages", len(imp.Posts), len(imp.Pages))
	}
	post := imp.Posts[0]
	if post.Title != "Old times" || post.Slug != "old-times" ||
		!strings.Contains(post.Content, "Part one.") || !strings.Contains(post.Content, "Part two.") ||
		post.Category != "Stories" || post.Tags[0] != "retro" || post.Author != 3 {
		t.Fatalf("post = %+v", post)
	}
	if imp.Pages[0].Type != "page" || imp.Pages[0].Slug != "imprint" {
		t.Fatalf("page = %+v", imp.Pages[0])
	}
	if imp.Authors[3].Name != "Melody Nelson" {
		t.Fatalf("authors = %+v", imp.Authors)
	}
	if len(imp.Media) != 1 || imp.Media[0]["file_path"] != "/site/cover.jpg" {
		t.Fatalf("assets = %+v", imp.Media)
	}
	// include toggles: entries off → only the page remains.
	src.MovableType.IncludeEntries = boolPtr(false)
	src.MovableType.IncludeAssets = boolPtr(false)
	res2, err := CMSConnector{}.Load(src)
	if err != nil || len(res2.CMS.Posts) != 0 || len(res2.CMS.Pages) != 1 || len(res2.CMS.Media) != 0 {
		t.Fatalf("toggles: %+v %v", res2.CMS, err)
	}
}

// TestMovableTypeCommentsSkippedByDefault pins the include_comments=false
// default (GO-058): no comment query, no .Extra, no data-view key.
func TestMovableTypeCommentsSkippedByDefault(t *testing.T) {
	src := cmsSource("movable_type", movableTypeFixture(t), "content")
	res, err := CMSConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if res.CMS.Posts[0].Extra != nil {
		t.Fatalf("comments must be skipped by default: %#v", res.CMS.Posts[0].Extra)
	}
	view := res.Data.(map[string]interface{})["posts"].([]interface{})[0].(map[string]interface{})
	if _, ok := view["comments"]; ok {
		t.Fatal("data view must not expose comments by default")
	}
}

// TestMovableTypeComments covers include_comments=true (GO-058): visible
// comments attach to their entry as .Extra["comments"] ordered by creation
// time, hidden comments are excluded, and the data view exposes the list.
func TestMovableTypeComments(t *testing.T) {
	src := cmsSource("movable_type", movableTypeFixture(t), "content")
	src.MovableType.IncludeComments = true
	res, err := CMSConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	post := res.CMS.Posts[0]
	comments, ok := post.Extra["comments"].([]map[string]interface{})
	if !ok || len(comments) != 2 {
		t.Fatalf("comments = %#v", post.Extra)
	}
	// Ordered by creation time; the hidden comment (comment_visible=0) is excluded.
	if comments[0]["author"] != "Bert" || comments[0]["body"] != "First!" {
		t.Fatalf("first comment = %#v", comments[0])
	}
	if comments[1]["author"] != "Anna" || comments[1]["email"] != "anna@example.com" ||
		comments[1]["url"] != "https://anna.example.com" || comments[1]["body"] != "Lovely read." {
		t.Fatalf("second comment = %#v", comments[1])
	}
	if d, ok := comments[1]["date"].(time.Time); !ok || d.Year() != 2026 || d.Month() != 2 {
		t.Fatalf("comment date = %#v", comments[1]["date"])
	}
	// Comments on MT pages attach the same way.
	pageComments, ok := res.CMS.Pages[0].Extra["comments"].([]map[string]interface{})
	if !ok || len(pageComments) != 1 || pageComments[0]["author"] != "Carol" {
		t.Fatalf("page comments = %#v", res.CMS.Pages[0].Extra)
	}
	// The data view exposes the same list for mode: data templates.
	view := res.Data.(map[string]interface{})["posts"].([]interface{})[0].(map[string]interface{})
	if got, ok := view["comments"].([]map[string]interface{}); !ok || len(got) != 2 {
		t.Fatalf("data view comments = %#v", view["comments"])
	}
	// include_comments now resolves instead of being rejected as deferred.
	if _, err := Resolve(Config{Sources: map[string]SourceConfig{"mt": {Type: "cms", Adapter: "movable_type",
		Driver: "sqlite", Database: "x.db", MovableType: MovableTypeOptions{IncludeComments: true}}}}); err != nil {
		t.Fatalf("include_comments must resolve: %v", err)
	}
}

// ─── shared CMS plumbing ─────────────────────────────────────────────────────

func TestCMSResolveMatrix(t *testing.T) {
	cases := map[string]SourceConfig{
		"bad adapter": {Type: "cms", Adapter: "ghost", Driver: "sqlite", Database: "x.db"},
		"bad mode":    {Type: "cms", Adapter: "wordpress", Mode: "hybrid", Driver: "sqlite", Database: "x.db"},
		"no db":       {Type: "cms", Adapter: "wordpress", Driver: "sqlite"},
		"bad driver":  {Type: "cms", Adapter: "wordpress", Driver: "oracle", Database: "x.db"},
	}
	for label, sc := range cases {
		if _, err := Resolve(Config{Sources: map[string]SourceConfig{"cms": sc}}); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
	sources, err := Resolve(Config{Sources: map[string]SourceConfig{"wp": {
		Type: "cms", Adapter: "wordpress", Driver: "sqlite", Database: "x.db"}}})
	if err != nil || sources[0].Mode != "content" {
		t.Fatalf("default mode = %+v, %v", sources, err)
	}
}

func TestCMSHelpers(t *testing.T) {
	if _, err := adapterFor("ghost"); err == nil {
		t.Fatal("unknown adapter must error")
	}
	if inPlaceholders("pgx", 2, 3) != "$2,$3,$4" || inPlaceholders("sqlite", 1, 2) != "?,?" {
		t.Fatal("inPlaceholders")
	}
	if asString(nil) != "" || asString([]byte("x")) != "x" || asString(7) != "7" {
		t.Fatal("asString")
	}
	if asInt(int64(5)) != 5 || asInt("8") != 8 || asInt([]byte("9")) != 9 || asInt(3.7) != 3 ||
		asInt(nil) != 0 || asInt("abc") != 0 || asInt(4) != 4 {
		t.Fatal("asInt")
	}
	if cmsTime(int64(1767225600)).Year() != 2026 || cmsTime("2026-01-02").Day() != 2 ||
		!cmsTime(nil).IsZero() || !cmsTime("gibberish").IsZero() ||
		cmsTime([]byte("2026-01-02 10:00:00")).Hour() != 10 ||
		cmsTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Year() != 2026 ||
		cmsTime(1767225600).Year() != 2026 {
		t.Fatal("cmsTime")
	}
	if fallbackSlug("keep", "T") != "keep" || fallbackSlug("", "My Great Post") != "my-great-post" {
		t.Fatal("fallbackSlug")
	}
	if firstWords("SELECT a FROM b WHERE c", 2) != "SELECT a…" {
		t.Fatal("firstWords")
	}
	if slugFromAlias("/blog/deep/post", "t") != "post" || slugFromAlias("", "Fallback Title") != "fallback-title" {
		t.Fatal("slugFromAlias")
	}
	if drupalStatus(1) != "publish" || drupalStatus(0) != "draft" {
		t.Fatal("drupalStatus")
	}
}
