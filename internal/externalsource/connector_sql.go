package externalsource

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	// Database drivers, registered for database/sql (plan phase 3).
	_ "github.com/go-sql-driver/mysql" // mysql / mariadb
	_ "github.com/jackc/pgx/v5/stdlib" // postgres
	_ "modernc.org/sqlite"             // sqlite (pure Go, no cgo)
)

// SQLConnector loads read-only query results from MySQL/MariaDB, PostgreSQL
// and SQLite. Queries live exclusively in configuration (never in templates),
// are statically verified to be single SELECT statements, and run under the
// source timeout with a hard row limit. DSNs come from the environment and
// are scrubbed from every error.
type SQLConnector struct{}

// sqlOpen is swappable in tests.
var sqlOpen = sql.Open

// Load executes every configured query and exposes the rows as
// .ExternalData.<source>.<query> ([]map[column]value).
func (SQLConnector) Load(src Source) (*Result, error) {
	driver, dsn := driverAndDSN(src)
	db, err := sqlOpen(driver, dsn)
	if err != nil {
		return nil, fail(src, "connect", redactSecret(err, dsn))
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), src.Timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fail(src, "connect", redactSecret(err, dsn))
	}

	names := make([]string, 0, len(src.Queries))
	for name := range src.Queries {
		names = append(names, name)
	}
	sort.Strings(names)

	data := make(map[string]interface{}, len(names))
	records := 0
	for _, name := range names {
		rows, err := runQuery(ctx, db, src.Queries[name])
		if err != nil {
			return nil, fail(src, "query "+name, redactSecret(err, dsn))
		}
		data[name] = rows
		records += len(rows)
	}
	return &Result{Name: src.Name, Type: src.Type, Data: data, Metadata: Metadata{
		SourceType: src.Type, Identifier: sqlIdentifier(src), FetchedAt: time.Now(),
		RecordCount: records, ContentType: driver,
	}}, nil
}

// driverAndDSN maps the configured driver to the registered database/sql name.
func driverAndDSN(src Source) (string, string) {
	switch src.Driver {
	case "sqlite":
		return "sqlite", src.Database
	case "postgres":
		return "pgx", src.DSN
	default: // mysql, mariadb
		return "mysql", src.DSN
	}
}

// sqlIdentifier is the loggable identifier of a SQL source: never the DSN.
func sqlIdentifier(src Source) string {
	if src.Driver == "sqlite" {
		return "sqlite:" + src.Database
	}
	return src.Driver + " (dsn from environment)"
}

// redactSecret removes a secret substring from an error's message so driver
// errors can never leak credentials into logs.
func redactSecret(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), secret, "[redacted]")
	return fmt.Errorf("%s", msg)
}

// runQuery executes one read-only query and scans generic rows. Exceeding
// max_rows is an error, not a silent truncation.
func runQuery(ctx context.Context, db *sql.DB, q Query) ([]interface{}, error) {
	rows, err := db.QueryContext(ctx, q.SQL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]interface{}, 0, 64)
	for rows.Next() {
		if len(out) >= q.MaxRows {
			return nil, fmt.Errorf("result exceeds max_rows (%d) — raise the limit or narrow the query", q.MaxRows)
		}
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

// normalizeSQLValue converts driver values into template-friendly types.
func normalizeSQLValue(v interface{}) interface{} {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t
	}
	return v
}

// validateReadOnlySQL enforces the read-only contract statically: one
// statement, starting with SELECT (or a WITH … SELECT CTE), no piggybacked
// statements via semicolons.
func validateReadOnlySQL(query string) error {
	stripped := stripSQLComments(query)
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stripped), ";"))
	if trimmed == "" {
		return fmt.Errorf("query is empty")
	}
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("only a single statement is allowed")
	}
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return fmt.Errorf("only SELECT (or WITH … SELECT) statements are allowed, got %q…", firstWord(upper))
	}
	return nil
}

// stripSQLComments removes -- line and /* block */ comments.
func stripSQLComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "--"):
			if nl := strings.IndexByte(s[i:], '\n'); nl >= 0 {
				i += nl + 1
			} else {
				i = len(s)
			}
		case strings.HasPrefix(s[i:], "/*"):
			if end := strings.Index(s[i:], "*/"); end >= 0 {
				i += end + 2
			} else {
				i = len(s)
			}
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// firstWord returns the leading token of a statement for error messages.
func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
