// Package ssg exposes assets compiled into the binary. The bundled starter
// themes live at the module root because go:embed cannot reference parent
// directories (DOC-013).
package ssg

import "embed"

// EmbeddedThemes carries the bundled starter themes (templates/simple and
// templates/krowy), scaffolded on first use when the requested theme has no
// local template files — this is what makes `ssg my-blog simple example.com`
// work without a checkout of the repository.
//
//go:embed templates/simple templates/krowy
var EmbeddedThemes embed.FS

// EmbeddedWorkers carries the batteries-included Cloudflare Pages Functions
// templates scaffolded by `ssg new worker <template>` (GO-066). They live at
// the module root for the same go:embed reason as the themes above.
//
// `all:` is required, not a plain `//go:embed workers`: a Pages Function whose
// filename starts with `_` (a shared, non-routed module like comments'
// `_lib.ts`) is excluded by go:embed's default `_`/`.` rule, so the scaffold
// would drop it and the importing functions would fail to build (GO-078).
//
// The workers are listed one by one rather than as `all:workers`, because a
// worker with a test suite has a node_modules directory beside its source, and
// `all:workers` swept that into the binary — six hundred megabytes of test
// tooling, shipped to every user, and copied into their project by `ssg new
// worker`. TestEmbeddedWorkersCoverEveryWorker fails when a new worker is added
// here on disk and not added to this list.
//
//go:embed all:workers/comments
//go:embed all:workers/contact-form
//go:embed all:workers/conversions-proxy
//go:embed all:workers/cookie-consent
//go:embed all:workers/dynamic-price
//go:embed all:workers/ecommerce/functions
//go:embed all:workers/ecommerce/public
//go:embed all:workers/ecommerce/scripts
//go:embed all:workers/ecommerce/test
//go:embed workers/ecommerce/README.md
//go:embed workers/ecommerce/package.json
//go:embed workers/ecommerce/tsconfig.json
//go:embed workers/ecommerce/vitest.config.ts
//go:embed workers/ecommerce/wrangler.snippet.toml
//go:embed all:workers/rate-limit
//go:embed all:workers/republish-trigger
//go:embed all:workers/stripe-checkout
var EmbeddedWorkers embed.FS
