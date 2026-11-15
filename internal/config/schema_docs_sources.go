package config

// Schema entries: MDDB and external data sources. Everything that names where
// the build reads content or data from is `executes`; credentials are secrets.

import "github.com/spagu/ssg/internal/externalsource"

var _ = describe(Config{}, typeDocs{
	"mddb":             x("MDDB server the content is read from").in(gSources),
	"external_sources": x("files, HTTP APIs, SQL databases and CMS databases read at build time").in(gSources),
})

var _ = describe(MddbConfig{}, typeDocs{
	"enabled":        x("read content from MDDB"),
	"url":            x("MDDB base URL"),
	"protocol":       x("connection protocol").oneOf("", "http", "grpc"),
	"api_key":        s("MDDB API key"),
	"collection":     x("collection the content is read from"),
	"lang":           p("language filter, e.g. en_US"),
	"timeout":        p("request timeout in seconds"),
	"batch_size":     p("documents per request (default 1000)"),
	"watch":          p("rebuild when the collection changes"),
	"allow_http":     x("send the API key over plain http to a non-loopback host"),
	"trusted":        x("skip the HTML sanitizer for MDDB content"),
	"watch_interval": p("seconds between change checks (default 30)"),
	"sources":        x("several collections, each with a filter, type and section"),
})

var _ = describe(MddbSource{}, typeDocs{
	"collection": x("collection read"),
	"filter":     p("metadata values a document may have, per key"),
	"type":       p("file every document as a page or a post").oneOf("", "page", "post"),
	"section":    p("content section the documents belong to"),
})

var _ = describe(externalsource.Config{}, typeDocs{
	"enabled":                x("read external sources"),
	"cache_dir":              x("cache directory (default .ssg-cache/external-sources)"),
	"offline":                p("build from the cache only"),
	"refresh":                p("fetch again, ignoring freshness"),
	"stale_if_error":         p("use a stale cache entry when a fetch fails (default on)").defaults(true),
	"fail_on_cache_miss":     p("fail an offline build on a missing entry (default on)").defaults(true),
	"max_concurrent_sources": p("sources fetched at once"),
	"allowed_hosts":          x("hosts HTTP sources may reach (empty = any public host)"),
	"defaults":               x("settings every source inherits"),
	"sources":                x("the sources, by name"),
})

var _ = describe(externalsource.Defaults{}, typeDocs{
	"required":          p("a failing source fails the build"),
	"allow_http":        x("permit plain http://"),
	"allow_private":     x("permit loopback and private addresses"),
	"max_response_size": p("largest response accepted"),
	"timeout":           p("request timeout (default 10s)"),
	"cache_ttl":         p("freshness of a cache entry (default 1h)"),
	"stale_ttl":         p("how long a stale entry may serve (default 24h)"),
	"retries":           p("extra attempts (default 2)"),
	"retry_backoff":     p("wait between attempts (default 500ms)"),
})

var _ = describe(externalsource.SourceConfig{}, typeDocs{
	"type":           x("source type: file, http, sql"),
	"format":         p("data format, e.g. json, yaml, csv"),
	"path":           x("file read by a file source"),
	"required":       p("a failing source fails the build"),
	"transform":      p("post-parse transformation"),
	"csv":            p("CSV parser options"),
	"url":            x("URL fetched by an HTTP source"),
	"headers":        s("request headers; values may hold $NAME references").secretAs(SecretInline),
	"query":          x("query parameters"),
	"auth":           x("authentication of the request"),
	"pagination":     x("how a multi-page HTTP source is fetched"),
	"allow_http":     x("permit plain http://"),
	"allow_private":  x("permit loopback and private addresses"),
	"timeout":        p("request timeout"),
	"cache_ttl":      p("freshness of a cache entry"),
	"stale_ttl":      p("how long a stale entry may serve"),
	"retries":        p("extra attempts"),
	"retry_backoff":  p("wait between attempts"),
	"driver":         x("SQL driver").oneOf("", "mysql", "mariadb", "postgres", "sqlite"),
	"dsn":            s("SQL connection string"),
	"database":       x("SQLite database file"),
	"queries":        x("named read-only SQL queries"),
	"content_map":    p("record field behind each page field (records as pages)"),
	"content_format": p("format of the mapped content").oneOf("", "markdown", "html", "text"),
	"content_errors": p("what a bad record does").oneOf("", "warn", "strict"),
	"max_rows":       p("record cap (default 10000)"),
	"adapter":        x("CMS adapter over the SQL driver").oneOf("", "wordpress", "drupal", "movable_type"),
	"mode":           p("records as content (default) or as data").oneOf("", "content", "data"),
	"wordpress":      x("WordPress adapter options"),
	"drupal":         x("Drupal adapter options"),
	"movable_type":   x("Movable Type adapter options"),
})

var _ = describe(externalsource.TransformConfig{}, typeDocs{
	"select": p("dot path into the parsed data"),
})

var _ = describe(externalsource.CSVOptions{}, typeDocs{
	"header":    p("the first row is a header (default on)").defaults(true),
	"delimiter": p("field delimiter (default ,)"),
})

var _ = describe(externalsource.AuthConfig{}, typeDocs{
	"type":     x("how the request authenticates").oneOf("", "bearer", "basic", "header"),
	"token":    s("bearer token"),
	"username": x("basic-auth user"),
	"password": s("basic-auth password"),
	"header":   x("header name of header auth"),
	"value":    s("header value of header auth"),
})

var _ = describe(externalsource.PaginationConfig{}, typeDocs{
	"mode":           x("page numbers or Link rel=next").oneOf("", "page", "link"),
	"param":          x("page query parameter (default page)"),
	"start_page":     x("first page number (default 1)"),
	"per_page":       x("page size sent (0 = not sent)"),
	"per_page_param": x("page-size parameter name (default per_page)"),
	"max_pages":      x("hard page limit (default 10)"),
})

var _ = describe(externalsource.QueryConfig{}, typeDocs{
	"sql":      x("read-only SQL query"),
	"max_rows": p("row cap (default 10000)"),
})

var _ = describe(externalsource.ContentMap{}, typeDocs{
	"title":              p("record field of the title"),
	"slug":               p("record field or template of the slug"),
	"content":            p("record field of the body"),
	"excerpt":            p("record field of the excerpt"),
	"date":               p("record field of the date"),
	"modified":           p("record field of the modification date"),
	"type":               p("page type; \"=product\" sets a constant"),
	"status":             p("record field of the status"),
	"link":               p("record field of the link"),
	"description":        p("record field of the meta description"),
	"image":              p("record field of the image"),
	"lang":               p("record field of the language"),
	"sitemap_changefreq": p("sitemap changefreq per record"),
	"sitemap_priority":   p("sitemap priority per record"),
	"taxonomies":         p("taxonomy → record field holding its terms"),
	"schema":             p("structured data every record's page carries"),
})

var _ = describe(externalsource.WordPressOptions{}, typeDocs{
	"table_prefix":          x("table prefix (default wp_)"),
	"post_types":            p("post types read (default post, page)"),
	"statuses":              p("statuses read (default publish)"),
	"include_media":         p("read media"),
	"include_custom_fields": p("read custom fields"),
	"include_taxonomies":    p("read taxonomies"),
})

var _ = describe(externalsource.DrupalOptions{}, typeDocs{
	"version":        p("Drupal version (informational)"),
	"bundles":        p("bundles read (default article, page)"),
	"published_only": p("read published nodes only"),
	"include_fields": p("read node fields into .Extra"),
})

var _ = describe(externalsource.MovableTypeOptions{}, typeDocs{
	"include_entries":  p("read entries"),
	"include_pages":    p("read pages"),
	"include_assets":   p("read assets"),
	"include_comments": p("read visible comments into .Extra"),
})
