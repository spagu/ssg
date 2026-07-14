// Package externalsource implements the unified external data system
// (audit/ssg-external-sources-implementation-plan.md), phase 1: local file
// sources (YAML/JSON/TOML/CSV/XML) behind one registry, one result/metadata
// model and one error model, exposed to templates as .ExternalData without
// touching the existing .Data namespace. HTTP, SQL and CMS connectors are
// later phases and are rejected with a descriptive error for now.
package externalsource

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Config is the `external_sources:` block of the site configuration.
type Config struct {
	Enabled  bool                    `yaml:"enabled" toml:"enabled" json:"enabled"`
	Defaults Defaults                `yaml:"defaults" toml:"defaults" json:"defaults"`
	Sources  map[string]SourceConfig `yaml:"sources" toml:"sources" json:"sources"`
}

// Defaults apply to every source that does not override them.
type Defaults struct {
	Required *bool  `yaml:"required" toml:"required" json:"required"`
	MaxSize  string `yaml:"max_response_size" toml:"max_response_size" json:"max_response_size"`
}

// SourceConfig is one declared source (YAML/TOML/JSON shape).
type SourceConfig struct {
	Type      string          `yaml:"type" toml:"type" json:"type"`
	Format    string          `yaml:"format" toml:"format" json:"format"`
	Path      string          `yaml:"path" toml:"path" json:"path"`
	Required  *bool           `yaml:"required" toml:"required" json:"required"`
	Transform TransformConfig `yaml:"transform" toml:"transform" json:"transform"`
	CSV       CSVOptions      `yaml:"csv" toml:"csv" json:"csv"`
}

// TransformConfig is the shared post-parse transformation layer. Phase 1
// implements `select` (a dot path into the parsed structure).
type TransformConfig struct {
	Select string `yaml:"select" toml:"select" json:"select"`
}

// CSVOptions tune the CSV parser.
type CSVOptions struct {
	Header    *bool  `yaml:"header" toml:"header" json:"header"`          // default true
	Delimiter string `yaml:"delimiter" toml:"delimiter" json:"delimiter"` // default ","
}

// Source is one fully-resolved source definition.
type Source struct {
	Name      string
	Type      string
	Format    string
	Path      string
	Required  bool
	MaxSize   int64
	Transform TransformConfig
	CSV       CSVOptions
}

// nameRe matches the same identifier space as taxonomy names.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// supportedFormats for the file connector.
var supportedFormats = map[string]bool{"yaml": true, "json": true, "toml": true, "csv": true, "xml": true}

// laterPhaseTypes are planned connector types not yet implemented.
var laterPhaseTypes = map[string]string{"http": "phase 2", "sql": "phase 3", "cms": "phases 4-6"}

// defaultMaxSize caps file sources at 5MB unless configured otherwise.
const defaultMaxSize = 5 << 20

// Resolve validates the configuration and returns the sources in
// deterministic (name-sorted) order.
func Resolve(cfg Config) ([]Source, error) {
	maxSize, err := parseSize(cfg.Defaults.MaxSize, defaultMaxSize)
	if err != nil {
		return nil, fmt.Errorf("external_sources.defaults.max_response_size: %w", err)
	}
	names := make([]string, 0, len(cfg.Sources))
	for name := range cfg.Sources {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Source, 0, len(names))
	for _, name := range names {
		src, err := resolveSource(name, cfg.Sources[name], cfg.Defaults, maxSize)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, nil
}

// resolveSource validates and normalizes one source definition.
func resolveSource(name string, sc SourceConfig, defaults Defaults, maxSize int64) (Source, error) {
	if !nameRe.MatchString(name) {
		return Source{}, fmt.Errorf("invalid external source name %q (want lowercase letters, digits, _ or -)", name)
	}
	if phase, planned := laterPhaseTypes[sc.Type]; planned {
		return Source{}, fmt.Errorf("external source %q: type %q is planned for %s and not available yet — only \"file\" is supported", name, sc.Type, phase)
	}
	if sc.Type != "file" {
		return Source{}, fmt.Errorf("external source %q: unsupported type %q (supported: file)", name, sc.Type)
	}
	if sc.Path == "" {
		return Source{}, fmt.Errorf("external source %q: path is required", name)
	}
	format := strings.ToLower(sc.Format)
	if format == "" {
		format = formatFromExtension(sc.Path)
	}
	if format == "yml" {
		format = "yaml"
	}
	if !supportedFormats[format] {
		return Source{}, fmt.Errorf("external source %q: unsupported format %q (supported: yaml, json, toml, csv, xml)", name, sc.Format)
	}
	required := true
	if defaults.Required != nil {
		required = *defaults.Required
	}
	if sc.Required != nil {
		required = *sc.Required
	}
	return Source{Name: name, Type: sc.Type, Format: format, Path: sc.Path,
		Required: required, MaxSize: maxSize, Transform: sc.Transform, CSV: sc.CSV}, nil
}

// formatFromExtension infers a parser format from the file extension.
func formatFromExtension(path string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
}

// parseSize parses "5MB"/"512KB"/"1GB" or a plain byte count.
func parseSize(s string, def int64) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return def, nil
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "GB"):
		mult, s = 1<<30, strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		mult, s = 1<<20, strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		mult, s = 1<<10, strings.TrimSuffix(s, "KB")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q (want e.g. 5MB, 512KB or a byte count)", s)
	}
	return n * mult, nil
}
