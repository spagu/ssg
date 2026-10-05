package main

// The snap's private /tmp (#322). Strict confinement gives the snap its own
// /tmp and /var/tmp: a build told to write to /tmp/site succeeds, prints the
// path, and leaves nothing there on the host. A script that then reads the
// directory fails far from the cause, so the build says it up front.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/generator"
)

// snapPrivateDirs are the host directories a strictly confined snap sees
// only as its own private copies.
var snapPrivateDirs = []string{"/tmp", "/var/tmp"}

// snapPrivatePaths lists the configured paths that would land in the snap's
// private copies, as "role path" pairs; nil outside a snap.
func snapPrivatePaths(cfg generator.Config, inSnap bool) []string {
	if !inSnap {
		return nil
	}
	var out []string
	for _, p := range []struct{ role, path string }{
		{"output", cfg.OutputDir}, {"content", cfg.ContentDir}, {"templates", cfg.TemplatesDir},
	} {
		if p.path == "" {
			continue
		}
		abs, err := filepath.Abs(p.path)
		if err != nil {
			continue
		}
		for _, dir := range snapPrivateDirs {
			if abs == dir || strings.HasPrefix(abs, dir+string(os.PathSeparator)) {
				out = append(out, p.role+" "+abs)
				break
			}
		}
	}
	return out
}

// warnSnapPrivatePaths prints the warning when the snap build is pointed at
// its private /tmp.
func warnSnapPrivatePaths(w io.Writer, cfg generator.Config, inSnap bool) {
	for _, p := range snapPrivatePaths(cfg, inSnap) {
		role, path, _ := strings.Cut(p, " ")
		_, _ = fmt.Fprintf(w, "⚠️  The snap has its own private /tmp: the %s directory %s is inside it, not on the host.\n"+
			"   Use a directory under your home instead, or install ssg another way (docs/INSTALL.md).\n", role, path)
	}
}
