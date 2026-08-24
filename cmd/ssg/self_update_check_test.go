package main

// `ssg self-update-check` and the startup version line (#193).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// withVersion pins the reported version for one test.
func withVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

// withGitHub points the release lookup at a stand-in server.
func withGitHub(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() {
		githubAPIBase = old
		srv.Close()
	})
}

// releaseFeed answers the releases endpoint with one tag.
func releaseFeed(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"` + tag +
			`","html_url":"https://github.com/spagu/ssg/releases/tag/` + tag + `"}`))
	}
}

// runCheck runs the command and returns what it wrote to stdout.
func runCheck(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var code int
	out, err := captureStdout(func() error {
		code = runSelfUpdateCheck(args)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, code
}

// TestANewerReleaseIsReportedWithTheCommandToGetIt — the whole point. A version
// number the reader then has to look up an upgrade command for is half an answer.
func TestANewerReleaseIsReportedWithTheCommandToGetIt(t *testing.T) {
	withVersion(t, "1.8.30")
	withGitHub(t, releaseFeed("v1.8.47"))
	t.Setenv("SNAP", "")

	out, code := runCheck(t)
	if code != 0 {
		t.Errorf("exit = %d, want 0 — being out of date is not an error", code)
	}
	for _, want := range []string{"1.8.47", "you have 1.8.30", "releases/tag/v1.8.47"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestBeingCurrentSaysSoAndOffersNothing: a check that always suggests an
// upgrade command teaches the reader to ignore it.
func TestBeingCurrentSaysSoAndOffersNothing(t *testing.T) {
	withVersion(t, "1.8.47")
	withGitHub(t, releaseFeed("v1.8.47"))

	out, code := runCheck(t)
	if code != 0 {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(out, "nothing to do") {
		t.Errorf("output = %q", out)
	}
	if strings.Contains(out, "Upgrade with") || strings.Contains(out, "newer release") {
		t.Errorf("a current install must not be told to upgrade:\n%s", out)
	}
}

// TestADevelopmentBuildAheadOfTheReleaseIsNotToldToDowngrade. Building from
// main between releases is normal here; "a newer release is out: 1.8.47" when
// you are on 1.8.48 would be wrong.
func TestADevelopmentBuildAheadOfTheReleaseIsNotToldToDowngrade(t *testing.T) {
	withVersion(t, "1.8.48")
	withGitHub(t, releaseFeed("v1.8.47"))

	out, _ := runCheck(t)
	if !strings.Contains(out, "ahead of the newest release") {
		t.Errorf("output = %q", out)
	}
}

// TestAnUnparseableVersionIsTreatedAsOlder: `go build` with no ldflags reports
// "dev", and telling that build it is current would be a lie.
func TestAnUnparseableVersionIsTreatedAsOlder(t *testing.T) {
	withVersion(t, "dev")
	withGitHub(t, releaseFeed("v1.8.47"))

	out, _ := runCheck(t)
	if !strings.Contains(out, "newer release is out: 1.8.47") {
		t.Errorf("output = %q", out)
	}
}

// TestTheUpgradeCommandMatchesTheInstall — a snap told to `brew upgrade` is
// worse than being told nothing.
func TestTheUpgradeCommandMatchesTheInstall(t *testing.T) {
	withVersion(t, "1.8.30")
	withGitHub(t, releaseFeed("v1.8.47"))
	t.Setenv("SNAP", "/snap/static-site-generator/42")

	// detectInstallOrigin reads the real executable, which under `go test` is
	// the test binary — so assert the mapping directly as well as end to end.
	snap := detectInstallOrigin("/snap/static-site-generator/42/bin/ssg")
	if snap.Name != "snap" || !strings.Contains(snap.Upgrade, "snap refresh static-site-generator") {
		t.Errorf("snap origin = %+v", snap)
	}
	out, _ := runCheck(t)
	if !strings.Contains(out, "installed via") {
		t.Errorf("the origin must be named:\n%s", out)
	}
}

// TestAnUnreachableServerFailsLoudlyAndPointsAtTheReleases: silence would read
// as "you are current".
func TestAnUnreachableServerFailsLoudlyAndPointsAtTheReleases(t *testing.T) {
	withVersion(t, "1.8.47")
	old := githubAPIBase
	githubAPIBase = "http://127.0.0.1:1"
	t.Cleanup(func() { githubAPIBase = old })

	var code int
	stderr := captureStderr(t, func() {
		_, _ = captureStdout(func() error {
			code = runSelfUpdateCheck(nil)
			return nil
		})
	})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "could not reach") || !strings.Contains(stderr, "releases") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestABadAnswerIsAnError, rather than being read as a version.
func TestABadAnswerIsAnError(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"not json": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{{{`))
		},
		"no tag": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"html_url":"x"}`))
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			withVersion(t, "1.8.47")
			withGitHub(t, h)
			var code int
			captureStderr(t, func() {
				_, _ = captureStdout(func() error {
					code = runSelfUpdateCheck(nil)
					return nil
				})
			})
			if code != 1 {
				t.Errorf("exit = %d, want 1", code)
			}
		})
	}
}

// TestArgumentsAreRefusedWithUsage — there are none, and silently ignoring one
// hides a typo.
func TestArgumentsAreRefusedWithUsage(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() { code = runSelfUpdateCheck([]string{"--now"}) })
	if code != 1 {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(stderr, "usage: ssg self-update-check") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestTheCheckIsDispatched: a subcommand nobody can reach is not a feature.
func TestTheCheckIsDispatched(t *testing.T) {
	if _, handled := dispatchSingleVerb([]string{"self-update-check", "--now"}); !handled {
		t.Error("`ssg self-update-check` must dispatch")
	}
}

// TestVersionComparison covers the ordering the report turns on.
func TestVersionComparison(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.8.47", "1.8.47", 0},
		{"v1.8.47", "1.8.47", 0},
		{"1.8.46", "1.8.47", -1},
		{"1.8.47", "1.8.46", 1},
		{"1.9.0", "1.10.0", -1}, // not string order
		{"1.8.9", "1.8.10", -1}, // nor is this
		{"2.0.0", "1.99.99", 1},
		{"1.8", "1.8.0", 0}, // a short version is not a different one
		{"1.8.47-rc1", "1.8.47", 0},
		{"1.8.47+build9", "1.8.47", 0},
		{"dev", "1.8.47", -1},
		{"1.8.47", "dev", 1},
		{"dev", "dev", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestParseVersionRejectsWhatItCannotOrder rather than guessing a number.
func TestParseVersionRejectsWhatItCannotOrder(t *testing.T) {
	for _, bad := range []string{"", "v", "1.8.47.3", "1.x.4", "-1.0.0", "  "} {
		if _, ok := parseVersion(bad); ok {
			t.Errorf("parseVersion(%q) must not parse", bad)
		}
	}
	if v, ok := parseVersion("v1.8.47"); !ok || v != [3]int{1, 8, 47} {
		t.Errorf("parseVersion = %v, %v", v, ok)
	}
}

// TestTheStartupLineNamesTheVersionAndObeysQuiet.
func TestTheStartupLineNamesTheVersionAndObeysQuiet(t *testing.T) {
	withVersion(t, "1.8.47")

	loud, err := captureStdout(func() error {
		logRunningVersion(&config.Config{})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loud, "ssg 1.8.47") {
		t.Errorf("startup line = %q", loud)
	}

	quiet, err := captureStdout(func() error {
		logRunningVersion(&config.Config{Quiet: true})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if quiet != "" {
		t.Errorf("--quiet must print nothing, got %q", quiet)
	}
}

// TestTheUpgradeCommandIsPrintedWhenThereIsOne — the branch that carries the
// whole value of the feature.
func TestTheUpgradeCommandIsPrintedWhenThereIsOne(t *testing.T) {
	out, err := captureStdout(func() error {
		reportVersionComparison("1.8.30", "1.8.47",
			"https://github.com/spagu/ssg/releases/tag/v1.8.47",
			installOrigin{Name: "snap", Upgrade: "sudo snap refresh static-site-generator"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Upgrade with:") ||
		!strings.Contains(out, "sudo snap refresh static-site-generator") {
		t.Errorf("output = %q", out)
	}
	if strings.Contains(out, "Download it from") {
		t.Errorf("an install with an upgrade command must not also be sent to the releases page:\n%s", out)
	}
}

// TestAMalformedEndpointIsAnErrorRatherThanAPanic.
func TestAMalformedEndpointIsAnErrorRatherThanAPanic(t *testing.T) {
	old := githubAPIBase
	githubAPIBase = "http://bad\x7fhost"
	t.Cleanup(func() { githubAPIBase = old })

	if _, err := fetchLatestRelease(); err == nil {
		t.Error("an unbuildable request must be an error")
	}
}
