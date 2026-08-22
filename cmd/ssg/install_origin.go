package main

// How this binary got here, so an upgrade hint names the right command (#193).
//
// ssg installs six ways — snap, Homebrew, apt, dnf/rpm, Docker and a raw
// binary — and each has its own upgrade command. Printing all six is not help,
// it is a menu the reader has to work through to find their own line. Printing
// the wrong one is worse than printing none.
//
// So the origin is detected from where the running executable actually sits,
// not from the operating system: a Homebrew ssg on Linux is still Homebrew, and
// a raw binary dropped in /usr/local/bin on a Debian box is not apt's.

import (
	"os"
	"path/filepath"
	"strings"
)

// installOrigin is one way ssg reaches a machine.
type installOrigin struct {
	// Name is how a human refers to it.
	Name string
	// Upgrade is the command that updates it, empty when there is none to give.
	Upgrade string
}

// dockerMarker is the file every container runtime leaves in the filesystem
// root. A variable so a test can point it somewhere that does not exist.
var dockerMarker = "/.dockerenv"

// executableOf reports the running binary's path, resolved through symlinks —
// /usr/local/bin/ssg pointing into a Homebrew cellar is a Homebrew install, and
// the unresolved path would say otherwise.
func executableOf() string { return resolveExecutable(os.Executable()) }

// resolveExecutable is the testable half: both failures it handles are real —
// a platform where os.Executable is unsupported, and a path that cannot be
// resolved because it was deleted or sits behind a permission the process does
// not have — and neither is reachable through executableOf from a test.
func resolveExecutable(exe string, err error) string {
	if err != nil {
		return ""
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		return resolved
	}
	return exe // unresolvable: the raw path still identifies it better than nothing
}

// detectInstallOrigin identifies how this binary was installed.
//
// Order matters. A snap runs inside a container-like mount namespace and a
// Homebrew formula can live under /usr/local, so the more specific markers are
// tested before the more general ones.
func detectInstallOrigin(exe string) installOrigin {
	switch {
	case isSnapInstall(exe):
		return installOrigin{Name: "snap", Upgrade: "sudo snap refresh static-site-generator"}
	case isHomebrewInstall(exe):
		return installOrigin{Name: "Homebrew", Upgrade: "brew upgrade spagu/tap/ssg"}
	case isDockerInstall():
		return installOrigin{Name: "Docker", Upgrade: "docker pull ghcr.io/spagu/ssg:latest"}
	case isDebInstall(exe):
		return installOrigin{Name: "apt", Upgrade: "sudo apt update && sudo apt install --only-upgrade ssg"}
	case isRPMInstall(exe):
		return installOrigin{Name: "dnf", Upgrade: "sudo dnf upgrade ssg"}
	}
	return installOrigin{Name: "a downloaded binary"}
}

// isSnapInstall reports whether this is the snap build. $SNAP is exported to
// every process a snap starts — including a Go toolchain run from inside one —
// so the variable alone proves nothing; the executable has to live under it.
func isSnapInstall(exe string) bool {
	snap := os.Getenv("SNAP")
	return snap != "" && exe != "" && strings.HasPrefix(exe, snap+string(os.PathSeparator))
}

// isHomebrewInstall covers both prefixes Homebrew uses (Apple silicon and
// Intel/Linuxbrew) plus a custom one named by the environment.
func isHomebrewInstall(exe string) bool {
	if exe == "" {
		return false
	}
	prefixes := []string{"/opt/homebrew", "/usr/local/Cellar", "/home/linuxbrew/.linuxbrew"}
	if custom := os.Getenv("HOMEBREW_PREFIX"); custom != "" {
		prefixes = append(prefixes, custom)
	}
	for _, p := range prefixes {
		if strings.HasPrefix(exe, p+string(os.PathSeparator)) {
			return true
		}
	}
	return strings.Contains(exe, string(os.PathSeparator)+"Cellar"+string(os.PathSeparator))
}

// isDockerInstall reports whether this is running inside a container.
func isDockerInstall() bool {
	_, err := os.Stat(dockerMarker)
	return err == nil
}

// isDebInstall and isRPMInstall recognise a package-managed binary by the
// package database that claims the path, not by the distribution: a binary
// copied into /usr/bin by hand is not apt's to upgrade.
// Variables rather than constants so a test can point them at a directory it
// controls: the mapping from "this machine uses dpkg" to "say apt" would
// otherwise only ever be exercised on a machine that happens to use dpkg.
var (
	dpkgDatabase = "/var/lib/dpkg/status"
	rpmDatabases = []string{"/var/lib/rpm", "/usr/lib/sysimage/rpm"}
)

func isDebInstall(exe string) bool {
	return packagedAt(exe, dpkgDatabase)
}

func isRPMInstall(exe string) bool {
	for _, db := range rpmDatabases {
		if packagedAt(exe, db) {
			return true
		}
	}
	return false
}

// packagedAt reports whether exe sits in a system binary directory on a machine
// whose package database exists.
func packagedAt(exe, database string) bool {
	if exe != "/usr/bin/ssg" && exe != "/usr/local/bin/ssg" {
		return false
	}
	_, err := os.Stat(database)
	return err == nil
}
