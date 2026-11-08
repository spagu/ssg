// Package snapenv tells whether this process is the strictly confined snap
// build of ssg. Confinement changes what ssg can reach: host programs (hooks,
// wpexporter, cwebp) are invisible, and /tmp is private to the snap.
package snapenv

import (
	"os"
	"strings"
)

// executable is a seam so tests can place the binary under a fake snap root.
var executable = os.Executable

// Root returns the snap root this ssg runs from, or "" when it is not the
// snap build. See RootOf.
func Root() string {
	exe, err := executable()
	if err != nil {
		return ""
	}
	return RootOf(exe)
}

// RootOf returns $SNAP when exe lives under it, else "".
//
// $SNAP alone proves nothing: snapd exports it to every process a snap starts,
// so an ssg built or launched from inside the Go snap sees a $SNAP that has
// nothing to do with it. Only an executable under that root is the snap build.
func RootOf(exe string) string {
	snap := os.Getenv("SNAP")
	if snap == "" || exe == "" || !strings.HasPrefix(exe, snap+string(os.PathSeparator)) {
		return ""
	}
	return snap
}
