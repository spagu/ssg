package migrate

// Choosing the engine that runs (#160). The tests use real files in a temp tree
// — a binary that cannot be stat'd is exactly what the code decides on — but
// never execute one: the version banner comes through the Version seam.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEngine writes an executable file and returns its path.
func fakeEngine(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// bannersFor answers with each path's version, so a test states the versions it
// is choosing between instead of installing them.
func bannersFor(versions map[string]string) func(string) string {
	return func(bin string) string {
		if v, ok := versions[bin]; ok {
			return "wpexporter version v" + v
		}
		return ""
	}
}

// snapEnv puts the process inside a snap the way a confined run really is: $SNAP
// set AND this executable living under it. $SNAP alone is set for every snap
// process — including the Go snap that builds ssg — so the executable test is
// what makes the search look in the right tree.
func snapEnv(t *testing.T, root string) {
	t.Helper()
	t.Setenv("SNAP", root)
	exe := fakeEngine(t, filepath.Join(root, "bin"), "ssg")
	executablePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executablePath = os.Executable })
}

// TestSelectEnginePrefersTheNewest: the whole point. A host engine newer than
// the bundled one is what the operator installed and what must run — the lag
// between a wpexporter release and a snap rebuild is the bug being fixed.
func TestSelectEnginePrefersTheNewest(t *testing.T) {
	snap := t.TempDir()
	home := t.TempDir()
	snapEnv(t, snap)
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	bundled := fakeEngine(t, filepath.Join(snap, "bin"), wpexporterBinary)
	hostBin := fakeEngine(t, filepath.Join(home, "go", "bin"), wpexporterBinary)

	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return bundled, nil },
		Version:  bannersFor(map[string]string{bundled: "1.8.11", hostBin: "1.8.12"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice.Path != hostBin {
		t.Fatalf("ran %s, want the newer host engine %s", choice.Path, hostBin)
	}
	if choice.Origin != "$HOME" {
		t.Errorf("origin = %q", choice.Origin)
	}
}

// TestSelectEngineKeepsBundledWhenItIsNewest: the bundled engine is a floor,
// not a ceiling — an older host copy must not win.
func TestSelectEngineKeepsBundledWhenItIsNewest(t *testing.T) {
	snap := t.TempDir()
	home := t.TempDir()
	snapEnv(t, snap)
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	bundled := fakeEngine(t, filepath.Join(snap, "bin"), wpexporterBinary)
	old := fakeEngine(t, filepath.Join(home, ".local", "bin"), wpexporterBinary)

	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return bundled, nil },
		Version:  bannersFor(map[string]string{bundled: "1.8.12", old: "1.8.4"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice.Path != bundled {
		t.Fatalf("ran %s, want the bundled engine %s", choice.Path, bundled)
	}
}

// TestSelectEngineOutsideASnapUsesPath: an ordinary install searches nothing —
// PATH is already the answer, and probing someone's home directory would be
// both pointless and surprising.
func TestSelectEngineOutsideASnapUsesPath(t *testing.T) {
	t.Setenv("SNAP", "")
	t.Setenv(engineEnvVar, "")
	bin := fakeEngine(t, t.TempDir(), wpexporterBinary)

	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return bin, nil },
		Version:  bannersFor(map[string]string{bin: "1.8.12"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice.Path != bin || choice.Origin != "PATH" {
		t.Fatalf("choice = %+v, want %s from PATH", choice, bin)
	}
	if len(choice.Skipped) != 0 {
		t.Errorf("nothing was passed over, got %v", choice.Skipped)
	}
}

// TestForeignSnapIsNotOurs: $SNAP is set for every snap process. An ssg built
// or launched from inside the Go snap must not go looking for an engine under
// someone else's tree.
func TestForeignSnapIsNotOurs(t *testing.T) {
	t.Setenv("SNAP", "/snap/go/11227")
	executablePath = func() (string, error) { return "/usr/local/bin/ssg", nil }
	t.Cleanup(func() { executablePath = os.Executable })

	if got := confinedSnapRoot(); got != "" {
		t.Fatalf("confinedSnapRoot = %q — another snap's $SNAP is not ours", got)
	}
}

// TestSnapWrapperIsPassedOver: /snap/bin/wpexporter is a symlink to snapd, and
// a confined snap cannot join snapd's mount namespace. Running it fails with
// "cannot join mount namespace of pid 1", so it is named and skipped rather
// than chosen.
func TestSnapWrapperIsPassedOver(t *testing.T) {
	snapEnv(t, t.TempDir()) // only a confined ssg is stopped by another snap
	dir := t.TempDir()
	snapd := fakeEngine(t, dir, "snap")
	wrapper := filepath.Join(dir, wpexporterBinary)
	if err := os.Symlink(snapd, wrapper); err != nil {
		t.Fatal(err)
	}
	if reason := unrunnableReason(wrapper); reason != snapWrapperReason {
		t.Fatalf("reason = %q, want the snap-wrapper explanation", reason)
	}
	if !isSnapWrapper("/snap/bin/wpexporter") {
		t.Error("a /snap/bin entry is a snapd wrapper")
	}
	// The false positive that would matter: an ordinary engine binary, and one
	// reached through a symlink that is not snapd, both stay runnable.
	plain := fakeEngine(t, t.TempDir(), wpexporterBinary)
	if isSnapWrapper(plain) {
		t.Error("an ordinary wpexporter binary must not be mistaken for a snap")
	}
	linked := filepath.Join(t.TempDir(), wpexporterBinary)
	if err := os.Symlink(plain, linked); err != nil {
		t.Fatal(err)
	}
	if isSnapWrapper(linked) {
		t.Error("a symlink to a real engine is still a real engine")
	}
	if reason := unrunnableReason(linked); reason != "" {
		t.Errorf("a symlinked engine must be runnable: %q", reason)
	}
	_ = snapd
}

// TestSelectEngineHonoursExplicitPath: someone who names a binary means that
// binary — silently running a different one is how this went unnoticed for two
// releases.
func TestSelectEngineHonoursExplicitPath(t *testing.T) {
	t.Setenv("SNAP", "")
	t.Setenv(engineEnvVar, "")
	onPath := fakeEngine(t, t.TempDir(), wpexporterBinary)
	named := fakeEngine(t, t.TempDir(), wpexporterBinary)

	choice, err := selectEngine(Options{
		EnginePath: named,
		LookPath:   func(string) (string, error) { return onPath, nil },
		Version:    bannersFor(map[string]string{onPath: "9.9.9", named: "1.8.11"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice.Path != named {
		t.Fatalf("ran %s — an explicit engine wins even when it is older", choice.Path)
	}

	// The environment says the same thing when no flag does.
	t.Setenv(engineEnvVar, named)
	choice, err = selectEngine(Options{
		LookPath: func(string) (string, error) { return onPath, nil },
		Version:  bannersFor(map[string]string{onPath: "9.9.9", named: "1.8.11"}),
	})
	if err != nil || choice.Path != named {
		t.Fatalf("%s must be honoured: %+v %v", engineEnvVar, choice, err)
	}
}

// TestExplicitEngineNeverFallsBack: a named engine that cannot run is an error
// naming it. Falling back would run something the operator did not ask for and
// report success.
func TestExplicitEngineNeverFallsBack(t *testing.T) {
	t.Setenv("SNAP", "")
	t.Setenv(engineEnvVar, "")
	onPath := fakeEngine(t, t.TempDir(), wpexporterBinary)
	missing := filepath.Join(t.TempDir(), "nowhere", wpexporterBinary)

	_, err := selectEngine(Options{
		EnginePath: missing,
		LookPath:   func(string) (string, error) { return onPath, nil },
		Version:    bannersFor(map[string]string{onPath: "1.8.12"}),
	})
	if err == nil {
		t.Fatal("a named engine that is not there must be an error")
	}
	for _, want := range []string{missing, reasonNotFound, engineEnvVar} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error must mention %q: %v", want, err)
		}
	}

	// A directory and a non-executable file are named just as plainly.
	dir := t.TempDir()
	if _, err := selectEngine(Options{EnginePath: dir}); err == nil ||
		!strings.Contains(err.Error(), "not a file") {
		t.Errorf("a directory: %v", err)
	}
	plain := filepath.Join(dir, "notexec")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := selectEngine(Options{EnginePath: plain}); err == nil ||
		!strings.Contains(err.Error(), "not executable") {
		t.Errorf("a non-executable file: %v", err)
	}
}

// TestNoEngineAnywhereExplainsItself: the message has to fit the install — a
// snap that lost its bundled copy is a different problem from a bare PATH.
func TestNoEngineAnywhereExplainsItself(t *testing.T) {
	t.Setenv("SNAP", "")
	t.Setenv(engineEnvVar, "")
	_, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err == nil || !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("outside a snap: %v", err)
	}

	snap := t.TempDir()
	snapEnv(t, snap)
	userHomeDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })
	_, err = selectEngine(Options{
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err == nil || !strings.Contains(err.Error(), "missing from this snap") {
		t.Fatalf("inside a snap: %v", err)
	}
}

// TestMissingHomeDoesNotStopTheSearch: an unreadable home is not a reason to
// fail — the bundled engine is still there.
func TestMissingHomeDoesNotStopTheSearch(t *testing.T) {
	snap := t.TempDir()
	snapEnv(t, snap)
	userHomeDir = func() (string, error) { return "", os.ErrPermission }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })
	bundled := fakeEngine(t, filepath.Join(snap, "bin"), wpexporterBinary)

	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		Version:  bannersFor(map[string]string{bundled: "1.8.11"}),
	})
	if err != nil || choice.Path != bundled {
		t.Fatalf("choice = %+v, err = %v", choice, err)
	}
}

// TestCompareVersions covers the ordering the choice rests on.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b [3]int
		want int
	}{
		{[3]int{1, 8, 12}, [3]int{1, 8, 11}, 1},
		{[3]int{1, 8, 11}, [3]int{1, 8, 12}, -1},
		{[3]int{1, 8, 11}, [3]int{1, 8, 11}, 0},
		{[3]int{2, 0, 0}, [3]int{1, 99, 99}, 1},
		{[3]int{1, 9, 0}, [3]int{1, 8, 99}, 1},
		{[3]int{0, 0, 0}, [3]int{0, 0, 0}, 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%v, %v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestUnreadableBannerStillRuns: a wrapper or a fork that prints no version is
// not proof of an old engine, so it is used rather than rejected.
func TestUnreadableBannerStillRuns(t *testing.T) {
	t.Setenv("SNAP", "")
	t.Setenv(engineEnvVar, "")
	bin := fakeEngine(t, t.TempDir(), wpexporterBinary)
	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return bin, nil },
		Version:  func(string) string { return "some fork, no version" },
	})
	if err != nil || choice.Path != bin {
		t.Fatalf("choice = %+v, err = %v", choice, err)
	}
}

// TestEngineSelectionNotesReportWhatWasSkipped: the line that answers "I
// installed a newer wpexporter and nothing changed".
func TestEngineSelectionNotes(t *testing.T) {
	notes := engineSelectionNotes(engineChoice{
		Skipped: []string{"/snap/bin/wpexporter (" + snapWrapperReason + ")"},
	})
	if len(notes) != 1 || !strings.Contains(notes[0], "skipped /snap/bin/wpexporter") {
		t.Fatalf("notes = %v", notes)
	}
	if got := engineSelectionNotes(engineChoice{}); len(got) != 0 {
		t.Errorf("nothing skipped, nothing said: %v", got)
	}
}

// TestSearchReportsAnUnrunnableEngine: an engine that exists and cannot be run
// reaches the report, while a location that simply held nothing stays quiet.
// The difference matters — "I installed it and nothing changed" deserves an
// answer, an empty directory does not deserve a line.
func TestSearchReportsAnUnrunnableEngine(t *testing.T) {
	snap := t.TempDir()
	home := t.TempDir()
	snapEnv(t, snap)
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	bundled := fakeEngine(t, filepath.Join(snap, "bin"), wpexporterBinary)
	// A host copy that is there but has no execute bit — a downloaded release
	// binary nobody chmod'ed.
	dud := filepath.Join(home, "bin", wpexporterBinary)
	if err := os.MkdirAll(filepath.Dir(dud), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dud, []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		Version:  bannersFor(map[string]string{bundled: "1.8.11"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice.Path != bundled {
		t.Fatalf("the runnable engine must win: %+v", choice)
	}
	if len(choice.Skipped) != 1 || !strings.Contains(choice.Skipped[0], "not executable") {
		t.Fatalf("the unrunnable engine must be named once: %v", choice.Skipped)
	}
	// go/bin and .local/bin held nothing and are not worth a word.
	for _, s := range choice.Skipped {
		if strings.Contains(s, reasonNotFound) {
			t.Errorf("an empty location must not be reported: %s", s)
		}
	}
}

// TestSnapEngineRunsFromAnOrdinaryInstall: the commonest setup there is — ssg
// from a package or `go install`, wpexporter from the snap store. /snap/bin
// works fine for an unconfined process, so it must be chosen, not refused.
// Only a snap-installed ssg is stopped by another snap.
func TestSnapEngineRunsFromAnOrdinaryInstall(t *testing.T) {
	t.Setenv("SNAP", "")
	t.Setenv(engineEnvVar, "")

	if snapWrapperBlocks("/snap/bin/wpexporter") {
		t.Fatal("an unconfined ssg can run the wpexporter snap")
	}
	if reason := unrunnableReason("/snap/bin/wpexporter"); reason == snapWrapperReason {
		t.Fatal("the snap wrapper must not be refused outside a snap")
	}

	// And PATH's hit is used as found, wherever it lives.
	choice, err := selectEngine(Options{
		LookPath: func(string) (string, error) { return "/snap/bin/wpexporter", nil },
		Version:  bannersFor(map[string]string{"/snap/bin/wpexporter": "1.8.12"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice.Path != "/snap/bin/wpexporter" {
		t.Fatalf("choice = %+v, want the snap engine", choice)
	}
	if len(choice.Skipped) != 0 {
		t.Errorf("nothing to pass over: %v", choice.Skipped)
	}
}
