package main

// Detecting how this binary was installed (#193).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withoutInstallMarkers clears everything detectInstallOrigin reads, so a test
// asserts on what it sets rather than on the machine it runs on.
//
// The package databases are included deliberately. The suite runs on a Debian
// machine, where /var/lib/dpkg/status exists, so a test that only neutralised
// the others got "apt" from the host and never reached the case it was written
// for — the same shape of ordering dependency as #191.
func withoutInstallMarkers(t *testing.T) {
	t.Helper()
	t.Setenv("SNAP", "")
	t.Setenv("HOMEBREW_PREFIX", "")
	absent := t.TempDir()
	oldDocker, oldDpkg, oldRPM := dockerMarker, dpkgDatabase, rpmDatabases
	dockerMarker = filepath.Join(absent, "no-such-marker")
	dpkgDatabase = filepath.Join(absent, "no-such-dpkg")
	rpmDatabases = []string{filepath.Join(absent, "no-such-rpm")}
	t.Cleanup(func() {
		dockerMarker, dpkgDatabase, rpmDatabases = oldDocker, oldDpkg, oldRPM
	})
}

// TestEachInstallGetsItsOwnCommand. Printing all six upgrade commands is a menu
// the reader has to search; printing the wrong one is worse than none.
func TestEachInstallGetsItsOwnCommand(t *testing.T) {
	withoutInstallMarkers(t)
	t.Setenv("SNAP", "/snap/static-site-generator/42")

	if got := detectInstallOrigin("/snap/static-site-generator/42/bin/ssg"); got.Name != "snap" ||
		got.Upgrade != "sudo snap refresh static-site-generator" {
		t.Errorf("snap = %+v", got)
	}

	t.Setenv("SNAP", "")
	for exe, want := range map[string]string{
		"/opt/homebrew/bin/ssg":                "Homebrew",
		"/usr/local/Cellar/ssg/1.8.47/bin/ssg": "Homebrew",
		"/home/linuxbrew/.linuxbrew/bin/ssg":   "Homebrew",
	} {
		got := detectInstallOrigin(exe)
		if got.Name != want || !strings.Contains(got.Upgrade, "brew upgrade") {
			t.Errorf("detectInstallOrigin(%q) = %+v, want %s", exe, got, want)
		}
	}
}

// TestACustomHomebrewPrefixIsHonoured: Homebrew can be installed anywhere, and
// it says where through the environment.
func TestACustomHomebrewPrefixIsHonoured(t *testing.T) {
	withoutInstallMarkers(t)
	t.Setenv("HOMEBREW_PREFIX", "/srv/brew")
	if got := detectInstallOrigin("/srv/brew/bin/ssg"); got.Name != "Homebrew" {
		t.Errorf("custom prefix = %+v", got)
	}
}

// TestDockerIsRecognisedByItsMarker, and offers a pull rather than a package
// manager that is not in the image.
func TestDockerIsRecognisedByItsMarker(t *testing.T) {
	withoutInstallMarkers(t)
	marker := filepath.Join(t.TempDir(), ".dockerenv")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := dockerMarker
	dockerMarker = marker
	t.Cleanup(func() { dockerMarker = old })

	got := detectInstallOrigin("/usr/local/bin/ssg")
	if got.Name != "Docker" || !strings.Contains(got.Upgrade, "docker pull ghcr.io/spagu/ssg") {
		t.Errorf("docker = %+v", got)
	}
}

// TestSnapWinsOverDocker: a snap runs in its own mount namespace, so both
// markers can be present at once and the more specific one has to win.
func TestSnapWinsOverDocker(t *testing.T) {
	withoutInstallMarkers(t)
	marker := filepath.Join(t.TempDir(), ".dockerenv")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := dockerMarker
	dockerMarker = marker
	t.Cleanup(func() { dockerMarker = old })
	t.Setenv("SNAP", "/snap/static-site-generator/42")

	if got := detectInstallOrigin("/snap/static-site-generator/42/bin/ssg"); got.Name != "snap" {
		t.Errorf("origin = %+v, want snap", got)
	}
}

// TestSnapNeedsTheExecutableUnderIt, not merely the variable. $SNAP is exported
// to every process a snap starts — a Go toolchain run from inside one included —
// so the variable alone proves nothing.
func TestSnapNeedsTheExecutableUnderIt(t *testing.T) {
	withoutInstallMarkers(t)
	t.Setenv("SNAP", "/snap/something-else/7")

	if got := detectInstallOrigin("/home/me/bin/ssg"); got.Name == "snap" {
		t.Errorf("a binary outside $SNAP is not the snap build: %+v", got)
	}
	if !isSnapInstall("/snap/something-else/7/bin/ssg") {
		t.Error("a binary under $SNAP is the snap build")
	}
	if isSnapInstall("") {
		t.Error("an unknown executable path cannot be the snap build")
	}
	t.Setenv("SNAP", "")
	if isSnapInstall("/snap/x/1/bin/ssg") {
		t.Error("without $SNAP there is no snap build")
	}
}

// TestAHandPlacedBinaryIsNobodysToUpgrade: a binary copied into /usr/bin on a
// Debian machine is not apt's, and `apt install --only-upgrade` would not move
// it. Falling back to the releases page is the honest answer.
func TestAHandPlacedBinaryIsNobodysToUpgrade(t *testing.T) {
	withoutInstallMarkers(t)
	got := detectInstallOrigin("/home/me/.local/bin/ssg")
	if got.Name != "a downloaded binary" || got.Upgrade != "" {
		t.Errorf("origin = %+v", got)
	}
	// The package check needs both the path and the package database.
	if packagedAt("/home/me/bin/ssg", "/") {
		t.Error("a path outside the system directories is not package-managed")
	}
	if packagedAt("/usr/bin/ssg", filepath.Join(t.TempDir(), "absent")) {
		t.Error("no package database means no package")
	}
}

// TestAPathOutsideTheSystemDirectoriesIsNeverPackageManaged, whatever database
// the machine happens to have.
func TestAPathOutsideTheSystemDirectoriesIsNeverPackageManaged(t *testing.T) {
	for _, exe := range []string{"/home/me/ssg", "/opt/ssg/bin/ssg", ""} {
		if isRPMInstall(exe) || isDebInstall(exe) {
			t.Errorf("%q must not be package-managed", exe)
		}
	}
}

// TestExecutableOfResolvesSymlinks: /usr/local/bin/ssg pointing into a Homebrew
// cellar is a Homebrew install, and the unresolved path says otherwise.
func TestExecutableOfResolvesSymlinks(t *testing.T) {
	if got := executableOf(); got == "" {
		t.Fatal("the running executable must be identifiable")
	} else if !filepath.IsAbs(got) {
		t.Errorf("executableOf = %q, want an absolute path", got)
	}
}

// TestACellarPathAnywhereIsHomebrew: a tap installed under a prefix nobody
// declared still lands in a directory named Cellar.
func TestACellarPathAnywhereIsHomebrew(t *testing.T) {
	withoutInstallMarkers(t)
	if got := detectInstallOrigin("/data/brew/Cellar/ssg/1.8.47/bin/ssg"); got.Name != "Homebrew" {
		t.Errorf("origin = %+v", got)
	}
	if isHomebrewInstall("") {
		t.Error("an unknown executable path is not Homebrew")
	}
}

// TestAnRPMMachineGetsDnf: asserted through the predicate rather than the host,
// so the mapping is covered wherever the suite runs.
func TestAnRPMMachineGetsDnf(t *testing.T) {
	withoutInstallMarkers(t)
	rpmDatabases = []string{t.TempDir()} // restored by withoutInstallMarkers

	got := detectInstallOrigin("/usr/bin/ssg")
	if got.Name != "dnf" || !strings.Contains(got.Upgrade, "dnf upgrade ssg") {
		t.Errorf("origin = %+v", got)
	}
}

// TestADebMachineGetsApt, the same way.
func TestADebMachineGetsApt(t *testing.T) {
	withoutInstallMarkers(t)
	f := filepath.Join(t.TempDir(), "status")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dpkgDatabase = f // restored by withoutInstallMarkers

	got := detectInstallOrigin("/usr/bin/ssg")
	if got.Name != "apt" || !strings.Contains(got.Upgrade, "apt install --only-upgrade ssg") {
		t.Errorf("origin = %+v", got)
	}
}

// TestResolvingTheExecutableHandlesBothFailures.
func TestResolvingTheExecutableHandlesBothFailures(t *testing.T) {
	if got := resolveExecutable("/anything", errors.New("unsupported")); got != "" {
		t.Errorf("an unknown executable = %q, want empty", got)
	}
	// A path that cannot be resolved still identifies the binary better than
	// nothing, so it comes back unresolved rather than empty.
	missing := filepath.Join(t.TempDir(), "gone")
	if got := resolveExecutable(missing, nil); got != missing {
		t.Errorf("unresolvable path = %q, want it unchanged", got)
	}
	// The ordinary case resolves.
	real := filepath.Join(t.TempDir(), "real")
	if err := os.WriteFile(real, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolveExecutable(real, nil); got == "" {
		t.Error("an existing path must resolve")
	}
}
