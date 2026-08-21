package main

// `ssg mddb push-theme` — keep an MDDB collection in step with the theme (#190).
//
// The find tools scan the project locally, which answers most questions and
// needs nothing installed. What a local scan cannot do is answer a question
// phrased as a sentence — "where is the page background set?" — because that is
// a search problem, not a text-matching one. Putting the theme in an MDDB
// collection is what makes that answerable, and this is what puts it there.
//
// The sync is a reconciliation, not an append: every template and asset is
// upserted under its project-relative path, and any document in the collection
// whose path no longer exists on disk is deleted. Running it twice changes
// nothing the second time.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mddb"
)

// pushThemeFlags are the options `ssg mddb push-theme` accepts.
type pushThemeFlags struct {
	dry  bool
	lang string
}

// runMddbPushTheme uploads the theme to the configured MDDB collection.
func runMddbPushTheme(args []string) int {
	flags, err := parsePushThemeFlags(args)
	if err != nil {
		errf("❌ %v\n\nusage: ssg mddb push-theme [--dry] [--lang=en]\n", err)
		return 1
	}
	cfg := loadConfig(nil)
	sc := cfg.MCP.Search
	if !sc.Enabled() {
		errf("❌ no MDDB search target configured — set mcp.search.mddb_url and mcp.search.mddb_collection\n" +
			"   (see docs/MCP.md; without it the find tools scan the project locally, which needs no setup)\n")
		return 1
	}
	client := mddb.NewClient(mddb.Config{BaseURL: sc.MddbURL, APIKey: expandEnvValue(sc.MddbAPIKey)})
	lang := firstNonEmpty(flags.lang, sc.MddbLang, "en")

	files := themeFiles(cfg)
	if len(files) == 0 {
		errf("❌ no template or asset files found under %s or %s\n", cfg.TemplatesDir, cfg.StaticDir)
		return 1
	}
	return syncTheme(client, sc.MddbCollection, lang, files, flags, cfg.Quiet)
}

// syncTheme upserts every file and removes documents whose file is gone.
func syncTheme(client *mddb.Client, collection, lang string, files []string,
	flags pushThemeFlags, quiet bool) int {
	onDisk := make(map[string]bool, len(files))
	for _, rel := range files {
		onDisk[rel] = true
	}

	pushed, failed := 0, 0
	for _, rel := range files {
		body, readErr := os.ReadFile(rel) // #nosec G304 -- enumerated from the configured theme dirs
		if readErr != nil {
			errf("⚠️  %s: %v\n", rel, readErr)
			failed++
			continue
		}
		if flags.dry {
			pushed++
			continue
		}
		if addErr := client.Add(themeDocument(collection, lang, rel, body)); addErr != nil {
			errf("⚠️  %s: %v\n", rel, addErr)
			failed++
			continue
		}
		pushed++
	}

	removed := pruneTheme(client, collection, lang, onDisk, flags.dry)
	if !quiet {
		verb := "pushed"
		if flags.dry {
			verb = "would push"
		}
		fmt.Printf("✅ %s %d theme file(s) to %q, removed %d stale document(s)\n",
			verb, pushed, collection, removed)
	}
	if failed > 0 {
		errf("⚠️  %d file(s) failed\n", failed)
		return 1
	}
	return 0
}

// pruneTheme deletes documents whose source file no longer exists. A collection
// that cannot be listed is not fatal: the upserts above already landed, and
// refusing to report them because the cleanup half failed would be worse.
func pruneTheme(client *mddb.Client, collection, lang string, onDisk map[string]bool, dry bool) int {
	docs, err := client.GetAll(collection, lang, 0)
	if err != nil {
		errf("⚠️  could not list %q to remove stale documents: %v\n", collection, err)
		return 0
	}
	removed := 0
	for _, d := range docs {
		if onDisk[d.Key] {
			continue
		}
		if dry {
			removed++
			continue
		}
		if delErr := client.Delete(mddb.DeleteRequest{Collection: collection, Key: d.Key, Lang: lang}); delErr != nil {
			errf("⚠️  removing %s: %v\n", d.Key, delErr)
			continue
		}
		removed++
	}
	return removed
}

// themeDocument describes one file as an MDDB document. The key is the
// project-relative path, so a search hit names the file to open — and the
// checksum makes an unchanged file recognisable without comparing bodies.
func themeDocument(collection, lang, rel string, body []byte) mddb.AddRequest {
	sum := sha256.Sum256(body)
	return mddb.AddRequest{
		Collection: collection,
		Key:        rel,
		Lang:       lang,
		ContentMD:  string(body),
		Meta: map[string][]string{
			"path":     {rel},
			"kind":     {themeKind(rel)},
			"size":     {strconv.Itoa(len(body))},
			"checksum": {hex.EncodeToString(sum[:])},
		},
	}
}

// themeKind labels a file by what it does, so a query can be narrowed to
// stylesheets or to markup.
func themeKind(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".css", ".scss", ".sass":
		return "style"
	case ".js", ".mjs", ".ts":
		return "script"
	case ".html", ".htm", ".tmpl":
		return "template"
	case ".json", ".yaml", ".yml", ".toml":
		return "data"
	}
	return "asset"
}

// themeFiles lists every template and static asset, project-relative and sorted.
//
// It cannot fail: the walk callback swallows per-entry errors — a theme
// directory that is not there is an ordinary project, not a problem — and
// WalkDir only returns what its callback returns. An error return here would be
// a branch no caller could ever take.
func themeFiles(cfg *config.Config) []string {
	var out []string
	for _, base := range []string{cfg.TemplatesDir, cfg.StaticDir} {
		if base == "" {
			continue
		}
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a missing theme directory is simply empty
			}
			out = append(out, filepath.ToSlash(p))
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// parsePushThemeFlags reads the subcommand's own flags.
func parsePushThemeFlags(args []string) (pushThemeFlags, error) {
	var f pushThemeFlags
	for _, a := range args {
		switch {
		case a == "--dry" || a == "--dry-run":
			f.dry = true
		case strings.HasPrefix(a, "--lang="):
			f.lang = strings.TrimPrefix(a, "--lang=")
		default:
			return f, fmt.Errorf("unknown flag %q", a)
		}
	}
	return f, nil
}

// firstNonEmpty returns the first value that is set.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
