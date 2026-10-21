package main

// Which ssg this is (#337).
//
// A release build stamps the version with -ldflags "-X main.Version=…", as the
// Makefile, the Dockerfile and the release workflow do. `go install
// github.com/spagu/ssg/cmd/ssg@v1.8.73` does not, so a binary installed the
// way the README documents called itself "dev" — and so did every site footer
// that printed it. The Go toolchain records the module version in the binary
// anyway; this reads it when nothing was stamped.

import (
	"runtime/debug"
	"strings"
)

// devVersion is what Version holds when no build flag set it.
const devVersion = "dev"

func init() {
	Version = resolveVersion(Version, debug.ReadBuildInfo)
}

// resolveVersion keeps a stamped version, and otherwise takes the module
// version Go recorded, written without the "v" as a stamped release is:
// "1.8.73" for a `go install …@v1.8.73` build, and for a build in a git
// checkout the version Go derives from its tags ("1.8.73+dirty" with local
// changes). "(devel)", recorded when Go knows nothing better, stays "dev".
func resolveVersion(stamped string, read func() (*debug.BuildInfo, bool)) string {
	if stamped != devVersion {
		return stamped
	}
	info, ok := read()
	if !ok || info == nil {
		return stamped
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return stamped
	}
	return strings.TrimPrefix(v, "v")
}
