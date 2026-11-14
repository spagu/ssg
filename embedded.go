// Package ssg exposes assets compiled into the binary. The bundled starter
// themes live at the module root because go:embed cannot reference parent
// directories (DOC-013).
package ssg

import "embed"

// EmbeddedThemes carries the bundled starter themes (templates/simple,
// templates/krowy and templates/apidoc), scaffolded on first use when the requested theme has no
// local template files — this is what makes `ssg my-blog simple example.com`
// work without a checkout of the repository.
//
//go:embed templates/simple templates/krowy templates/apidoc
var EmbeddedThemes embed.FS

// EmbeddedWorkers carries the batteries-included Cloudflare Pages Functions
// templates scaffolded by `ssg new worker <template>` (GO-066): contact-form,
// stripe-checkout, dynamic-price, conversions-proxy, cookie-consent, comments,
// republish-trigger, rate-limit, newsletter and web-push — plus newsletter-ui,
// a standalone Worker that ships its own wrangler.toml and is deployed with
// `wrangler deploy` rather than merged into the site's Pages build (GO-123).
// A new directory under workers/ registers itself — `ssg new worker` lists
// whatever is embedded. They live at the module root for the same go:embed
// reason as the themes above.
//
// `all:` is required, not a plain `//go:embed workers`: a Pages Function whose
// filename starts with `_` (a shared, non-routed module like comments'
// `_lib.ts`) is excluded by go:embed's default `_`/`.` rule, so the scaffold
// would drop it and the importing functions would fail to build (GO-078).
//
// Every template carries a module.json (GO-143, docs/WORKER-MODULES.md). The
// loose files beside the templates — module.schema.json, the TypeScript reader
// module-manifest.ts and the cross-template tests — ride along too, but only
// directories are templates, so `ssg new worker` neither lists nor scaffolds
// them; the schema is embedded for the build to read (GO-144).
//
// It embeds whatever workers/ holds when the binary is built, so a local build
// from a tree with untracked worker directories lists templates a release
// does not have (#380). Release binaries are built from a clean checkout.
//
//go:embed all:workers
var EmbeddedWorkers embed.FS
