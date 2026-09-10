package main

// The build's own accounting, on the command line (GO-097).
//
// Two surfaces over one measurement: `--profile` reports the build that just
// ran, and `ssg profile page /foo/` answers the follow-up question about one
// page from the report the last build left behind.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// startPprof begins CPU profiling when --profile-pprof names a directory, and
// returns the function that stops it and writes the heap profile. It always
// returns a callable, so the caller defers it without checking.
//
// A missing directory, or a file that will not open, is a warning rather than
// a failed build: this is a maintainer's instrument, and the site it is
// pointed at still has to build.
func startPprof(cfg *config.Config) func() {
	dir := cfg.ProfilePprof
	if dir == "" {
		return func() {}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		errf("⚠️  --profile-pprof: %v\n", err)
		return func() {}
	}
	cpu, err := os.Create(filepath.Join(dir, "cpu.prof")) // #nosec G304 -- a directory the operator named
	if err != nil {
		errf("⚠️  --profile-pprof: %v\n", err)
		return func() {}
	}
	if err := pprof.StartCPUProfile(cpu); err != nil {
		errf("⚠️  --profile-pprof: %v\n", err)
		_ = cpu.Close()
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		_ = cpu.Close()
		writeHeapProfile(dir)
		if !cfg.Quiet {
			fmt.Printf("   🔬 pprof: %s/cpu.prof, %s/heap.prof\n", dir, dir)
		}
	}
}

// writeHeapProfile snapshots the heap after the build, when the numbers mean
// something. A GC first, so what is reported is live memory rather than
// whatever had not been collected yet.
func writeHeapProfile(dir string) {
	f, err := os.Create(filepath.Join(dir, "heap.prof")) // #nosec G304 -- a directory the operator named
	if err != nil {
		errf("⚠️  --profile-pprof heap: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		errf("⚠️  --profile-pprof heap: %v\n", err)
	}
}

// emitProfile finishes the measurement and reports it: the table on stdout
// always, and the JSON file when that is the mode asked for.
func emitProfile(prof *generator.Profile, cfg *config.Config) {
	if prof == nil {
		return
	}
	prof.Finish()
	now := time.Now()
	// The report prints even under --quiet: quiet suppresses build chatter,
	// and this is the thing the flag was passed to produce.
	prof.WriteText(os.Stdout, now)
	if cfg.Profile != generator.ProfileJSON {
		return
	}
	path, err := prof.WriteJSON(".", Version, now)
	if err != nil {
		errf("⚠️  --profile=json: %v\n", err)
		return
	}
	if !cfg.Quiet {
		fmt.Printf("   📄 %s\n", path)
	}
}

// isProfileSubcommand keeps the verb+noun dispatch rule: `ssg profile page` is
// a subcommand, while a source directory literally named "profile" still
// builds.
func isProfileSubcommand(noun string) bool { return noun == "page" }

// runProfilePage implements `ssg profile page /url/`: what that page cost in
// the last profiled build.
func runProfilePage(args []string) int {
	if len(args) == 0 {
		errf("❌ usage: ssg profile page /url/\n")
		return 2
	}
	report, err := generator.LoadProfileReport(".")
	if err != nil {
		if os.IsNotExist(err) {
			errf("❌ no %s here — run a build with --profile=json first.\n", generator.ProfileFileName)
			return 1
		}
		errf("❌ %v\n", err)
		return 1
	}
	want := normalizeProfileURL(args[0])
	for _, p := range report.Pages {
		if normalizeProfileURL(p.Path) == want {
			printProfilePage(report, p)
			return 0
		}
	}
	errf("❌ %s is not in %s (%d pages measured).\n", want, generator.ProfileFileName, len(report.Pages))
	errf("   Only pages rendered through a template are measured; assets and copied files are not.\n")
	return 1
}

// printProfilePage reports one page's cost against the build it belongs to.
func printProfilePage(report generator.ProfileReport, p generator.PageTiming) {
	fmt.Println(p.Path)
	fmt.Printf("   render          %8.1f ms\n", p.Millis)
	if report.TotalMs > 0 {
		fmt.Printf("   share           %8.1f%% of a %.2f s build\n", p.Millis/report.TotalMs*100, report.TotalMs/1000)
	}
	fmt.Printf("   build           %s", report.Time.Local().Format("2006-01-02 15:04:05"))
	if report.Version != "" {
		fmt.Printf(" · ssg %s", report.Version)
	}
	fmt.Println()
	// Honest about the half that does not exist yet: the cost is measured, the
	// reason for it is not, and inventing a tree here would be worse than
	// saying so.
	fmt.Println("   dependency tree: requires the incremental build graph (GO-094)")
}

// normalizeProfileURL compares URLs the way a person types them against the
// way the report writes them: leading slash, trailing slash, no index.html.
func normalizeProfileURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "index.html")
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	if !strings.HasSuffix(u, "/") && filepath.Ext(u) == "" {
		u += "/"
	}
	return u
}
