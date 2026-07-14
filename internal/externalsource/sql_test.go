package externalsource

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newSQLiteFixture creates a products database on disk and returns its path.
func newSQLiteFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	stmts := []string{
		`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, slug TEXT, price REAL, published INTEGER)`,
		`INSERT INTO products VALUES (1, 'Widget', 'widget', 9.99, 1)`,
		`INSERT INTO products VALUES (2, 'Gadget', 'gadget', 19.99, 1)`,
		`INSERT INTO products VALUES (3, 'Hidden', 'hidden', 0, 0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// sqliteSource builds a resolved sqlite source over the fixture.
func sqliteSource(t *testing.T, queries map[string]Query) Source {
	t.Helper()
	return Source{Name: "catalog", Type: "sql", Driver: "sqlite", Database: newSQLiteFixture(t),
		Required: true, Timeout: 5 * time.Second, Queries: queries}
}

func TestSQLConnectorLoadsQueries(t *testing.T) {
	src := sqliteSource(t, map[string]Query{
		"products": {SQL: "SELECT id, name, slug, price FROM products WHERE published = 1 ORDER BY id", MaxRows: 100},
		"count":    {SQL: "SELECT COUNT(*) AS n FROM products", MaxRows: 10},
	})
	res, err := SQLConnector{}.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	data := res.Data.(map[string]interface{})
	products := data["products"].([]interface{})
	first := products[0].(map[string]interface{})
	if len(products) != 2 || first["name"] != "Widget" || first["slug"] != "widget" {
		t.Fatalf("products = %#v", products)
	}
	if res.Metadata.RecordCount != 3 || res.Metadata.SourceType != "sql" ||
		!strings.Contains(res.Metadata.Identifier, "sqlite:") {
		t.Fatalf("meta = %+v", res.Metadata)
	}
}

func TestSQLConnectorMaxRowsExceeded(t *testing.T) {
	src := sqliteSource(t, map[string]Query{
		"all": {SQL: "SELECT id FROM products", MaxRows: 1},
	})
	_, err := SQLConnector{}.Load(src)
	if err == nil || !strings.Contains(err.Error(), "max_rows") {
		t.Fatalf("err = %v", err)
	}
}

func TestSQLConnectorQueryAndConnectErrors(t *testing.T) {
	src := sqliteSource(t, map[string]Query{
		"broken": {SQL: "SELECT missing_column FROM nope", MaxRows: 10},
	})
	_, err := SQLConnector{}.Load(src)
	if err == nil || !strings.Contains(err.Error(), "failed at query broken") {
		t.Fatalf("query err = %v", err)
	}
	// Unreachable database file directory.
	bad := Source{Name: "x", Type: "sql", Driver: "sqlite", Database: "/nonexistent-dir/db.sqlite",
		Timeout: 2 * time.Second, Queries: map[string]Query{"q": {SQL: "SELECT 1", MaxRows: 1}}}
	if _, err := (SQLConnector{}).Load(bad); err == nil {
		t.Fatal("connect to unreachable path must error")
	}
}

func TestSQLDSNRedaction(t *testing.T) {
	err := redactSecret(fmt.Errorf("dial failed for user:pass@tcp(db:3306)/wp"), "user:pass@tcp(db:3306)/wp")
	if strings.Contains(err.Error(), "pass") {
		t.Fatalf("dsn leaked: %v", err)
	}
	if redactSecret(nil, "x") != nil || redactSecret(fmt.Errorf("e"), "").Error() != "e" {
		t.Fatal("redact edge cases")
	}
}

func TestValidateReadOnlySQL(t *testing.T) {
	valid := []string{
		"SELECT * FROM t",
		"  select id from t;  ",
		"WITH recent AS (SELECT * FROM t) SELECT * FROM recent",
		"-- comment\nSELECT 1",
		"/* block */ SELECT 1",
	}
	for _, q := range valid {
		if err := validateReadOnlySQL(q); err != nil {
			t.Errorf("%q rejected: %v", q, err)
		}
	}
	invalid := []string{
		"",
		"DELETE FROM t",
		"DROP TABLE t",
		"UPDATE t SET a=1",
		"INSERT INTO t VALUES (1)",
		"SELECT 1; DELETE FROM t",
		"-- only a comment",
	}
	for _, q := range invalid {
		if err := validateReadOnlySQL(q); err == nil {
			t.Errorf("%q accepted", q)
		}
	}
	if stripSQLComments("a /* unterminated") != "a " || stripSQLComments("b -- eol") != "b " {
		t.Fatal("comment stripping edges")
	}
	if firstWord("  ") != "" || firstWord("DROP TABLE") != "DROP" {
		t.Fatal("firstWord")
	}
}

func TestSQLResolveMatrix(t *testing.T) {
	t.Setenv("ES_TEST_DSN", "user:pass@tcp(db:3306)/shop")
	query := map[string]QueryConfig{"products": {SQL: "SELECT 1"}}
	cases := map[string]SourceConfig{
		"bad driver":   {Type: "sql", Driver: "oracle", Queries: query},
		"sqlite no db": {Type: "sql", Driver: "sqlite", Queries: query},
		"missing dsn":  {Type: "sql", Driver: "mysql", Queries: query},
		"literal dsn":  {Type: "sql", Driver: "mysql", DSN: "user:pass@db/x", Queries: query},
		"unset env":    {Type: "sql", Driver: "mysql", DSN: "$ES_UNSET_DSN", Queries: query},
		"no queries":   {Type: "sql", Driver: "sqlite", Database: "x.db"},
		"bad q name": {Type: "sql", Driver: "sqlite", Database: "x.db",
			Queries: map[string]QueryConfig{"Bad Name": {SQL: "SELECT 1"}}},
		"write query": {Type: "sql", Driver: "sqlite", Database: "x.db",
			Queries: map[string]QueryConfig{"q": {SQL: "DELETE FROM t"}}},
		"bad timeout": {Type: "sql", Driver: "sqlite", Database: "x.db", Timeout: "never", Queries: query},
	}
	for label, sc := range cases {
		if _, err := Resolve(Config{Sources: map[string]SourceConfig{"db": sc}}); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
	// Valid mysql/mariadb/postgres configs resolve with env DSN + row default.
	for _, driver := range []string{"mysql", "mariadb", "postgres"} {
		cfg := Config{Sources: map[string]SourceConfig{"db": {
			Type: "sql", Driver: driver, DSN: "$ES_TEST_DSN", Queries: query}}}
		sources, err := Resolve(cfg)
		if err != nil {
			t.Fatalf("%s: %v", driver, err)
		}
		s := sources[0]
		if s.DSN != "user:pass@tcp(db:3306)/shop" || s.Queries["products"].MaxRows != defaultMaxRows {
			t.Fatalf("%s resolved = %+v", driver, s)
		}
		wantDriver := map[string]string{"mysql": "mysql", "mariadb": "mysql", "postgres": "pgx"}[driver]
		if got, _ := driverAndDSN(s); got != wantDriver {
			t.Fatalf("driverAndDSN(%s) = %s", driver, got)
		}
		if strings.Contains(sqlIdentifier(s), "pass") {
			t.Fatalf("identifier leaks dsn: %s", sqlIdentifier(s))
		}
	}
}

func TestSQLRegistryIntegration(t *testing.T) {
	path := newSQLiteFixture(t)
	cfg := Config{Enabled: true, CacheDir: t.TempDir(), Sources: map[string]SourceConfig{
		"catalog": {Type: "sql", Driver: "sqlite", Database: path,
			Queries: map[string]QueryConfig{"products": {SQL: "SELECT name FROM products WHERE published = 1"}}},
	}}
	reg, warns, err := Load(cfg)
	if err != nil || len(warns) != 0 {
		t.Fatalf("load: %v %v", err, warns)
	}
	rows := reg.Data()["catalog"].(map[string]interface{})["products"].([]interface{})
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
}
