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
// templates scaffolded by `ssg new worker <template>` (GO-066): contact-form,
// stripe-checkout, dynamic-price and conversions-proxy. They live at the module
// root for the same go:embed reason as the themes above.
//
//go:embed workers
var EmbeddedWorkers embed.FS
