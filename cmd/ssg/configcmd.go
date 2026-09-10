package main

// `ssg config view|set|unset` (GO-101).
//
// The MCP designer tools could already change a handful of presentation
// settings from an assistant; a person at a terminal had no way to change
// anything at all without opening an editor. Both now go through one engine,
// so an agent and a human edit a config identically — and both leave the
// comments alone, which matters more here than anywhere else: the configs in
// this project carry their documentation in comments, and an editor that
// re-serialised the parsed struct would delete all of it on the first write.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spagu/ssg/internal/config"
)

// isConfigSubcommand keeps the verb+noun dispatch rule: `ssg config view` is a
// subcommand, while a source directory literally named "config" still builds.
func isConfigSubcommand(noun string) bool {
	switch noun {
	case "view", "set", "unset":
		return true
	}
	return false
}

// runConfig implements `ssg config <verb> [args]`.
func runConfig(args []string) int {
	verb, rest := args[0], args[1:]
	path, rest := configPathFrom(rest)
	if path == "" {
		errf("❌ no config file found — pass --config=FILE or run this in a project with .ssg.yaml.\n")
		return 1
	}
	switch verb {
	case "view":
		return runConfigView(path, rest)
	case "set":
		return runConfigSet(path, rest)
	default:
		return runConfigUnset(path, rest)
	}
}

// configPathFrom pulls --config=FILE (or --config FILE) out of the arguments
// and falls back to the same discovery a build uses, so `ssg config view` in a
// project needs no arguments at all.
func configPathFrom(args []string) (string, []string) {
	var rest []string
	path := ""
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], configFlag+"="):
			path = strings.TrimPrefix(args[i], configFlag+"=")
		case args[i] == configFlag && i+1 < len(args):
			path = args[i+1]
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	if path == "" {
		path = config.FindConfigFile()
	}
	return path, rest
}

// yamlOnly refuses a TOML or JSON config. Version one edits YAML: keeping a
// TOML file's comments through an edit is its own piece of work, and quietly
// rewriting one without them would be the kind of loss this feature exists to
// prevent.
func yamlOnly(path string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return nil
	default:
		return fmt.Errorf("%s: `ssg config set` edits YAML configs only — %s keeps its comments only if you edit it yourself",
			path, filepath.Ext(path))
	}
}

// runConfigView prints the whole file, one path out of it, or the effective
// value the generator will actually see.
func runConfigView(path string, args []string) int {
	effective := false
	var target string
	for _, a := range args {
		switch {
		case a == "--effective":
			effective = true
		case strings.HasPrefix(a, "-"):
			errf("❌ unknown option %s (try --effective)\n", a)
			return 2
		default:
			target = a
		}
	}
	if effective {
		return printEffective(path, target)
	}
	src, err := os.ReadFile(path) // #nosec G304,G703 -- the project's own config, named by the operator on the command line
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	if target == "" {
		fmt.Print(string(src))
		return 0
	}
	out, err := config.GetYAMLPath(src, target)
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	fmt.Print(string(out))
	return 0
}

// printEffective shows the value after defaults and normalisation — what the
// generator reads, which is not always what the file says. A key left out of
// the file has a default; `language_configs` are expanded; `minify_all` sets
// three other flags. That difference is worth being able to see.
//
// Secrets are not. This view is the resolved config, which means an API key
// the file only referenced as $VAR is a literal value by the time it gets
// here, and printing the whole thing would put it on a terminal, in a scroll
// buffer and in whatever log the session is being written to. Every field that
// can hold a credential is redacted before anything is printed.
func printEffective(path, target string) int {
	cfg, err := config.Load(path)
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	// #nosec G117 -- redactSecrets on the next line blanks every credential
	// before a byte of this is printed; the marshal itself never leaves here.
	data, err := yaml.Marshal(cfg)
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	data = redactSecrets(data)
	if target == "" {
		fmt.Print(string(data))
		return 0
	}
	out, err := config.GetYAMLPath(data, target)
	if err != nil {
		errf("❌ %v (the effective config has every key, so this path does not exist at all)\n", err)
		return 1
	}
	fmt.Print(string(out))
	return 0
}

// redactedValue is what a credential reads as in the effective view.
const redactedValue = "«redacted — read it from the file or the environment»"

// secretPaths are the config keys that can carry a credential. The list is
// deliberately over-broad: a key wrongly redacted costs one glance at the file,
// while a key wrongly printed cannot be taken back.
var secretPaths = []string{
	"jwt_secret", "server_users", "api_key", "tls_key",
	"mcp.git.token", "mcp.search.mddb_api_key",
	"mddb.api_key", "mddb.password", "mddb.dsn",
	"external_sources.sources", "ai", "worker", "deploy", "notify",
}

// redactSecrets blanks every credential-bearing key that is actually present.
// It works on the marshalled YAML rather than the struct so one list covers
// nested sections whose shapes differ.
func redactSecrets(data []byte) []byte {
	for _, path := range secretPaths {
		if _, err := config.GetYAMLPath(data, path); err != nil {
			continue // not set in this config
		}
		out, err := config.SetYAMLPath(data, path, redactedValue)
		if err != nil {
			// Better to drop the key entirely than to print it.
			if out, err = config.UnsetYAMLPath(data, path); err != nil {
				continue
			}
		}
		data = out
	}
	return data
}

// runConfigSet writes one value.
func runConfigSet(path string, args []string) int {
	forceString, asJSON := false, false
	var rest []string
	for _, a := range args {
		switch a {
		case "--string":
			forceString = true
		case "--json":
			asJSON = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) != 2 {
		errf("❌ usage: ssg config set <path> <value> [--string|--json]\n")
		return 2
	}
	if err := yamlOnly(path); err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	target, raw := rest[0], rest[1]
	var value interface{}
	if asJSON {
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			errf("❌ --json: %v\n", err)
			return 1
		}
	} else {
		value = config.ParseYAMLValue(raw, forceString)
	}
	return applyConfigEdit(path, target, func(src []byte) ([]byte, error) {
		return config.SetYAMLPath(src, target, value)
	}, fmt.Sprintf("%s = %s", target, raw))
}

// runConfigUnset removes one value.
func runConfigUnset(path string, args []string) int {
	if len(args) != 1 {
		errf("❌ usage: ssg config unset <path>\n")
		return 2
	}
	if err := yamlOnly(path); err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	target := args[0]
	return applyConfigEdit(path, target, func(src []byte) ([]byte, error) {
		return config.UnsetYAMLPath(src, target)
	}, fmt.Sprintf("%s unset", target))
}

// applyConfigEdit runs one edit and keeps the file valid.
//
// The edited bytes are validated in a sibling file BEFORE the original is
// touched, so an edit that would leave the project unbuildable never reaches
// the config at all — the file stays byte-for-byte as it was, rather than
// being written and rolled back.
func applyConfigEdit(path, target string, edit func([]byte) ([]byte, error), what string) int {
	src, err := os.ReadFile(path) // #nosec G304,G703 -- the project's own config, named by the operator on the command line
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	updated, err := edit(src)
	if err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	check := siblingCheckPath(path)
	// #nosec G306,G703 -- a scratch copy beside the config the operator named
	if err := os.WriteFile(check, updated, 0o644); err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	defer func() { _ = os.Remove(check) }() // #nosec G703 -- the scratch file this function just made
	if _, err := config.Load(check); err != nil {
		errf("❌ %s would make the configuration invalid: %v\n", target, err)
		errf("   %s is unchanged.\n", path)
		return 1
	}
	// #nosec G306,G703 -- the config file the operator named, written back in place
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		errf("❌ %v\n", err)
		return 1
	}
	fmt.Printf("✅ %s: %s\n", path, what)
	return 0
}

// siblingCheckPath names the scratch file used to validate an edit. It keeps
// the original's extension, because that is how the loader picks a parser, and
// sits beside it, because a config resolves `include:` relative to itself.
func siblingCheckPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".ssgcheck" + ext
}
