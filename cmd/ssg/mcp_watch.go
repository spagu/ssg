package main

// One process for preview, MCP and the filesystem (#184).
//
// `ssg mcp --http --listen=…` was very nearly the whole development server: it
// serves the MCP endpoint, serves the preview, and rebuilds with live reload
// after every MCP mutation. What it did not do was notice a file changed by
// anything else — a human editor open beside the agent, an rsync, a CMS export
// refreshing content. `--watch` was parsed into the config by parseFlags and
// then read by nobody in this path, so the flag was accepted and swallowed.
//
// Two things are fixed here, and only one of them was reported.
//
//  1. Every rebuild goes through one mutex. The reported worry was a filesystem
//     watcher racing MCP mutations, but the race predates any watcher: the
//     Streamable HTTP transport runs each request on its own goroutine, so two
//     concurrent tools/call already rebuilt the same output tree at the same
//     time — and captureStdout swaps the process-wide os.Stdout while it does
//     it. A rebuild racing a rebuild over one output directory is a preview
//     that intermittently serves half a site.
//
//  2. With `--watch`, the mcp path runs the same polling loop the serve path
//     runs — the same watchIteration, the same fileSigCache, the same
//     content-signature gate that skips touch-only events — and calls the same
//     serialised rebuild. Outside edits now reload the preview exactly as MCP
//     edits do.
//
// The alternative was `--watch-runner="ssg mcp --listen=…"`: one command line,
// still two processes, two independent builders over one output tree, and
// nothing serialising them. That is the arrangement this replaces.

import (
	"strings"
	"sync"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// mcpRebuilder owns the site build for the whole `ssg mcp` process: the MCP
// tools' afterMutate hook and the filesystem watcher both call it, and the
// mutex is what makes "both" safe.
//
// The configuration is held here rather than captured by a closure so a config
// reload during the watch loop changes what a subsequent rebuild builds from.
type mcpRebuilder struct {
	mu     sync.Mutex
	genCfg generator.Config
	cfg    *config.Config
	// buildFn is the build itself. A field rather than a direct call to build
	// so that the serialisation this type exists for can be asserted on — two
	// real site builds racing over one temp directory would be the bug under
	// test rather than an observation of it.
	buildFn func(generator.Config, *config.Config) error
}

// newMCPRebuilder wires a rebuilder to the configuration the process started on.
func newMCPRebuilder(genCfg generator.Config, cfg *config.Config) *mcpRebuilder {
	return &mcpRebuilder{genCfg: genCfg, cfg: cfg, buildFn: build}
}

// rebuild runs one build and pushes the result to an open `--http` preview: a
// reload on success, the error overlay otherwise (GO-090). A no-op push without
// --http.
//
// Output is captured rather than printed. stdout belongs to the JSON-RPC
// channel, so build noise reaching it would corrupt the protocol; the captured
// text is returned so the model reads what the build said. The capture swaps
// os.Stdout process-wide, which is only safe because this whole method is
// serialised.
func (r *mcpRebuilder) rebuild() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	genCfg, cfg := r.genCfg, r.cfg
	quiet := *cfg
	quiet.Quiet = true
	out, err := captureStdout(func() error { return r.buildFn(genCfg, &quiet) })
	if err != nil {
		notifyBuildError(err.Error())
	} else {
		// The generated _redirects/_headers moved with the build; the preview
		// re-reads them so it keeps serving what the platform would (#181).
		republishOutputRules(cfg)
		notifyReload()
	}
	return out, err
}

// reconfigure adopts a reloaded configuration. Taken under the same lock as a
// build, so a rebuild in flight finishes against the settings it started with
// rather than swapping generators half-way.
func (r *mcpRebuilder) reconfigure(genCfg generator.Config, cfg *config.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.genCfg, r.cfg = genCfg, cfg
}

// config returns the configuration a rebuild would currently use.
func (r *mcpRebuilder) config() *config.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

// mcpWatchInterval is the poll period, matching the serve path's loop.
const mcpWatchInterval = 1 * time.Second

// mcpWatcher polls the site's inputs on behalf of `ssg mcp --watch`. It holds
// the same state runWatchLoop holds — one signature cache reused across polls,
// the last build time, the last content signature, the config file's own hash —
// because it is the same watch, driven from a different command.
type mcpWatcher struct {
	rebuilder  *mcpRebuilder
	args       []string // the arguments left after `ssg mcp`'s own flags
	configPath string
	logf       func(string, ...any)
	dirs       []string
	sigCache   *fileSigCache
	lastBuild  time.Time
	lastSig    string
	configSig  string
	stop       chan struct{}
	done       chan struct{}
	// interval is the poll period. A field rather than a read of the constant
	// so a test can drive the loop at its own pace without writing to state
	// another test's watcher is still reading.
	interval time.Duration
}

// newMCPWatcher primes a watcher from the current state of the tree, so the
// first poll reports changes made after the process started rather than
// rebuilding immediately on everything that already existed.
func newMCPWatcher(r *mcpRebuilder, args []string, configPath string, logf func(string, ...any)) *mcpWatcher {
	cfg := r.config()
	dirs := watchDirs(cfg)
	cache := newFileSigCache()
	return &mcpWatcher{
		rebuilder:  r,
		args:       args,
		configPath: configPath,
		logf:       logf,
		dirs:       dirs,
		sigCache:   cache,
		lastBuild:  time.Now(),
		lastSig:    cache.signature(dirs),
		configSig:  fileSignature(configPath),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		interval:   mcpWatchInterval,
	}
}

// poll runs one iteration and reports whether it rebuilt.
//
// The config file is an input of its own, exactly as it is in the serve path
// (#70): an edit reloads it, republishes the endpoints onto the running preview
// (#180) and rebuilds, so a watcher never keeps building from the configuration
// it started with.
func (w *mcpWatcher) poll() bool {
	if sig := fileSignature(w.configPath); sig != w.configSig {
		w.configSig = sig
		w.reload()
		_, _ = w.rebuilder.rebuild()
		w.lastBuild = time.Now()
		return true
	}
	rebuilt := false
	w.lastBuild, w.lastSig = watchIteration(w.dirs, w.sigCache, w.lastBuild, w.lastSig, func() {
		rebuilt = true
		_, _ = w.rebuilder.rebuild()
	})
	return rebuilt
}

// reload adopts an edited config file, or keeps the last good one when the edit
// does not parse — a half-saved file must not take the watcher down.
//
// The boundary is worth stating plainly: what follows the file is what a
// *rebuild* is made of, plus the endpoint routing table. The MCP server's own
// surface — which roles are exposed, which directories its tools may write to,
// whether the git PR flow is armed — is fixed when the process starts, because
// those were handed to mcp.NewServer and a running server does not re-read
// them. Moving content_dir in a live config therefore changes what is built and
// not what the assistant is allowed to edit; that needs a restart.
func (w *mcpWatcher) reload() {
	newGen, newCfg, ok := reloadWatchConfig(w.args, w.configPath, w.rebuilder.config())
	if !ok {
		return
	}
	w.rebuilder.reconfigure(newGen, newCfg)
	w.dirs = watchDirs(newCfg)
	w.sigCache = newFileSigCache()
	w.lastSig = w.sigCache.signature(w.dirs)
}

// run polls until the watcher is stopped. Called on its own goroutine: `ssg
// mcp` is already blocked on stdio or parked on the HTTP transport.
func (w *mcpWatcher) run() {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// stopWatching ends the loop and waits for the poll in flight to finish. The
// CLI never calls it — the watcher lives as long as the process it belongs to —
// but a loop with no way out is a loop no test can start without leaking it
// into every test that follows, and a rebuild still running after its owner
// went away is the same shared-output hazard this file exists to remove.
//
// Only valid on a watcher whose run() was started; it waits for that goroutine.
func (w *mcpWatcher) stopWatching() {
	close(w.stop)
	<-w.done
}

// startMCPWatch begins watching the filesystem when `--watch` was given, and
// says which inputs it is watching — the same line the serve path prints, on
// stderr, because stdout is the JSON-RPC channel.
//
// Without --watch this is a no-op, so a plain `ssg mcp` is unchanged: MCP
// mutations still rebuild through the same serialised path.
func startMCPWatch(r *mcpRebuilder, args []string, configPath string, logf func(string, ...any)) *mcpWatcher {
	if !r.config().Watch {
		return nil
	}
	w := newMCPWatcher(r, args, configPath, logf)
	logf("   👀 watching %s — edits from outside MCP rebuild and reload too", strings.Join(watchedInputs(r.config(), configPath), ", "))
	go w.run()
	return w
}
