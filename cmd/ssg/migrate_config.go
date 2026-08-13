package main

// After a migration, the export knows things the scaffolded config does not:
// what the site calls itself, what it says it is about, which timezone its
// dates were written in, and what colours its theme used. Those landed in
// metadata.json and stopped there, so every migrated project started as an
// untitled site in the default palette (#119).
//
// This fills in ONLY the keys the config has not got. A value the author (or an
// earlier migration) already wrote is never touched, so re-running a migration
// cannot undo a decision — and the file is edited as a YAML document, so
// comments and key order survive.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/models"
)

// migratedConfigKey is one key filled in from the export.
type migratedConfigKey struct {
	name  string
	value interface{}
}

// applyMigratedIdentity fills the config's empty identity keys from the
// export's metadata.json and reports what it set. A missing or unreadable
// metadata file is not an error: the migration itself succeeded, and the config
// simply stays as scaffolded.
func applyMigratedIdentity(configPath, dest string) []string {
	if configPath == "" {
		return nil
	}
	meta, err := readExportMetadata(filepath.Join(dest, "metadata.json"))
	if err != nil {
		return nil
	}
	src, err := os.ReadFile(configPath) // #nosec G304 -- the project's own config file
	if err != nil {
		return nil
	}

	var applied []string
	for _, k := range migratedIdentityKeys(meta) {
		if config.HasYAMLKey(src, k.name) {
			continue
		}
		updated, setErr := config.SetYAMLKey(src, k.name, k.value)
		if setErr != nil {
			continue
		}
		src = updated
		applied = append(applied, fmt.Sprintf("%s: %s", k.name, describeConfigValue(k.value)))
	}
	if len(applied) == 0 {
		return nil
	}
	if err := os.WriteFile(configPath, src, 0o644); err != nil { // #nosec G306 -- project config file
		return nil
	}
	return applied
}

// migratedIdentityKeys turns the export's site metadata into config keys,
// skipping anything the export did not report.
func migratedIdentityKeys(meta models.Metadata) []migratedConfigKey {
	var keys []migratedConfigKey
	add := func(name string, value interface{}, keep bool) {
		if keep {
			keys = append(keys, migratedConfigKey{name, value})
		}
	}
	// The site's own name and tagline, preferring what its <head> actually
	// rendered (og:site_name) over the CMS setting when both exist.
	title := firstNonEmptyString(meta.Marketing.OGSiteName, meta.Site.Name)
	add("title", title, title != "")
	add("description", meta.Site.Description, meta.Site.Description != "")

	if tz := ianaTimezone(meta.Site.Timezone); tz != "" {
		add("timezone", tz, true)
	}
	add("colors", meta.Marketing.Colors, len(meta.Marketing.Colors) > 0)
	return keys
}

// readExportMetadata decodes an export's metadata.json.
func readExportMetadata(path string) (models.Metadata, error) {
	var meta models.Metadata
	f, err := os.Open(path) // #nosec G304 -- the export this run just wrote
	if err != nil {
		return meta, err
	}
	defer func() { _ = f.Close() }()
	err = json.NewDecoder(f).Decode(&meta)
	return meta, err
}

// ianaTimezone keeps only a zone ssg can actually load. WordPress reports
// either an IANA name ("Europe/Warsaw") or a bare UTC offset ("UTC+0", "UTC-5");
// the offset form is not a zone — it carries no DST rules — so only plain UTC
// survives it. Writing an unloadable zone would be worse than writing none.
func ianaTimezone(tz string) string {
	tz = strings.TrimSpace(tz)
	switch {
	case tz == "":
		return ""
	case strings.Contains(tz, "/"):
		return tz
	case strings.EqualFold(tz, "UTC"), strings.EqualFold(tz, "UTC+0"), strings.EqualFold(tz, "UTC-0"):
		return "UTC"
	}
	return ""
}

// firstNonEmptyString returns the first value that is not empty.
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// describeConfigValue renders a set value for the report: scalars as-is, a
// palette as its role count, so the summary stays one line per key.
func describeConfigValue(v interface{}) string {
	if m, ok := v.(map[string]string); ok {
		return fmt.Sprintf("%d colour(s) from the source theme", len(m))
	}
	return fmt.Sprintf("%v", v)
}
