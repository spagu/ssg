# External sources

{% raw %}
One unified system feeds templates with data from outside the content tree:
local files, remote HTTP APIs, SQL databases and CMS databases (WordPress,
Drupal, Movable Type) — all behind a single registry, one cache, one secrets
rule and one error model. The legacy `.Data` namespace is untouched. A working
project lives in [`examples/external-sources/`](../examples/external-sources/).

```text
External Source → Connector → Parser/Adapter → Normalizer → Cache
             → Unified Data Registry → Generator → Templates
```

## Configuration

```yaml
external_sources:
  enabled: true

  cache_dir: .ssg-cache/external-sources
  offline: false            # serve HTTP sources from the disk cache only
  refresh: false            # ignore fresh cache entries, re-fetch
  stale_if_error: true      # fall back to an expired copy when a fetch fails
  fail_on_cache_miss: true  # offline + no cached copy = build failure
  max_concurrent_sources: 4
  allowed_hosts: []         # optional HTTP allowlist: "api.example.com", "*.example.com"

  defaults:
    timeout: 10s
    cache_ttl: 1h
    stale_ttl: 24h
    retries: 2
    retry_backoff: 500ms
    max_response_size: 5MB
    required: true

  sources:
    local_catalog:
      type: file
      path: ./data/catalog.csv
    products_api:
      type: http
      url: https://api.example.com/products
      format: json
    products_db:
      type: sql
      driver: postgres
      dsn: "$PRODUCT_DB_DSN"
      queries:
        products:
          sql: SELECT id, name, slug FROM products WHERE published = true
    wordpress:
      type: cms
      adapter: wordpress
      driver: mysql
      dsn: "$WORDPRESS_DSN"
```

Source names must match `[a-z][a-z0-9_-]*`. Sources load in deterministic
(name-sorted) order, up to `max_concurrent_sources` at a time. A `required`
source's failure aborts the build; an optional one (`required: false`) warns
and is skipped. Every failure names the source, its type and the stage
(`config`, `read`, `fetch`, `parse`, `transform`, `connect`, `query`,
`import`) — and never contains credentials.

**Secrets come only from the environment.** Values written as `"$NAME"`
resolve from the environment at build time; literal secrets in `dsn`, `auth`
and `jwt`-style fields are rejected outright, and a referenced-but-unset
variable fails the build naming the variable (never a value).

## Local files (`type: file`)

Formats: YAML, JSON, TOML, CSV, XML — inferred from the extension or forced
with `format:`. Files are size-capped by `max_response_size`.

```yaml
rates:
  type: file
  path: ./data/rates.csv
  csv:
    header: true        # rows become {column: value} maps (default)
    delimiter: ","
```

XML maps to nested template-friendly maps: attributes become plain keys,
repeated elements collect into lists, text-only elements collapse to strings
and mixed content keeps its text under `#text`.

## Remote HTTP (`type: http`)

```yaml
products_api:
  type: http
  url: https://api.example.com/products
  format: json          # json | csv | xml (yaml/toml also work)
  headers:
    Accept: application/json
  query:
    page: "1"
  auth:
    type: bearer        # bearer | basic | header
    token: "$API_TOKEN"
  timeout: 10s
  cache_ttl: 1h
  stale_ttl: 24h
  retries: 2
  retry_backoff: 500ms
```

Security, always on:

- HTTPS required; plain `http://` needs `allow_http: true`.
- localhost and private/link-local IPs are refused **at dial time**, which
  also defeats DNS rebinding — opt out per source with `allow_private: true`
  for self-hosted APIs.
- Optional global `allowed_hosts` allowlist (exact or `*.wildcard`).
- Redirects are capped at 5 and every hop is re-validated.
- Responses are size-capped; clearly conflicting `Content-Type`s are rejected.
- Error messages carry the URL without its query string.

Retries with linear backoff cover network errors, 429 and 5xx. Successful
payloads land in the shared disk cache (`<hash>.body` + `<hash>.meta.json`,
sha256-verified; corrupted entries are evicted). Within `cache_ttl` the cache
is served without touching the network; after that, a failed refetch can serve
the stale copy for `stale_ttl` (`stale_if_error`).

CLI: `--offline` (cache only), `--refresh-external-sources` (ignore fresh
cache), `--external-source=NAME` (narrow the refresh), `--clear-external-cache`.

## SQL (`type: sql`)

Drivers: `mysql`, `mariadb`, `postgres`, `sqlite` (pure Go, no cgo). Queries
live **only** in configuration — never in templates — and are statically
validated: a single statement, `SELECT` (or `WITH … SELECT`) only, no
piggybacked statements. Each query runs under the source `timeout` with a hard
`max_rows` cap (default 10000); exceeding it is an error, not a silent
truncation. DSNs are scrubbed from driver errors.

```yaml
inventory:
  type: sql
  driver: mysql
  dsn: "$INVENTORY_DSN"
  queries:
    products:
      sql: |
        SELECT id, name, slug, price
        FROM products
        WHERE published = true
      max_rows: 10000
```

```gotemplate
{{ range .ExternalData.inventory.products }}{{ .name }}: {{ .price }}{{ end }}
```

Operational rules: use a dedicated read-only database user, and TLS for remote
connections (configure both in the DSN).

## CMS adapters (`type: cms`)

```yaml
wordpress:
  type: cms
  adapter: wordpress      # wordpress | drupal | movable_type
  driver: mysql
  dsn: "$WORDPRESS_DSN"
  mode: content           # content (default) | data
```

`mode: content` merges the import into the site **before** URL, translation,
taxonomy and collision processing — imported posts render, paginate, join
feeds, sitemaps and archives exactly like native content. Imported authors
merge without overwriting IDs the local metadata already defines. `mode: data`
only exposes the import under `.ExternalData.<name>` (pages/posts/authors/
taxonomies/media/metadata maps) — the data view is available in both modes.

### WordPress

`wp_posts`, `wp_users`, `wp_terms`/`wp_term_taxonomy`/`wp_term_relationships`,
`wp_postmeta` and attachments. `post` and custom post types render as posts;
`page` maps to pages. `category`/`post_tag` feed the legacy fields; other
taxonomies land in the page's taxonomies map, so
[dynamic taxonomies](TAXONOMIES.md) pick them up. User-facing custom fields
(keys not starting with `_`) land in `.Extra`.

```yaml
wordpress:
  table_prefix: wp_
  post_types: [post, page, guide]
  statuses: [publish]
  include_media: true
  include_custom_fields: true
  include_taxonomies: true
```

### Drupal (8–11)

`node_field_data`, `node__body`, `users_field_data`,
`taxonomy_term_field_data`/`taxonomy_index` (vocabularies → taxonomies map)
and `path_alias` — aliases are preserved as explicit links, so Drupal URLs
survive the migration. With `include_fields: true`, dynamic `node__field_*`
tables are discovered per engine and land in `.Extra`. Drupal 7 uses a
different schema and is deferred.

```yaml
drupal:
  version: 10
  bundles: [article, page]
  published_only: true
  include_fields: true
```

### Movable Type

`mt_entry` (released entries and pages), `mt_author`,
`mt_category`/`mt_placement`, `mt_tag`/`mt_objecttag` and `mt_asset`.
Comments are deferred.

```yaml
movable_type:
  include_entries: true
  include_pages: true
  include_assets: true
```

## Template API

```gotemplate
{{ .ExternalData.products }}                     — parsed data per source
{{ .ExternalDataMeta.products.FetchedAt }}       — metadata (index and page contexts)
{{ getExternal "products" }}                     — helper, works in every context
{{ getExternalMeta "products" }}                 — Metadata struct
```

Metadata fields: `SourceType`, `Identifier` (always credential-free),
`FetchedAt`, `FromCache`, `Stale`, `Checksum`, `RecordCount`, `ContentType`.

## Transformations

```yaml
transform:
  select: data.items    # dot path into the parsed structure
```

`select` unwraps API envelopes before the data reaches templates. There is
deliberately no scripting, `eval` or embedded query runtime.

## Migration notes

- `.Data` (local `data/` files) is unchanged; the new system is additive.
- MDDB remains a separate content source; it may implement the same connector
  interface later.
- Builds stay deterministic offline: `--offline` + a warmed cache produces
  byte-identical output.

## Troubleshooting

- `failed at fetch … blocked address` — the host resolves to a private IP;
  set `allow_private: true` for self-hosted APIs.
- `offline mode and no cached copy` — run once online, or set
  `fail_on_cache_miss: false` to downgrade the miss to a warning.
- `must reference an environment variable` — secrets belong in the
  environment, not the config file.
- `result exceeds max_rows` — raise the query's `max_rows` or narrow the SQL.

## Deferred (phase 7+)

- Direct-URL template helpers (`getJSON`/`getCSV`/`getXML`).
- Adapters: Ghost, Strapi, Contentful, Sanity, Notion, Airtable,
  Google Sheets, GitHub, GitLab; Drupal 7; Movable Type comments.
- `watch: true` rebuilds on file-source changes.
- Example CMS projects with seed scripts.
{% endraw %}
