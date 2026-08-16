package migrate

// Choosing which wpexporter runs (#160).
//
// ssg used to take the first wpexporter on PATH. Inside the snap that is always
// the bundled copy, so an engine the operator installed themselves was never
// used and every engine fix waited for the next snap rebuild. Twice now the
// same report arrived: a fix ships in wpexporter, the host is upgraded, the
// migration keeps producing the old export.
//
// What a strictly-confined snap can actually execute, measured rather than
// assumed:
//
//   - $SNAP/bin/wpexporter — the bundled copy. Runs.
//   - /snap/bin/wpexporter — the wpexporter snap. Does NOT run: it is a symlink
//     to snapd, and a confined snap cannot join snapd's mount namespace.
//   - an absolute path under the real $HOME — runs. The `home` interface grants
//     read and execute, so `go install …/cmd/wpexporter@latest` is reachable.
//
// So the rule is: run the newest engine that can actually be executed, prefer an
// explicit choice over any search, and never fail on a candidate that merely
// cannot run — pass over it and say why.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// engineEnvVar names the engine outside any flag, for a shell or CI that would
// rather set it once than repeat it.
const engineEnvVar = "SSG_WPEXPORTER"

// hostEngineDirs are where a host installs a real wpexporter binary under $HOME.
// They are searched only inside a snap: outside one, PATH already covers them.
var hostEngineDirs = []string{"go/bin", ".local/bin", "bin"}

// userHomeDir and executablePath are seams so a test can point the host search
// at a temp tree without moving the process.
var (
	userHomeDir    = os.UserHomeDir
	executablePath = os.Executable
)

// confinedSnapRoot returns the snap ssg is running inside, or "" when it is not
// confined.
//
// $SNAP alone does not answer that: it is set for EVERY snap process, so an ssg
// built or launched from inside the Go snap sees a $SNAP that has nothing to do
// with it, and would go looking for an engine under someone else's tree. The
// test that settles it is whether this executable lives under that root.
func confinedSnapRoot() string {
	snap := os.Getenv("SNAP")
	if snap == "" {
		return ""
	}
	exe, err := executablePath()
	if err != nil || !strings.HasPrefix(exe, snap+string(os.PathSeparator)) {
		return ""
	}
	return snap
}

// engineCandidate is one wpexporter ssg might run, with where it came from so
// the report can explain the choice.
type engineCandidate struct {
	Path   string
	Origin string // "bundled with the snap", "PATH", "$HOME"
}

// engineChoice is the engine that will run, and what was passed over on the way.
type engineChoice struct {
	Path    string
	Banner  string
	Origin  string
	Skipped []string // human-readable reasons, reported once, never fatal
}

// selectEngine picks the newest wpexporter that can be executed.
//
// An explicit path (--engine or SSG_WPEXPORTER) wins outright: someone who
// names a binary means that binary, and silently running a different one would
// be worse than failing.
func selectEngine(opts Options) (engineChoice, error) {
	if explicit := explicitEngine(opts); explicit != "" {
		return chooseExplicitEngine(explicit, opts)
	}

	candidates, skipped := gatherEngineCandidates(opts)
	if len(candidates) == 0 {
		return engineChoice{}, fmt.Errorf("%s", missingEngineMessage(os.Getenv("SNAP")))
	}
	return newestEngine(candidates, skipped, opts), nil
}

// explicitEngine returns the operator's named engine, flag before environment.
func explicitEngine(opts Options) string {
	if p := strings.TrimSpace(opts.EnginePath); p != "" {
		return p
	}
	return strings.TrimSpace(os.Getenv(engineEnvVar))
}

// chooseExplicitEngine validates a named engine and refuses to fall back. A
// silent fallback is how the original problem stayed invisible for two releases.
func chooseExplicitEngine(path string, opts Options) (engineChoice, error) {
	if reason := unrunnableReason(path); reason != "" {
		return engineChoice{}, fmt.Errorf(`the engine you named cannot be run: %s
   %s
Name a real wpexporter binary (--engine /path/to/wpexporter, or %s)`,
			path, reason, engineEnvVar)
	}
	return engineChoice{Path: path, Banner: opts.versionOutput(path), Origin: "--engine"}, nil
}

// engineSearch accumulates candidates while remembering what it passed over.
type engineSearch struct {
	seen    map[string]bool
	found   []engineCandidate
	skipped []string
}

// resolved records a path PATH handed us. exec.LookPath has already established
// that its hit exists and is executable, so re-checking it would only disagree
// with the resolver that found it.
func (s *engineSearch) resolved(path, origin string) {
	s.consider(path, origin, "")
}

// guessed records a path ssg constructed itself, which has to be checked.
func (s *engineSearch) guessed(path, origin string) {
	s.consider(path, origin, unrunnableReason(path))
}

func (s *engineSearch) consider(path, origin, reason string) {
	if path == "" || s.seen[path] {
		return
	}
	s.seen[path] = true
	if snapWrapperBlocks(path) {
		reason = snapWrapperReason
	}
	switch reason {
	case "":
		s.found = append(s.found, engineCandidate{Path: path, Origin: origin})
	case reasonNotFound:
		// A location ssg merely guessed at and did not find is the ordinary
		// case, not news. Only an engine that exists and still cannot be run is
		// worth the operator's attention.
	default:
		s.skipped = append(s.skipped, fmt.Sprintf("%s (%s)", path, reason))
	}
}

// gatherEngineCandidates collects every runnable wpexporter, nearest first, and
// the reasons anything was passed over.
func gatherEngineCandidates(opts Options) (found []engineCandidate, skipped []string) {
	s := &engineSearch{seen: map[string]bool{}}

	if bin, err := opts.lookPath(wpexporterBinary); err == nil {
		s.resolved(bin, pathOrigin(bin))
	}
	// A snap's PATH holds only the snap's own bin, so the host's copies have to
	// be looked for by name — that is the whole point of the search (#160).
	snap := confinedSnapRoot()
	if snap == "" {
		return s.found, s.skipped
	}
	s.guessed(filepath.Join(snap, "bin", wpexporterBinary), "bundled with the snap")
	home, err := userHomeDir()
	if err != nil {
		return s.found, s.skipped
	}
	for _, dir := range hostEngineDirs {
		s.guessed(filepath.Join(home, dir, wpexporterBinary), "$HOME")
	}
	return s.found, s.skipped
}

// pathOrigin names where a PATH hit came from, so a bundled engine found
// through PATH is still reported as bundled.
func pathOrigin(bin string) string {
	if snap := os.Getenv("SNAP"); snap != "" && strings.HasPrefix(bin, snap) {
		return "bundled with the snap"
	}
	return "PATH"
}

// newestEngine runs the highest version among the candidates. Ties keep the
// first — candidates arrive nearest-first, so an explicit PATH entry outranks a
// guessed $HOME location at equal versions.
func newestEngine(candidates []engineCandidate, skipped []string, opts Options) engineChoice {
	best := engineChoice{Skipped: skipped}
	var bestVer [3]int
	for _, c := range candidates {
		banner := opts.versionOutput(c.Path)
		major, minor, patch := parseSemver(banner)
		ver := [3]int{major, minor, patch}
		if best.Path == "" || compareVersions(ver, bestVer) > 0 {
			best.Path, best.Banner, best.Origin, bestVer = c.Path, banner, c.Origin, ver
		}
	}
	return best
}

// compareVersions orders two version triples: -1, 0 or 1.
func compareVersions(a, b [3]int) int {
	for i := range a {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		}
	}
	return 0
}

// snapWrapperReason explains a /snap/bin entry. It is a snapd wrapper, not a
// binary: from inside a snap it fails with "cannot join mount namespace", and
// naming that up front is the difference between an answer and an afternoon.
// #nosec G101 -- an English sentence shown to the operator, not a credential
const snapWrapperReason = "it is the wpexporter snap, and a snap cannot execute another snap"

// reasonNotFound marks a candidate that simply is not there.
const reasonNotFound = "not found"

// snapWrapperBlocks reports whether path is a snap ssg cannot execute.
//
// Only a confined ssg is stopped by one: from an ordinary install,
// /snap/bin/wpexporter runs perfectly well — it is how most people have the
// engine. Rejecting it everywhere would break the commonest setup there is.
func snapWrapperBlocks(path string) bool {
	return isSnapWrapper(path) && confinedSnapRoot() != ""
}

// unrunnableReason explains why a path cannot be executed, or "" when it can.
func unrunnableReason(path string) string {
	if snapWrapperBlocks(path) {
		return snapWrapperReason
	}
	info, err := os.Stat(path)
	if err != nil {
		return reasonNotFound
	}
	if info.IsDir() {
		return "not a file"
	}
	if info.Mode().Perm()&0111 == 0 {
		return "not executable"
	}
	return ""
}

// isSnapWrapper reports whether path is snapd's launcher rather than a binary.
func isSnapWrapper(path string) bool {
	if filepath.Dir(path) == "/snap/bin" {
		return true
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return filepath.Base(target) == "snap"
}

// engineSelectionNotes renders what the search passed over and, when the engine
// that ran is not the bundled one, why. It answers "I upgraded and nothing
// changed" before the question is asked (#160).
func engineSelectionNotes(choice engineChoice) []string {
	var notes []string
	for _, s := range choice.Skipped {
		notes = append(notes, "skipped "+s)
	}
	return notes
}
