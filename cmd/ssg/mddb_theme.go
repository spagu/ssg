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
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/mddb"
)

// pushThemeFlags are the options `ssg mddb push-theme` accepts.
type pushThemeFlags struct {
	dry        bool
	lang       string
	validate   bool
	noValidate bool
}

// runMddbPushTheme uploads the theme to the configured MDDB collection.
func runMddbPushTheme(args []string) int {
	flags, err := parsePushThemeFlags(args)
	if err != nil {
		errf("❌ %v\n\nusage: ssg mddb push-theme [--dry] [--lang=en] [--no-validate]\n", err)
		return 1
	}
	cfg := loadConfig(nil)
	sc := cfg.MCP.Search
	if !sc.Enabled() {
		errf("❌ no MDDB search target configured — set mcp.search.mddb_url and mcp.search.mddb_collection\n" +
			"   (see docs/MCP.md; without it the find tools scan the project locally, which needs no setup)\n")
		return 1
	}
	client := mddb.NewClient(mddb.Config{
		BaseURL:   sc.MddbURL,
		APIKey:    expandEnvValue(sc.MddbAPIKey),
		AllowHTTP: sc.MddbAllowHTTP,
	})
	lang := firstNonEmpty(flags.lang, sc.MddbLang, "en")
	flags.validate = sc.ValidateEnabled() && !flags.noValidate

	files := themeFiles(cfg)
	if len(files) == 0 {
		errf("❌ no template or asset files found under %s or %s\n", cfg.TemplatesDir, cfg.StaticDir)
		return 1
	}
	return syncTheme(client, sc.MddbCollection, lang, files, flags, cfg.Quiet)
}

// parsePushThemeFlags reads the subcommand's own flags.
func parsePushThemeFlags(args []string) (pushThemeFlags, error) {
	var f pushThemeFlags
	for _, a := range args {
		switch {
		case a == "--dry" || a == "--dry-run":
			f.dry = true
		case a == "--no-validate":
			f.noValidate = true
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
