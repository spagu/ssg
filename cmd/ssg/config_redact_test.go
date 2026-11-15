package main

// SR-15 (GO-183 security review): `ssg config view --effective` masks what the
// configuration schema classes "secret", not a hand-written list.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/spagu/ssg/internal/config"
)

const sentinel = "SENTINEL-credential-value"

// legacySecretPaths is the hand-written list `ssg config view --effective`
// masked before SR-15, verbatim. Everything it masked must stay masked.
var legacySecretPaths = []string{
	"jwt_secret", "server_users", "api_key", "tls_key",
	"mcp.git.token", "mcp.search.mddb_api_key",
	"mddb.api_key", "mddb.password", "mddb.dsn",
	"external_sources.sources", "ai", "worker", "deploy", "notify",
}

// docAt builds a document holding leaf at a schema path: "[]" becomes a
// one-element list, "*" a map key.
func docAt(path string, leaf any) any {
	if path == "" {
		return leaf
	}
	seg, rest, _ := strings.Cut(path, ".")
	if seg == "*" {
		return map[string]any{"k": docAt(rest, leaf)}
	}
	if name, ok := strings.CutSuffix(seg, "[]"); ok {
		return map[string]any{name: []any{docAt(rest, leaf)}}
	}
	return map[string]any{seg: docAt(rest, leaf)}
}

// redactedAt marshals a document with the sentinel at path and redacts it.
func redactedAt(t *testing.T, path string) string {
	t.Helper()
	data, err := yaml.Marshal(docAt(path, sentinel))
	if err != nil {
		t.Fatal(err)
	}
	return string(redactSecrets(data))
}

// secretPathsOf lists every named schema node of class "secret".
func secretPathsOf(nodes []*config.SchemaNode, out []string) []string {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.Key != "" && n.Class == config.ClassSecret {
			out = append(out, n.Path)
		}
		out = secretPathsOf(n.Fields, out)
		out = secretPathsOf([]*config.SchemaNode{n.Items, n.Values}, out)
	}
	return out
}

// TestRedactSecretsCoversEverySchemaSecret: every key of class "secret", list
// elements and map values included, is masked.
func TestRedactSecretsCoversEverySchemaSecret(t *testing.T) {
	paths := secretPathsOf(config.BuildSchema().Keys, nil)
	for _, want := range []string{"tts.api_key", "workers[].auth.token", "endpoints[].password", "notifications[].headers", "ai.models.*.key", "external_sources.sources.*.auth.token"} {
		found := false
		for _, p := range paths {
			found = found || p == want
		}
		if !found {
			t.Errorf("%s is not a secret key of the schema any more: %v", want, paths)
		}
	}
	for _, p := range paths {
		if got := redactedAt(t, p); strings.Contains(got, sentinel) || !strings.Contains(got, "redacted") {
			t.Errorf("%s was not masked:\n%s", p, got)
		}
	}
}

// TestRedactSecretsKeepsTheLegacyList: a value masked before SR-15 stays
// masked. A path of the old list that is not a configuration key never
// reaches the effective view at all.
func TestRedactSecretsKeepsTheLegacyList(t *testing.T) {
	s := config.BuildSchema()
	effective, err := yaml.Marshal(config.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range legacySecretPaths {
		if s.Node(p) == nil {
			if _, err := config.GetYAMLPath(effective, p); err == nil {
				t.Errorf("%s is in the effective view but not in the schema", p)
			}
			continue
		}
		if got := redactedAt(t, p); strings.Contains(got, sentinel) {
			t.Errorf("%s was masked before SR-15 and is printed now:\n%s", p, got)
		}
	}
}

// TestRedactSecretsWalksShapes: both forms of a key with an alternate, keys
// the schema does not name, and ordinary values beside secrets.
func TestRedactSecretsWalksShapes(t *testing.T) {
	in := "title: Example\nstory: true\nlanguages: [en, pl]\nunknown_key: 1\n" +
		"web_push: {enabled: true}\nworkers:\n  - name: a\n    auth: {type: bearer, token: " + sentinel + "}\n"
	got := string(redactSecrets([]byte(in)))
	if strings.Contains(got, sentinel) {
		t.Errorf("a worker's token survived:\n%s", got)
	}
	for _, kept := range []string{"title: Example", "story: true", "unknown_key: 1", "name: a", "type: bearer"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q was taken too:\n%s", kept, got)
		}
	}
}

// TestRedactSecretsFailsClosed: what cannot be read or written back is one
// redacted line, never the input.
func TestRedactSecretsFailsClosed(t *testing.T) {
	for _, in := range []string{"jwt_secret: [" + sentinel, ""} {
		if got := string(redactSecrets([]byte(in))); got != redactedValue+"\n" {
			t.Errorf("%q → %q", in, got)
		}
	}
	saved := marshalYAML
	t.Cleanup(func() { marshalYAML = saved })
	marshalYAML = func(any) ([]byte, error) { return nil, errors.New("no") }
	if got := string(redactSecrets([]byte("jwt_secret: " + sentinel + "\n"))); got != redactedValue+"\n" {
		t.Errorf("a failed write-back printed %q", got)
	}
	inProject(t, ".ssg.yaml", sampleConfig)
	if code := runConfig([]string{"view", "--effective"}); code != 1 {
		t.Errorf("a config that cannot be marshalled: exit %d, want 1", code)
	}
}

// TestConfigViewEffectiveMasksNewSecrets: keys the old list missed.
func TestConfigViewEffectiveMasksNewSecrets(t *testing.T) {
	t.Setenv("SSG_TEST_TTS_KEY", sentinel)
	inProject(t, ".ssg.yaml", sampleConfig+"tts:\n  api_key: $SSG_TEST_TTS_KEY\n")
	out, err := captureStdout(func() error {
		if runConfig([]string{"view", "--effective", "tts.api_key"}) != 0 {
			t.Error("the effective view should print")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, sentinel) || !strings.Contains(out, "redacted") {
		t.Errorf("tts.api_key reached the terminal: %q", out)
	}
}

// TestConfigSetReportsAReadOnlyConfig: the check copy is written beside it,
// but the config itself cannot be rewritten — an error, file unchanged.
func TestConfigSetReportsAReadOnlyConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".ssg.yaml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o400); err != nil {
		t.Fatal(err)
	}
	if code := runConfig([]string{"set", "--config=" + path, "template", "ssgtheme"}); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if readFile(t, path) != sampleConfig {
		t.Error("the config was modified despite the failure")
	}
}
