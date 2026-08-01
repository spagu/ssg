# Server endpoints, content contracts and structured data (1.8.14)

[`ssg.yaml`](ssg.yaml) is a commented reference config for the features that
landed in 1.8.14. It is documentation-by-example — copy the blocks you need into
your own config.

## What it shows

- **Portable endpoints (#63).** One vendor-neutral `endpoints:` block with all
  four types — `redirect`, `proxy`, `form` and `auth`. They run natively on the
  built-in server (`ssg … --http`, self-hosted, no runtime), and
  `endpoints_platform: cloudflare` compiles the same declaration to Cloudflare
  Pages Functions at build time. Switch it to `netlify` or `vercel` and the
  functions change; your config does not. See
  [`docs/CONFIGURATION.md`](../../docs/CONFIGURATION.md#server-endpoints-portable-no-vendor-lock-in).
- **Content contracts (#62).** `content_schemas:` validate frontmatter per type;
  `strict` (or `--strict`) makes violations and broken links fail the build;
  `route_manifest` writes `routes.json`.
- **AI-first structured data (#61).** `seo: true` injects JSON-LD from
  frontmatter — alongside a theme's own OpenGraph — and `schema:` adds a
  site-wide publisher to every page.

## Notes

- The `auth` guard and the SSRF-guarded `proxy`/`form` delivery run on the
  **built-in server**; on a serverless platform, use that platform's own access
  control, and the proxy/webhook runs at the edge.
- `password: $MEMBERS_PW` is read from the environment — keep secrets out of the
  config file.
- This site itself dogfoods `seo`, `schema` and `route_manifest` — see the
  repository's `docs-site.yaml`.
