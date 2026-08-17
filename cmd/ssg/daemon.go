package main

// `ssg daemon` — several projects watched by one process (#169).
//
// The reload is the reason this exists rather than four terminals: editing
// .ssg_projects reconciles the fleet in place. A project whose settings did not
// change keeps running and keeps its port; only what actually changed is
// restarted. SIGHUP reloads on demand, SIGTERM and SIGINT stop everything the
// way each project would stop on its own.

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spagu/ssg/internal/daemon"
)

// daemonPollInterval is how often the projects file is checked for edits. A
// second is imperceptible to a person saving a file and free at this scale —
// one stat of one file.
const daemonPollInterval = time.Second

// daemonSignals is a seam: tests drive reload and shutdown without raising real
// signals at the test runner.
var daemonSignals = func() (reload, quit <-chan os.Signal) {
	r := make(chan os.Signal, 1)
	q := make(chan os.Signal, 1)
	signal.Notify(r, syscall.SIGHUP)
	signal.Notify(q, syscall.SIGINT, syscall.SIGTERM)
	return r, q
}

type daemonFlags struct {
	config string
	quiet  bool
	// once builds the fleet's set of projects, reports it and exits — enough to
	// check a projects file in CI without leaving anything running.
	once bool
}

// runDaemon supervises every project in the projects file until told to stop.
func runDaemon(args []string) int {
	flags, code := parseDaemonFlags(args)
	if code >= 0 {
		return code
	}

	cfg, err := daemon.Load(flags.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return 1
	}
	if len(cfg.Active()) == 0 {
		fmt.Fprintf(os.Stderr, "❌ %s lists no project to run\n", flags.config)
		return 1
	}
	if flags.once {
		reportDaemonProjects(cfg)
		return 0
	}
	return superviseProjects(cfg, flags)
}

// superviseProjects runs the fleet until a signal ends it.
func superviseProjects(cfg *daemon.Config, flags daemonFlags) int {
	out := os.Stdout
	sup := daemon.NewSupervisor(daemon.ExecRunner{Out: out}, out, 5*time.Second)
	defer sup.Shutdown()

	if !flags.quiet {
		fmt.Printf("🛠️  Watching %d project(s) from %s\n", len(cfg.Active()), flags.config)
	}
	if err := sup.Apply(cfg.Active()); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
	}

	reload, quit := daemonSignals()
	watcher := newProjectsWatcher(flags.config)
	ticker := time.NewTicker(daemonPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			if !flags.quiet {
				fmt.Println("\n🛑 Stopping every project...")
			}
			return 0
		case <-reload:
			reloadProjects(sup, flags, "SIGHUP")
		case <-ticker.C:
			if watcher.changed() {
				reloadProjects(sup, flags, flags.config+" changed")
			}
			// A project that died on its own comes back: a build that gave up
			// on a bad config must not silently leave a site unserved.
			if name := sup.Exited(); name != "" {
				if err := sup.Restart(name); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
				}
			}
		}
	}
}

// reloadProjects re-reads the projects file and reconciles the fleet. A file
// that no longer parses leaves everything running: the projects on disk are
// still serving, and stopping them over a typo would be the worse failure.
func reloadProjects(sup *daemon.Supervisor, flags daemonFlags, why string) {
	next, err := daemon.Load(flags.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n   Keeping the running projects — fix the file and save to retry.\n", why, err)
		return
	}
	if !flags.quiet {
		fmt.Printf("♻️  Reloading (%s)\n", why)
	}
	if err := sup.Apply(next.Active()); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
	}
	if !flags.quiet {
		fmt.Printf("   ✅ %d project(s) running: %v\n", len(sup.Names()), sup.Names())
	}
}

// reportDaemonProjects prints what would run, for --once.
func reportDaemonProjects(cfg *daemon.Config) {
	for _, p := range cfg.Active() {
		fmt.Printf("   %s → %s  ssg %v\n", p.Name, p.Dir, p.Command())
	}
	if disabled := len(cfg.Projects) - len(cfg.Active()); disabled > 0 {
		fmt.Printf("   (%d disabled)\n", disabled)
	}
}

// projectsWatcher notices edits to the projects file by size and mtime, the
// same way the build's own watch loop notices content.
type projectsWatcher struct {
	path string
	sig  string
}

func newProjectsWatcher(path string) *projectsWatcher {
	w := &projectsWatcher{path: path}
	w.sig = w.signature()
	return w
}

func (w *projectsWatcher) signature() string {
	info, err := os.Stat(w.path)
	if err != nil {
		return "" // a file being rewritten is momentarily absent; not a change
	}
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
}

func (w *projectsWatcher) changed() bool {
	sig := w.signature()
	if sig == "" || sig == w.sig {
		return false
	}
	w.sig = sig
	return true
}

func parseDaemonFlags(args []string) (daemonFlags, int) {
	f := daemonFlags{config: daemon.DefaultConfigFile}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--quiet" || arg == "-q":
			f.quiet = true
		case arg == "--once" || arg == "--check":
			f.once = true
		case arg == "--help" || arg == "-h":
			printDaemonUsage()
			return f, 0
		case len(arg) > 9 && arg[:9] == "--config=":
			f.config = arg[9:]
		case arg == "--config" && i+1 < len(args):
			f.config = args[i+1]
			i++
		default:
			fmt.Fprintf(os.Stderr, "❌ unknown flag %q\n\n", arg)
			printDaemonUsage()
			return f, 2
		}
	}
	if abs, err := filepath.Abs(f.config); err == nil {
		f.config = abs
	}
	return f, -1
}

func printDaemonUsage() {
	fmt.Print(`ssg daemon — watch several projects from one process

Usage:
   ssg daemon [--config FILE] [--once] [--quiet]

   --config FILE   projects file (default: ` + daemon.DefaultConfigFile + `)
   --once          print what would run and exit — a projects file check for CI
   --quiet         suppress the daemon's own output; projects still log

A projects file:

   projects:
     - name: blog
       dir: /srv/blog
       http: true
       port: 8801
     - name: shop
       dir: /srv/shop
       port: 8802          # a port implies http
     - name: docs
       dir: ../docs-site
       config: .ssg.prod.yaml
       args: ["--minify-all"]
     - name: staging
       dir: /srv/staging
       disabled: true

Each project runs an ordinary ssg --watch in its own directory. Editing the
projects file reloads the fleet in place: a project whose settings did not
change keeps running and keeps its port, and only what changed is restarted.
SIGHUP reloads on demand; SIGINT and SIGTERM stop everything.
`)
}
