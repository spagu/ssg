package main

// `ssg self-update-check` — is there a newer release, and how would you get it
// (#193).
//
// It reports and stops there. ssg does not update itself: rewriting your own
// binary needs more trust than a static site generator has any business asking
// for, and an upgrade that happens without being asked for is an upgrade
// nobody chose the timing of.
//
// It also runs only when invoked. A generator that contacts a server on every
// build is slow, breaks in air-gapped and CI environments, and reports on
// something nobody asked about.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// releasesRepo is the repository whose releases are checked.
const releasesRepo = "spagu/ssg"

// updateCheckTimeout bounds the request. A version check is a convenience, so
// it must never be the reason a terminal hangs.
const updateCheckTimeout = 10 * time.Second

// latestRelease is the part of GitHub's release payload this needs.
type latestRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// runSelfUpdateCheck compares the running version against the newest release.
func runSelfUpdateCheck(args []string) int {
	if len(args) > 0 {
		errf("❌ unknown argument %q\n\nusage: ssg self-update-check\n", args[0])
		return 1
	}
	origin := detectInstallOrigin(executableOf())
	fmt.Printf("🧱 ssg %s (installed via %s)\n", Version, origin.Name)

	release, err := fetchLatestRelease()
	if err != nil {
		errf("⚠️  could not reach %s: %v\n   Releases: https://github.com/%s/releases\n",
			githubAPIBase, err, releasesRepo)
		return 1
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	return reportVersionComparison(Version, latest, release.HTMLURL, origin)
}

// reportVersionComparison prints the verdict and returns the exit code.
func reportVersionComparison(current, latest, url string, origin installOrigin) int {
	switch compareVersions(current, latest) {
	case 0:
		fmt.Printf("✅ %s is the current release — nothing to do.\n", latest)
	case 1:
		// Ahead of the newest release: a development build, not a problem.
		fmt.Printf("✅ %s is ahead of the newest release (%s) — nothing to do.\n", current, latest)
	default:
		fmt.Printf("🆕 A newer release is out: %s (you have %s)\n", latest, current)
		if url != "" {
			fmt.Printf("   %s\n", url)
		}
		if origin.Upgrade != "" {
			fmt.Printf("\n   Upgrade with:\n     %s\n", origin.Upgrade)
		} else {
			fmt.Printf("\n   Download it from:\n     https://github.com/%s/releases/latest\n", releasesRepo)
		}
	}
	return 0
}

// fetchLatestRelease asks GitHub for the newest published release.
func fetchLatestRelease() (latestRelease, error) {
	var out latestRelease
	req, err := http.NewRequest(http.MethodGet, githubAPIBase+"/repos/"+releasesRepo+"/releases/latest", nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// Unauthenticated: a version check must work for someone who has never
	// configured a token, and this endpoint needs none.
	resp, err := (&http.Client{Timeout: updateCheckTimeout}).Do(req)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("GitHub answered %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return out, fmt.Errorf("decoding the release: %w", err)
	}
	if out.TagName == "" {
		return out, fmt.Errorf("the newest release has no tag")
	}
	return out, nil
}

// compareVersions returns -1 when a is older than b, 1 when newer, 0 when they
// are the same release.
//
// A non-numeric version — "dev", a commit hash from a source build — compares
// as older than any release, so a development build is told what the newest
// release is rather than being declared current.
func compareVersions(a, b string) int {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	switch {
	case !aok && !bok:
		return 0
	case !aok:
		return -1
	case !bok:
		return 1
	}
	for i := range av {
		if av[i] != bv[i] {
			if av[i] < bv[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseVersion reads a dotted numeric version into three parts, ignoring a
// leading "v" and anything after a "-" or "+" so a pre-release or build suffix
// compares by its release numbers.
func parseVersion(s string) ([3]int, bool) {
	var out [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 || s == "" {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
