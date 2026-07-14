package externalsource

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// CMSImportResult is the unified output of every CMS adapter (plan §CMS
// Unified Output): SSG-native pages/posts plus site-level collections.
type CMSImportResult struct {
	Pages      []models.Page
	Posts      []models.Page
	Authors    map[int]models.Author
	Taxonomies map[string][]string
	Media      []map[string]interface{}
	Metadata   map[string]interface{}
}

// cmsAdapter imports one CMS database into the unified result.
type cmsAdapter interface {
	Import(ctx context.Context, db *sql.DB, src Source) (*CMSImportResult, error)
}

// CMSConnector opens the database with the shared SQL drivers and delegates
// to the configured adapter. In `mode: content` the import is additionally
// exposed to the generator for merging into the site; in `mode: data` it only
// feeds .ExternalData.
type CMSConnector struct{}

// Load runs the adapter and packages both the data view and the import.
func (CMSConnector) Load(src Source) (*Result, error) {
	adapter, err := adapterFor(src.Adapter)
	if err != nil {
		return nil, fail(src, "config", err)
	}
	driver, dsn := driverAndDSN(src)
	db, err := sqlOpen(driver, dsn)
	if err != nil {
		return nil, fail(src, "connect", redactSecret(err, dsn))
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(2)

	ctx, cancel := context.WithTimeout(context.Background(), src.Timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fail(src, "connect", redactSecret(err, dsn))
	}
	imported, err := adapter.Import(ctx, db, src)
	if err != nil {
		return nil, fail(src, "import", redactSecret(err, dsn))
	}
	res := &Result{Name: src.Name, Type: src.Type, Data: cmsDataView(imported),
		Metadata: Metadata{SourceType: src.Type, Identifier: src.Adapter + " via " + sqlIdentifier(src),
			FetchedAt: time.Now(), RecordCount: len(imported.Pages) + len(imported.Posts),
			ContentType: src.Adapter}}
	if src.Mode == "content" {
		res.CMS = imported // the generator merges this into the site
	}
	return res, nil
}

// fallbackSlug derives a slug from the title when the CMS basename is empty.
func fallbackSlug(slug, title string) string {
	if slug != "" {
		return slug
	}
	return strings.ToLower(strings.Join(strings.Fields(title), "-"))
}

// adapterFor maps the configured adapter name to its implementation.
func adapterFor(name string) (cmsAdapter, error) {
	switch name {
	case "wordpress":
		return wordpressAdapter{}, nil
	case "drupal":
		return drupalAdapter{}, nil
	case "movable_type":
		return movableTypeAdapter{}, nil
	}
	return nil, fmt.Errorf("unsupported adapter %q", name)
}

// cmsDataView renders the import as template-friendly maps so `mode: data`
// (and templates in content mode) can iterate it under .ExternalData.
func cmsDataView(r *CMSImportResult) map[string]interface{} {
	pages := make([]interface{}, 0, len(r.Pages))
	for _, p := range r.Pages {
		pages = append(pages, cmsPageView(p))
	}
	posts := make([]interface{}, 0, len(r.Posts))
	for _, p := range r.Posts {
		posts = append(posts, cmsPageView(p))
	}
	authors := make([]interface{}, 0, len(r.Authors))
	for _, a := range r.Authors {
		authors = append(authors, map[string]interface{}{"id": a.ID, "name": a.Name, "slug": a.Slug})
	}
	return map[string]interface{}{
		"pages": pages, "posts": posts, "authors": authors,
		"taxonomies": r.Taxonomies, "media": r.Media, "metadata": r.Metadata,
	}
}

// cmsPageView flattens one imported page for the data namespace.
func cmsPageView(p models.Page) map[string]interface{} {
	return map[string]interface{}{
		"id": p.ID, "title": p.Title, "slug": p.Slug, "type": p.Type, "status": p.Status,
		"date": p.Date, "modified": p.Modified, "excerpt": p.Excerpt, "content": p.Content,
		"author": p.Author, "category": p.Category, "tags": p.Tags, "link": p.Link,
	}
}

// ─── shared adapter helpers ──────────────────────────────────────────────────

// identRe guards identifiers interpolated into CMS SQL (table prefixes).
var identRe = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// queryMaps runs a query and scans every row into a column→value map.
func queryMaps(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", firstWords(query, 4), err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		record := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			record[col] = normalizeSQLValue(values[i])
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// firstWords truncates a query for error messages.
func firstWords(s string, n int) string {
	fields := strings.Fields(s)
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, " ") + "…"
}

// inPlaceholders builds an IN(...) placeholder list for the driver ("?" for
// mysql/sqlite, "$N" for postgres) starting at position start (1-based).
func inPlaceholders(driver string, start, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		if driver == "pgx" {
			parts[i] = fmt.Sprintf("$%d", start+i)
		} else {
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ",")
}

// asString renders any scanned value as a string.
func asString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	}
	return fmt.Sprintf("%v", v)
}

// asInt renders any scanned numeric value as an int.
func asInt(v interface{}) int {
	switch t := v.(type) {
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	case []byte:
		return atoiSafe(string(t))
	case string:
		return atoiSafe(t)
	}
	return 0
}

// atoiSafe parses an int, returning 0 on failure.
func atoiSafe(s string) int {
	n := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil {
		return 0
	}
	return n
}

// cmsTime parses CMS datetime values: native time.Time, unix seconds, or the
// common "2006-01-02 15:04:05" / RFC3339 string layouts.
func cmsTime(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case int64:
		return time.Unix(t, 0).UTC()
	case int:
		return time.Unix(int64(t), 0).UTC()
	case []byte:
		return parseTimeString(string(t))
	case string:
		return parseTimeString(t)
	}
	return time.Time{}
}

// parseTimeString tries the datetime layouts CMS databases actually use.
func parseTimeString(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts
		}
	}
	return time.Time{}
}
