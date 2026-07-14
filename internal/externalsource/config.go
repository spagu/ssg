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
	"time"
)

// Config is the `external_sources:` block of the site configuration.
type Config struct {
	Enabled bool `yaml:"enabled" toml:"enabled" json:"enabled"`

	// Shared disk cache (phase 2): populated by HTTP sources, consulted for
	// freshness, stale-if-error and offline builds.
	CacheDir        string `yaml:"cache_dir" toml:"cache_dir" json:"cache_dir"` // default .ssg-cache/external-sources
	Offline         bool   `yaml:"offline" toml:"offline" json:"offline"`
	Refresh         bool   `yaml:"refresh" toml:"refresh" json:"refresh"`
	StaleIfError    *bool  `yaml:"stale_if_error" toml:"stale_if_error" json:"stale_if_error"`             // default true
	FailOnCacheMiss *bool  `yaml:"fail_on_cache_miss" toml:"fail_on_cache_miss" json:"fail_on_cache_miss"` // default true (offline mode)
	MaxConcurrent   int    `yaml:"max_concurrent_sources" toml:"max_concurrent_sources" json:"max_concurrent_sources"`

	// AllowedHosts restricts HTTP sources to these hosts ("api.example.com" or
	// "*.example.com"). Empty = any public host.
	AllowedHosts []string `yaml:"allowed_hosts" toml:"allowed_hosts" json:"allowed_hosts"`

	Defaults Defaults                `yaml:"defaults" toml:"defaults" json:"defaults"`
	Sources  map[string]SourceConfig `yaml:"sources" toml:"sources" json:"sources"`

	// CLI-only knobs (not read from the config file).
	ClearCache bool   `yaml:"-" toml:"-" json:"-"` // --clear-external-cache
	Only       string `yaml:"-" toml:"-" json:"-"` // --external-source=<name>: narrow --refresh to one source
}

// Defaults apply to every source that does not override them.
type Defaults struct {
	Required     *bool  `yaml:"required" toml:"required" json:"required"`
	MaxSize      string `yaml:"max_response_size" toml:"max_response_size" json:"max_response_size"`
	Timeout      string `yaml:"timeout" toml:"timeout" json:"timeout"`                   // default 10s
	CacheTTL     string `yaml:"cache_ttl" toml:"cache_ttl" json:"cache_ttl"`             // default 1h
	StaleTTL     string `yaml:"stale_ttl" toml:"stale_ttl" json:"stale_ttl"`             // default 24h
	Retries      *int   `yaml:"retries" toml:"retries" json:"retries"`                   // default 2
	RetryBackoff string `yaml:"retry_backoff" toml:"retry_backoff" json:"retry_backoff"` // default 500ms
}

// AuthConfig authenticates HTTP sources. Secret values must reference
// environment variables ("$API_TOKEN"); literals are rejected so credentials
// never live in the config file.
type AuthConfig struct {
	Type     string `yaml:"type" toml:"type" json:"type"` // bearer | basic | header
	Token    string `yaml:"token" toml:"token" json:"token"`
	Username string `yaml:"username" toml:"username" json:"username"`
	Password string `yaml:"password" toml:"password" json:"password"`
	Header   string `yaml:"header" toml:"header" json:"header"`
	Value    string `yaml:"value" toml:"value" json:"value"`
}

// SourceConfig is one declared source (YAML/TOML/JSON shape).
type SourceConfig struct {
	Type      string          `yaml:"type" toml:"type" json:"type"`
	Format    string          `yaml:"format" toml:"format" json:"format"`
	Path      string          `yaml:"path" toml:"path" json:"path"`
	Required  *bool           `yaml:"required" toml:"required" json:"required"`
	Transform TransformConfig `yaml:"transform" toml:"transform" json:"transform"`
	CSV       CSVOptions      `yaml:"csv" toml:"csv" json:"csv"`

	// HTTP sources (phase 2).
	URL          string            `yaml:"url" toml:"url" json:"url"`
	Headers      map[string]string `yaml:"headers" toml:"headers" json:"headers"`
	Query        map[string]string `yaml:"query" toml:"query" json:"query"`
	Auth         AuthConfig        `yaml:"auth" toml:"auth" json:"auth"`
	AllowHTTP    bool              `yaml:"allow_http" toml:"allow_http" json:"allow_http"`          // permit plain http:// (default: HTTPS only)
	AllowPrivate bool              `yaml:"allow_private" toml:"allow_private" json:"allow_private"` // permit localhost/private IPs (self-hosted APIs)
	Timeout      string            `yaml:"timeout" toml:"timeout" json:"timeout"`
	CacheTTL     string            `yaml:"cache_ttl" toml:"cache_ttl" json:"cache_ttl"`
	StaleTTL     string            `yaml:"stale_ttl" toml:"stale_ttl" json:"stale_ttl"`
	Retries      *int              `yaml:"retries" toml:"retries" json:"retries"`
	RetryBackoff string            `yaml:"retry_backoff" toml:"retry_backoff" json:"retry_backoff"`

	// SQL sources (phase 3). DSNs must reference environment variables; SQLite
	// takes a local file path via database.
	Driver   string                 `yaml:"driver" toml:"driver" json:"driver"` // mysql | mariadb | postgres | sqlite
	DSN      string                 `yaml:"dsn" toml:"dsn" json:"dsn"`
	Database string                 `yaml:"database" toml:"database" json:"database"`
	Queries  map[string]QueryConfig `yaml:"queries" toml:"queries" json:"queries"`
}

// QueryConfig is one named read-only query of a SQL source.
type QueryConfig struct {
	SQL     string `yaml:"sql" toml:"sql" json:"sql"`
	MaxRows int    `yaml:"max_rows" toml:"max_rows" json:"max_rows"` // default 10000
}

// Query is the resolved form of QueryConfig.
type Query struct {
	SQL     string
	MaxRows int
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

	// HTTP sources (phase 2); secret values already env-expanded.
	URL          string
	Headers      map[string]string
	Query        map[string]string
	Auth         AuthConfig
	AllowHTTP    bool
	AllowPrivate bool
	Timeout      time.Duration
	CacheTTL     time.Duration
	StaleTTL     time.Duration
	Retries      int
	RetryBackoff time.Duration

	// SQL sources (phase 3); DSN already env-expanded.
	Driver   string
	DSN      string
	Database string
	Queries  map[string]Query
}

// nameRe matches the same identifier space as taxonomy names.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// supportedFormats for the file connector.
var supportedFormats = map[string]bool{"yaml": true, "json": true, "toml": true, "csv": true, "xml": true}

// laterPhaseTypes are planned connector types not yet implemented.
var laterPhaseTypes = map[string]string{"cms": "phases 4-6"}

// sqlDrivers are the supported SQL engines ("mariadb" shares the mysql driver).
var sqlDrivers = map[string]bool{"mysql": true, "mariadb": true, "postgres": true, "sqlite": true}

// defaultMaxRows caps SQL query results unless max_rows overrides it.
const defaultMaxRows = 10000

// defaultMaxSize caps source payloads at 5MB unless configured otherwise.
const defaultMaxSize = 5 << 20

// Hard defaults for HTTP sources (plan §Konfiguracja główna).
const (
	defaultTimeout      = 10 * time.Second
	defaultCacheTTL     = time.Hour
	defaultStaleTTL     = 24 * time.Hour
	defaultRetries      = 2
	defaultRetryBackoff = 500 * time.Millisecond
	defaultConcurrency  = 4
)

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
		return Source{}, fmt.Errorf("external source %q: type %q is planned for %s and not available yet — supported: file, http, sql", name, sc.Type, phase)
	}
	if sc.Type != "file" && sc.Type != "http" && sc.Type != "sql" {
		return Source{}, fmt.Errorf("external source %q: unsupported type %q (supported: file, http, sql)", name, sc.Type)
	}
	src := Source{Name: name, Type: sc.Type, Required: boolLayer(true, defaults.Required, sc.Required),
		MaxSize: maxSize, Transform: sc.Transform, CSV: sc.CSV}
	switch sc.Type {
	case "sql":
		return src, resolveSQL(&src, sc, defaults)
	case "http":
		if err := resolveFormat(&src, sc); err != nil {
			return Source{}, err
		}
		return src, resolveHTTP(&src, sc, defaults)
	default:
		return src, resolveFormat(&src, sc)
	}
}

// boolLayer resolves hard default < defaults block < per-source override.
func boolLayer(hard bool, layers ...*bool) bool {
	out := hard
	for _, l := range layers {
		if l != nil {
			out = *l
		}
	}
	return out
}

// resolveSQL validates driver, credentials and the read-only query set. SQL
// sources have no parser format; the shared timeout still applies.
func resolveSQL(src *Source, sc SourceConfig, defaults Defaults) error {
	var err error
	if src.Timeout, err = resolveDuration(sc.Timeout, defaults.Timeout, defaultTimeout); err != nil {
		return fmt.Errorf("external source %q: timeout: %w", src.Name, err)
	}
	if err := resolveSQLConn(src, sc); err != nil {
		return err
	}
	return resolveSQLQueries(src, sc)
}

// resolveSQLConn validates the driver and credentials (env-only DSNs).
func resolveSQLConn(src *Source, sc SourceConfig) error {
	if !sqlDrivers[sc.Driver] {
		return fmt.Errorf("external source %q: unsupported driver %q (supported: mysql, mariadb, postgres, sqlite)", src.Name, sc.Driver)
	}
	src.Driver = sc.Driver
	src.Database = sc.Database
	if sc.Driver == "sqlite" {
		if sc.Database == "" {
			return fmt.Errorf("external source %q: database (file path) is required for sqlite", src.Name)
		}
		return nil
	}
	if sc.DSN == "" {
		return fmt.Errorf("external source %q: dsn is required for driver %q", src.Name, sc.Driver)
	}
	if !strings.HasPrefix(sc.DSN, "$") {
		return fmt.Errorf("external source %q: dsn must reference an environment variable (e.g. \"$PRODUCT_DB_DSN\"), not a literal", src.Name)
	}
	dsn, err := expandEnvRef(src.Name, "dsn", sc.DSN)
	if err != nil {
		return err
	}
	src.DSN = dsn
	return nil
}

// resolveSQLQueries validates names, read-only statements and row limits.
func resolveSQLQueries(src *Source, sc SourceConfig) error {
	if len(sc.Queries) == 0 {
		return fmt.Errorf("external source %q: at least one query is required", src.Name)
	}
	src.Queries = make(map[string]Query, len(sc.Queries))
	for qname, qc := range sc.Queries {
		if !nameRe.MatchString(qname) {
			return fmt.Errorf("external source %q: invalid query name %q (want lowercase letters, digits, _ or -)", src.Name, qname)
		}
		if err := validateReadOnlySQL(qc.SQL); err != nil {
			return fmt.Errorf("external source %q: query %q: %w", src.Name, qname, err)
		}
		maxRows := qc.MaxRows
		if maxRows <= 0 {
			maxRows = defaultMaxRows
		}
		src.Queries[qname] = Query{SQL: qc.SQL, MaxRows: maxRows}
	}
	return nil
}

// resolveFormat fills the parser format from config or the path/URL extension.
func resolveFormat(src *Source, sc SourceConfig) error {
	if sc.Type == "file" && sc.Path == "" {
		return fmt.Errorf("external source %q: path is required", src.Name)
	}
	src.Path = sc.Path
	format := strings.ToLower(sc.Format)
	if format == "" {
		switch sc.Type {
		case "file":
			format = formatFromExtension(sc.Path)
		case "http":
			format = formatFromExtension(sc.URL)
		}
	}
	if format == "yml" {
		format = "yaml"
	}
	if !supportedFormats[format] {
		return fmt.Errorf("external source %q: unsupported format %q (supported: yaml, json, toml, csv, xml)", src.Name, sc.Format)
	}
	src.Format = format
	return nil
}

// resolveHTTP fills the HTTP-specific fields: URL, expanded secrets and
// durations layered source > defaults > hard default.
func resolveHTTP(src *Source, sc SourceConfig, defaults Defaults) error {
	if sc.URL == "" {
		return fmt.Errorf("external source %q: url is required", src.Name)
	}
	src.URL = sc.URL
	src.AllowHTTP = sc.AllowHTTP
	src.AllowPrivate = sc.AllowPrivate

	var err error
	if src.Headers, err = expandValueMap(src.Name, "headers", sc.Headers); err != nil {
		return err
	}
	if src.Query, err = expandValueMap(src.Name, "query", sc.Query); err != nil {
		return err
	}
	if src.Auth, err = expandAuth(src.Name, sc.Auth); err != nil {
		return err
	}

	durations := []struct {
		target      *time.Duration
		source, def string
		hard        time.Duration
		field       string
	}{
		{&src.Timeout, sc.Timeout, defaults.Timeout, defaultTimeout, "timeout"},
		{&src.CacheTTL, sc.CacheTTL, defaults.CacheTTL, defaultCacheTTL, "cache_ttl"},
		{&src.StaleTTL, sc.StaleTTL, defaults.StaleTTL, defaultStaleTTL, "stale_ttl"},
		{&src.RetryBackoff, sc.RetryBackoff, defaults.RetryBackoff, defaultRetryBackoff, "retry_backoff"},
	}
	for _, d := range durations {
		if *d.target, err = resolveDuration(d.source, d.def, d.hard); err != nil {
			return fmt.Errorf("external source %q: %s: %w", src.Name, d.field, err)
		}
	}
	src.Retries = defaultRetries
	if defaults.Retries != nil {
		src.Retries = *defaults.Retries
	}
	if sc.Retries != nil {
		src.Retries = *sc.Retries
	}
	if src.Retries < 0 {
		return fmt.Errorf("external source %q: retries must be >= 0", src.Name)
	}
	return nil
}

// resolveDuration layers a duration: source value > defaults value > hard default.
func resolveDuration(sourceVal, defaultVal string, hard time.Duration) (time.Duration, error) {
	for _, v := range []string{sourceVal, defaultVal} {
		if v == "" {
			continue
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("invalid duration %q", v)
		}
		return d, nil
	}
	return hard, nil
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
