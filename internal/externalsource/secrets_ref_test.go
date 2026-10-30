package externalsource

import (
	"errors"
	"strings"
	"testing"
)

// TestExpandEnvRefSpellings covers issue #376: the whole-value form used by
// dsn and the auth secret fields accepts "${NAME}" exactly like "$NAME", and a
// value that is neither spelling is refused with a message naming both.
func TestExpandEnvRefSpellings(t *testing.T) {
	t.Setenv("ES_REF_DSN", "user:pass@tcp(db:3306)/shop")
	for _, value := range []string{"$ES_REF_DSN", "${ES_REF_DSN}"} {
		got, err := expandEnvRef("db", "dsn", value)
		if err != nil || got != "user:pass@tcp(db:3306)/shop" {
			t.Errorf("expandEnvRef(%q) = %q, %v", value, got, err)
		}
	}
	// A literal (no "$") passes through for the caller to refuse.
	if got, err := expandEnvRef("db", "dsn", "literal"); err != nil || got != "literal" {
		t.Errorf("literal = %q, %v", got, err)
	}
	// Unset braced variable names the variable, not "{NAME}".
	_, err := expandEnvRef("db", "dsn", "${ES_REF_UNSET}")
	var unset *UnsetEnvError
	if !errors.As(err, &unset) || unset.Name != "ES_REF_UNSET" {
		t.Fatalf("unset braced = %v, want UnsetEnvError naming ES_REF_UNSET", err)
	}
	for _, bad := range []string{"${ES_REF_DSN", "$ES_REF_DSN/x", "${}", "$", "$$ES_REF_DSN", "$1abc"} {
		_, err := expandEnvRef("db", "dsn", bad)
		if err == nil || errors.As(err, &unset) ||
			!strings.Contains(err.Error(), `"$NAME"`) || !strings.Contains(err.Error(), `"${NAME}"`) {
			t.Errorf("expandEnvRef(%q) err = %v, want a malformed-reference error naming both spellings", bad, err)
		}
	}
}

// TestBracedRefInDSNAndAuth resolves "${NAME}" end to end through Resolve for a
// SQL dsn and every auth secret field (#376), and keeps refusing literals.
func TestBracedRefInDSNAndAuth(t *testing.T) {
	t.Setenv("ES_REF_DSN", "user:pass@tcp(db:3306)/shop")
	t.Setenv("ES_REF_TOKEN", "s3cret")
	query := map[string]QueryConfig{"products": {SQL: "SELECT 1"}}
	sources, err := Resolve(Config{Sources: map[string]SourceConfig{
		"db": {Type: "sql", Driver: "mysql", DSN: "${ES_REF_DSN}", Queries: query},
		"wp": {Type: "cms", Adapter: "wordpress", Driver: "mysql", DSN: "${ES_REF_DSN}"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sources {
		if s.DSN != "user:pass@tcp(db:3306)/shop" {
			t.Errorf("%s dsn = %q", s.Name, s.DSN)
		}
	}

	auths := map[string]AuthConfig{
		"bearer": {Type: "bearer", Token: "${ES_REF_TOKEN}"},
		"basic":  {Type: "basic", Username: "u", Password: "${ES_REF_TOKEN}"},
		"header": {Type: "header", Header: "X-Key", Value: "${ES_REF_TOKEN}"},
	}
	for label, a := range auths {
		got, err := expandAuth("api", a)
		if err != nil || got.Token+got.Password+got.Value != "s3cret" {
			t.Errorf("%s: %+v, %v", label, got, err)
		}
	}

	if _, err := Resolve(Config{Sources: map[string]SourceConfig{
		"db": {Type: "sql", Driver: "mysql", DSN: "user:pass@db/x", Queries: query},
	}}); err == nil || !strings.Contains(err.Error(), "must reference an environment variable") {
		t.Errorf("literal dsn err = %v", err)
	}
	if _, err := expandAuth("api", AuthConfig{Type: "bearer", Token: "literal"}); err == nil {
		t.Error("literal token accepted")
	}
	if _, err := expandAuth("api", AuthConfig{Type: "bearer", Token: "${ES_REF_TOKEN"}); err == nil {
		t.Error("malformed braced token accepted")
	}
}
