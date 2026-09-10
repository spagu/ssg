package main

// `ssg migrate` past the happy path: the flags that throttle an engine and
// pass it arguments verbatim, the identity reload that must not clobber a
// build, and the live server that has to keep serving a failed one.

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/migrate"
)

// dcovFreePort returns a port nothing is listening on right now. The server
// walks forward from what it is given, so callers assert "at least this", never
// "exactly this" — which is what keeps the assertion free of a bind race.
func dcovFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no local TCP listener available: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// dcovAnnouncedPort reads the port out of the first http://127.0.0.1:NNNN in
// out, or -1 when the loopback address was never announced.
func dcovAnnouncedPort(out string) int {
	const marker = "http://127.0.0.1:"
	i := strings.Index(out, marker)
	if i < 0 {
		return -1
	}
	rest := out[i+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	port, err := strconv.Atoi(rest[:end])
	if err != nil {
		return -1
	}
	return port
}

// TestParseMigrateFlagsCarriesTheEngineThrottleAndItsVerbatimArguments (#171):
// bot protection in front of WordPress is ordinary and the engine's own
// diagnosis names a slower crawl and a different user agent. Those were
// unusable from `ssg migrate` until these flags existed, and --engine-arg is
// the general escape hatch — it must accumulate rather than replace, or a flag
// and its value could never be passed together.
func TestParseMigrateFlagsCarriesTheEngineThrottleAndItsVerbatimArguments(t *testing.T) {
	f, code := parseMigrateFlags([]string{
		"--rate-limit", "250", "--engine-arg", "--some-flag", "--engine-arg", "value",
		"--user-agent", "ssg/test"})
	if code >= 0 {
		t.Fatalf("engine flags must be accepted, got exit %d", code)
	}
	if f.rateLimit != 250 || f.userAgent != "ssg/test" {
		t.Errorf("parsed = %d ms, agent %q", f.rateLimit, f.userAgent)
	}
	if len(f.engineArgs) != 2 || f.engineArgs[0] != "--some-flag" || f.engineArgs[1] != "value" {
		t.Errorf("--engine-arg must accumulate in order, got %v", f.engineArgs)
	}
	// Zero is a legitimate "no throttle", not a missing value.
	if f, code := parseMigrateFlags([]string{"--rate-limit=0"}); code >= 0 || f.rateLimit != 0 {
		t.Errorf("--rate-limit=0: %d ms, code %d", f.rateLimit, code)
	}

	// A typo must stop the run: a crawl that silently ran at full speed against
	// a site that blocked it for exactly that is the failure this prevents.
	for _, bad := range [][]string{{"--rate-limit", "slow"}, {"--rate-limit=-5"}} {
		var diag string
		_, _ = captureStdout(func() error {
			diag = captureStderr(t, func() {
				if _, code := parseMigrateFlags(bad); code != 2 {
					t.Errorf("%v = %d, want 2", bad, code)
				}
			})
			return nil
		})
		if !strings.Contains(diag, "--rate-limit") {
			t.Errorf("%v must be explained: %q", bad, diag)
		}
	}
}

// TestReloadAfterIdentityLeavesTheBuildAloneWhenTheConfigCannotBeReread: the
// export completes the config and the build must see it in the same run. But a
// config that no longer loads — or was never written, because the project had
// none — must not blank the title the build already had.
func TestReloadAfterIdentityLeavesTheBuildAloneWhenTheConfigCannotBeReread(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{Title: "kept", Description: "kept too"}
	genCfg := generator.Config{Title: "kept", Description: "kept too"}

	gotGen, gotCfg := reloadAfterIdentity(cfg, genCfg)
	if gotGen.Title != "kept" || gotCfg.Title != "kept" {
		t.Errorf("no config file must leave the build untouched: %q / %q", gotGen.Title, gotCfg.Title)
	}

	if err := os.WriteFile(".ssg.yaml", []byte("title: [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gotGen, gotCfg = reloadAfterIdentity(cfg, genCfg)
	if gotGen.Title != "kept" || gotCfg.Title != "kept" {
		t.Errorf("an unparseable config must leave the build untouched: %q / %q", gotGen.Title, gotCfg.Title)
	}

	// And when it does load, the freshly written identity wins — the reason the
	// reread exists at all.
	if err := os.WriteFile(".ssg.yaml",
		[]byte("source: s\ntemplate: simple\ndomain: x.example.com\ntitle: From The Source Site\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gotGen, gotCfg = reloadAfterIdentity(cfg, genCfg)
	if gotGen.Title != "From The Source Site" || gotCfg.Title != "From The Source Site" {
		t.Errorf("a readable config must reach the build: %q / %q", gotGen.Title, gotCfg.Title)
	}
}

// TestMigrateLiveServesWhereTheAddressFlagsSay (#135): live migration is a
// server, and the address it announces has to be the one it took — including
// the --host and --port the operator asked for. With --watch and --http both on,
// auto-reload must be armed too, or the browser never shows the site filling up,
// which is the whole point of the live flow (GO-090/GO-100).
func TestMigrateLiveServesWhereTheAddressFlagsSay(t *testing.T) {
	t.Chdir(t.TempDir())
	port := dcovFreePort(t)
	blocked := stubMigrate(t, func(_ migrate.Options) (*migrate.Report, error) {
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})
	setReloadHub(nil)
	t.Cleanup(func() { setReloadHub(nil) })

	out, err := captureStdout(func() error {
		if code := runMigrate([]string{"wordpress", "https://live.example.com",
			"--watch", "--http", "--host", "127.0.0.1", "--port", strconv.Itoa(port)}); code != 0 {
			t.Errorf("live migrate = %d, want 0", code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if *blocked != 1 {
		t.Errorf("live mode must park so the server keeps serving, parked %d times", *blocked)
	}
	got := dcovAnnouncedPort(out)
	if got < port || got > port+portSearchSpan {
		t.Errorf("--host/--port must decide the announced address (asked %d, announced %d):\n%s", port, got, out)
	}
	if reloadHub.Load() == nil {
		t.Error("--watch --http must arm auto-reload, or the browser never refreshes")
	}
}

// TestMigrateLiveReportsABuildItCouldNotFinishAndKeepsServing: --http without
// --watch has nothing rebuilding on its own, so the final build is the only one
// that matters. When it fails the operator must be told — and the server must
// still stay up, so the partial site is browsable instead of the terminal dying.
func TestMigrateLiveReportsABuildItCouldNotFinishAndKeepsServing(t *testing.T) {
	t.Chdir(t.TempDir())
	// A regular file where the output directory belongs: nothing can be written
	// under it, so the build fails for a reason the operator could really hit.
	if err := os.WriteFile("output", []byte("in the way\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	port := dcovFreePort(t)
	blocked := stubMigrate(t, func(_ migrate.Options) (*migrate.Report, error) {
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})

	var diag string
	if _, err := captureStdout(func() error {
		diag = captureStderr(t, func() {
			if code := runMigrate([]string{"wordpress", "https://live.example.com",
				"--http", "--port", strconv.Itoa(port)}); code != 0 {
				t.Errorf("live migrate = %d, want 0", code)
			}
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diag, "building migrated site") {
		t.Errorf("a failed final build must be reported: %q", diag)
	}
	if *blocked != 1 {
		t.Errorf("the server must keep serving after a failed build, parked %d times", *blocked)
	}
}

// TestPrintMigrateReportNamesTheEngineAndTheMenusItBroughtBack: the snap ships
// its own wpexporter, so refreshing the host's copy changes nothing and nothing
// said so (#140) — the report must name the binary that actually ran. Menus are
// an editorial arrangement the content cannot rebuild, so their arrival is worth
// stating too (#132).
func TestPrintMigrateReportNamesTheEngineAndTheMenusItBroughtBack(t *testing.T) {
	out, err := captureStdout(func() error {
		printMigrateReport(&migrate.Report{
			Provider: "wordpress@1.2.0", Pages: 1, Posts: 2, Media: 3, Menus: 4,
			Engine: "/snap/ssg/current/bin/wpexporter",
		}, "https://example.com")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/snap/ssg/current/bin/wpexporter") {
		t.Errorf("the engine that ran must be named:\n%s", out)
	}
	if !strings.Contains(out, "4 navigation menu(s)") {
		t.Errorf("menus that arrived must be reported:\n%s", out)
	}
	if strings.Contains(out, "reader comments") {
		t.Errorf("a site with no comments must not be told about zero of them:\n%s", out)
	}
}
