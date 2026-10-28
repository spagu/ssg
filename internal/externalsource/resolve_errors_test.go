package externalsource

// Config that cannot mean anything fails at resolve time, before a single
// request or query runs, and names the field that is wrong.

import (
	"strings"
	"testing"
)

func TestResolveSourceRefusals(t *testing.T) {
	t.Setenv("SSG_TEST_UNSET_VAR", "")
	for name, c := range map[string]struct {
		sc   SourceConfig
		want string
	}{
		"sql with an unknown mode":  {SourceConfig{Type: "sql", Mode: "bogus"}, `unsupported mode "bogus"`},
		"http with an unknown mode": {SourceConfig{Type: "http", URL: "https://api.example.com/x.json", Mode: "bogus"}, `unsupported mode "bogus"`},
		"cms data with a bad timeout": {SourceConfig{Type: "cms", Adapter: "wordpress", Mode: "data", Timeout: "soon"},
			"timeout"},
		"an unset variable in the url": {SourceConfig{Type: "http", URL: "$SSG_TEST_UNSET_VAR/x.json"}, "SSG_TEST_UNSET_VAR"},
		"an unset variable in a header": {SourceConfig{Type: "http", URL: "https://api.example.com/x.json",
			Headers: map[string]string{"X-Key": "$SSG_TEST_UNSET_VAR"}}, "SSG_TEST_UNSET_VAR"},
		"an unset variable in the query": {SourceConfig{Type: "http", URL: "https://api.example.com/x.json",
			Query: map[string]string{"key": "$SSG_TEST_UNSET_VAR"}}, "SSG_TEST_UNSET_VAR"},
		"basic auth with a literal password": {SourceConfig{Type: "http", URL: "https://api.example.com/x.json",
			Auth: AuthConfig{Type: "basic", Username: "u", Password: "hunter2"}}, "must reference an environment variable"},
	} {
		if _, err := resolveSource("src", c.sc, Defaults{}, defaultMaxSize); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}
}

// TestResolveSourceExplicitContentSettings keeps what the config states.
func TestResolveSourceExplicitContentSettings(t *testing.T) {
	src, err := resolveSource("docs", SourceConfig{
		Type: "file", Path: "docs.json", Mode: "content",
		ContentMap:    ContentMap{Title: "name", Content: "body"},
		ContentFormat: FormatHTML, ContentErrors: "strict",
	}, Defaults{}, defaultMaxSize)
	if err != nil {
		t.Fatal(err)
	}
	if src.ContentFormat != FormatHTML || src.ContentErrors != "strict" {
		t.Errorf("got %q / %q", src.ContentFormat, src.ContentErrors)
	}
}
