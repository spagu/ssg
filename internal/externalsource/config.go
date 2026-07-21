// Package externalsource implements the unified external data system
// (audit/ssg-external-sources-implementation-plan.md): local file sources
// (YAML/JSON/TOML/CSV/XML), remote HTTP APIs (hardened client, disk cache,
// retries, optional pagination), read-only SQL queries and CMS imports
// (WordPress, Drupal, Movable Type) behind one registry, one result/metadata
// model and one error model, exposed to templates as .ExternalData without
// touching the existing .Data namespace.
package externalsource

import (
	"errors"
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

// Defaults apply to every source that does not override them. The two allow_*
// switches live here as well, so a whole local-dev config can opt into plain
// HTTP against a loopback API without repeating the keys per source (issue #35).
type Defaults struct {
	Required     *bool  `yaml:"required" toml:"required" json:"required"`
	AllowHTTP    *bool  `yaml:"allow_http" toml:"allow_http" json:"allow_http"`
	AllowPrivate *bool  `yaml:"allow_private" toml:"allow_private" json:"allow_private"`
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

// PaginationConfig fetches multi-page HTTP sources (GO-062). mode "page"
// increments a query parameter from start_page; mode "link" follows the
// Link rel="next" response header. Pages are aggregated as one JSON array,
// so pagination requires format "json". max_pages is a hard guard against
// runaway cursors.
type PaginationConfig struct {
	Mode         string `yaml:"mode" toml:"mode" json:"mode"`                               // page | link
	Param        string `yaml:"param" toml:"param" json:"param"`                            // mode=page query parameter (default "page")
	StartPage    int    `yaml:"start_page" toml:"start_page" json:"start_page"`             // mode=page first page number (default 1)
	PerPage      int    `yaml:"per_page" toml:"per_page" json:"per_page"`                   // page-size parameter value; only sent when > 0
	PerPageParam string `yaml:"per_page_param" toml:"per_page_param" json:"per_page_param"` // page-size parameter name (default "per_page")
	MaxPages     int    `yaml:"max_pages" toml:"max_pages" json:"max_pages"`                // hard page limit (default 10, max 1000)
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
	Pagination   PaginationConfig  `yaml:"pagination" toml:"pagination" json:"pagination"`
	AllowHTTP    *bool             `yaml:"allow_http" toml:"allow_http" json:"allow_http"`          // permit plain http:// (default: HTTPS only)
	AllowPrivate *bool             `yaml:"allow_private" toml:"allow_private" json:"allow_private"` // permit localhost/private IPs (self-hosted APIs)
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

	// CMS sources (phases 4-6) run over the same SQL drivers.
	Adapter     string             `yaml:"adapter" toml:"adapter" json:"adapter"` // wordpress | drupal | movable_type
	Mode        string             `yaml:"mode" toml:"mode" json:"mode"`          // content (default) | data
	WordPress   WordPressOptions   `yaml:"wordpress" toml:"wordpress" json:"wordpress"`
	Drupal      DrupalOptions      `yaml:"drupal" toml:"drupal" json:"drupal"`
	MovableType MovableTypeOptions `yaml:"movable_type" toml:"movable_type" json:"movable_type"`
}

// WordPressOptions tune the WordPress adapter.
type WordPressOptions struct {
	TablePrefix         string   `yaml:"table_prefix" toml:"table_prefix" json:"table_prefix"` // default wp_
	PostTypes           []string `yaml:"post_types" toml:"post_types" json:"post_types"`       // default [post, page]
	Statuses            []string `yaml:"statuses" toml:"statuses" json:"statuses"`             // default [publish]
	IncludeMedia        *bool    `yaml:"include_media" toml:"include_media" json:"include_media"`
	IncludeCustomFields *bool    `yaml:"include_custom_fields" toml:"include_custom_fields" json:"include_custom_fields"`
	IncludeTaxonomies   *bool    `yaml:"include_taxonomies" toml:"include_taxonomies" json:"include_taxonomies"`
}

// DrupalOptions tune the Drupal (8-11) adapter.
type DrupalOptions struct {
	Version       int      `yaml:"version" toml:"version" json:"version"` // informational; 8-11 share the schema
	Bundles       []string `yaml:"bundles" toml:"bundles" json:"bundles"` // default [article, page]
	PublishedOnly *bool    `yaml:"published_only" toml:"published_only" json:"published_only"`
	IncludeFields *bool    `yaml:"include_fields" toml:"include_fields" json:"include_fields"` // node__field_* → .Extra
}

// MovableTypeOptions tune the Movable Type adapter.
type MovableTypeOptions struct {
	IncludeEntries  *bool `yaml:"include_entries" toml:"include_entries" json:"include_entries"`
	IncludePages    *bool `yaml:"include_pages" toml:"include_pages" json:"include_pages"`
	IncludeAssets   *bool `yaml:"include_assets" toml:"include_assets" json:"include_assets"`
	IncludeComments bool  `yaml:"include_comments" toml:"include_comments" json:"include_comments"` // GO-058: visible comments → .Extra["comments"]
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

	// HTTP sources (phase 2); secret values already env-expanded. Pagination
	// carries resolved defaults (param, start_page, per_page_param, max_pages);
	// an empty Mode means single-request fetching.
	URL          string
	Headers      map[string]string
	Query        map[string]string
	Auth         AuthConfig
	Pagination   PaginationConfig
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

	// CMS sources (phases 4-6).
	Adapter     string
	Mode        string // "content" | "data"
	WordPress   WordPressOptions
	Drupal      DrupalOptions
	MovableType MovableTypeOptions
}

// nameRe matches the same identifier space as taxonomy names.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// supportedFormats for the file connector.
var supportedFormats = map[string]bool{"yaml": true, "json": true, "toml": true, "csv": true, "xml": true}

// cmsAdapters are the supported CMS adapters.
var cmsAdapters = map[string]bool{"wordpress": true, "drupal": true, "movable_type": true}

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

// Pagination bounds (GO-062): default page limit and the hard ceiling that
// guards against infinite cursors.
const (
	defaultMaxPages = 10
	hardMaxPages    = 1000
)

// Resolve validates the configuration and returns the sources in
// deterministic (name-sorted) order, dropping any skip warnings.
func Resolve(cfg Config) ([]Source, error) {
	sources, _, err := resolveAll(cfg)
	return sources, err
}

// resolveAll is Resolve plus the warnings for optional sources that were
// skipped. A source with required: false whose config references an unset
// environment variable is skipped instead of failing the build, so one shared
// config can carry env-driven sources nobody else has to set up (issue #35).
// Required sources still fail, naming the variable.
func resolveAll(cfg Config) ([]Source, []string, error) {
	maxSize, err := parseSize(cfg.Defaults.MaxSize, defaultMaxSize)
	if err != nil {
		return nil, nil, fmt.Errorf("external_sources.defaults.max_response_size: %w", err)
	}
	names := make([]string, 0, len(cfg.Sources))
	for name := range cfg.Sources {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Source, 0, len(names))
	var warnings []string
	for _, name := range names {
		sc := cfg.Sources[name]
		src, err := resolveSource(name, sc, cfg.Defaults, maxSize)
		if err != nil {
			var unset *UnsetEnvError
			if errors.As(err, &unset) && !boolLayer(true, cfg.Defaults.Required, sc.Required) {
				warnings = append(warnings, fmt.Sprintf("optional external source %q skipped: $%s is not set in the environment", name, unset.Name))
				continue
			}
			return nil, warnings, err
		}
		out = append(out, src)
	}
	return out, warnings, nil
}

// resolveSource validates and normalizes one source definition.
func resolveSource(name string, sc SourceConfig, defaults Defaults, maxSize int64) (Source, error) {
	if !nameRe.MatchString(name) {
		return Source{}, fmt.Errorf("invalid external source name %q (want lowercase letters, digits, _ or -)", name)
	}
	src := Source{Name: name, Type: sc.Type, Required: boolLayer(true, defaults.Required, sc.Required),
		MaxSize: maxSize, Transform: sc.Transform, CSV: sc.CSV}
	switch sc.Type {
	case "sql":
		return src, resolveSQL(&src, sc, defaults)
	case "cms":
		return src, resolveCMS(&src, sc, defaults)
	case "http":
		if err := resolveFormat(&src, sc); err != nil {
			return Source{}, err
		}
		return src, resolveHTTP(&src, sc, defaults)
	case "file":
		return src, resolveFormat(&src, sc)
	default:
		return Source{}, fmt.Errorf("external source %q: unsupported type %q (supported: file, http, sql, cms)", name, sc.Type)
	}
}

// resolveCMS validates the adapter and shares the SQL connection rules.
func resolveCMS(src *Source, sc SourceConfig, defaults Defaults) error {
	if !cmsAdapters[sc.Adapter] {
		return fmt.Errorf("external source %q: unsupported adapter %q (supported: wordpress, drupal, movable_type)", src.Name, sc.Adapter)
	}
	src.Adapter = sc.Adapter
	switch sc.Mode {
	case "", "content":
		src.Mode = "content"
	case "data":
		src.Mode = "data"
	default:
		return fmt.Errorf("external source %q: unsupported mode %q (supported: content, data)", src.Name, sc.Mode)
	}
	src.WordPress = sc.WordPress
	src.Drupal = sc.Drupal
	src.MovableType = sc.MovableType
	var err error
	if src.Timeout, err = resolveDuration(sc.Timeout, defaults.Timeout, defaultTimeout); err != nil {
		return fmt.Errorf("external source %q: timeout: %w", src.Name, err)
	}
	return resolveSQLConn(src, sc)
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
	// The URL expands env references so one config serves every environment:
	// url: "$API_BASE/api/products" (GO-055, issue #35).
	url, err := expandEnvInline(src.Name, "url", sc.URL)
	if err != nil {
		return err
	}
	src.URL = url
	src.AllowHTTP = boolLayer(false, defaults.AllowHTTP, sc.AllowHTTP)
	src.AllowPrivate = boolLayer(false, defaults.AllowPrivate, sc.AllowPrivate)

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
	return resolvePagination(src, sc)
}

// resolvePagination validates the optional pagination block (GO-062) and
// fills its defaults. Pages are aggregated as one JSON array, so pagination
// is limited to format "json".
func resolvePagination(src *Source, sc SourceConfig) error {
	pc := sc.Pagination
	if pc == (PaginationConfig{}) {
		return nil // pagination not configured
	}
	switch pc.Mode {
	case "page", "link":
	case "":
		return fmt.Errorf("external source %q: pagination.mode is required (supported: page, link)", src.Name)
	default:
		return fmt.Errorf("external source %q: unsupported pagination.mode %q (supported: page, link)", src.Name, pc.Mode)
	}
	if src.Format != "json" {
		return fmt.Errorf("external source %q: pagination requires format \"json\" (pages are aggregated as a JSON array)", src.Name)
	}
	if pc.Param == "" {
		pc.Param = "page"
	}
	if pc.PerPageParam == "" {
		pc.PerPageParam = "per_page"
	}
	if pc.PerPage < 0 {
		return fmt.Errorf("external source %q: pagination.per_page must be >= 0", src.Name)
	}
	if pc.StartPage == 0 {
		pc.StartPage = 1
	}
	if pc.StartPage < 0 {
		return fmt.Errorf("external source %q: pagination.start_page must be >= 1", src.Name)
	}
	if pc.MaxPages == 0 {
		pc.MaxPages = defaultMaxPages
	}
	if pc.MaxPages < 1 || pc.MaxPages > hardMaxPages {
		return fmt.Errorf("external source %q: pagination.max_pages must be between 1 and %d", src.Name, hardMaxPages)
	}
	src.Pagination = pc
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
