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
