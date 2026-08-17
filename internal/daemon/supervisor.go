package daemon

// Keeping the fleet running, and reloading it without stopping it.
//
// The reload rule is the whole design: a project whose fingerprint did not
// change is left running untouched. Editing one project's port must not rebuild
// the other three, and adding a fifth must not interrupt the four — otherwise
// "reload" is a restart wearing a different word.

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// Runner starts one project and returns a handle to the process it started. It
// is the seam that keeps tests off the process table.
type Runner interface {
	Start(p Project) (Process, error)
}

// Process is a running project.
type Process interface {
	// Stop ends it, waiting up to grace for it to leave on its own before
	// killing it. A watch loop holds a server socket, so an unclean stop means
	// the next start finds the port busy.
	Stop(grace time.Duration) error
	// Done closes when the process exits by itself — a crash, or a build that
	// gave up. The supervisor restarts it.
	Done() <-chan struct{}
}

// Supervisor keeps one process per active project and reconciles that set
// against a new configuration.
type Supervisor struct {
	runner Runner
	log    io.Writer
	grace  time.Duration

	mu      sync.Mutex
	running map[string]*supervised // by project name
	closed  bool
}

// supervised is one project's process and the fingerprint it was started from.
type supervised struct {
	project     Project
	proc        Process
	fingerprint string
}

// NewSupervisor returns a supervisor that starts projects with runner and
// narrates what it does to log.
func NewSupervisor(runner Runner, log io.Writer, grace time.Duration) *Supervisor {
	if grace <= 0 {
		grace = 5 * time.Second
	}
	return &Supervisor{runner: runner, log: log, grace: grace, running: map[string]*supervised{}}
}

// Apply reconciles the running set against want: projects that are gone are
// stopped, new ones started, changed ones restarted, and unchanged ones left
// exactly as they are.
func (s *Supervisor) Apply(want []Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("supervisor is shut down")
	}

	desired := map[string]Project{}
	for _, p := range want {
		desired[p.Name] = p
	}

	// Stop what is gone or changed, before starting anything: a project that
	// moved to a different port must release the old one first.
	for _, name := range sortedKeys(s.running) {
		cur := s.running[name]
		next, keep := desired[name]
		if keep && next.Fingerprint() == cur.fingerprint {
			continue
		}
		verb := "stopped"
		if keep {
			verb = "restarting"
		}
		s.stopLocked(name, verb)
	}

	var firstErr error
	for _, name := range sortedKeys(desired) {
		if _, alive := s.running[name]; alive {
			continue
		}
		if err := s.startLocked(desired[name]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// startLocked starts one project. A project that will not start is reported and
// skipped rather than aborting the fleet: three sites serving is better than
// none, and the one that failed is named.
func (s *Supervisor) startLocked(p Project) error {
	proc, err := s.runner.Start(p)
	if err != nil {
		s.logf("   ❌ %s: %v", p.Name, err)
		return fmt.Errorf("starting %s: %w", p.Name, err)
	}
	s.running[p.Name] = &supervised{project: p, proc: proc, fingerprint: p.Fingerprint()}
	s.logf("   ▶️  %s (%s)", p.Name, p.Dir)
	return nil
}

// stopLocked stops one project and forgets it.
func (s *Supervisor) stopLocked(name, verb string) {
	cur, ok := s.running[name]
	if !ok {
		return
	}
	delete(s.running, name)
	if err := cur.proc.Stop(s.grace); err != nil {
		s.logf("   ⚠️  %s did not stop cleanly: %v", name, err)
		return
	}
	s.logf("   ⏹️  %s %s", name, verb)
}

// Names lists the running projects, for a status line.
func (s *Supervisor) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedKeys(s.running)
}

// Exited returns the name of a running project whose process has ended by
// itself, or "" when they are all alive. The caller decides whether to restart
// it; keeping the policy out here is what makes the supervisor testable.
func (s *Supervisor) Exited() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range sortedKeys(s.running) {
		select {
		case <-s.running[name].proc.Done():
			return name
		default:
		}
	}
	return ""
}

// Restart brings one project back after it exited on its own.
func (s *Supervisor) Restart(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.running[name]
	if !ok {
		return nil
	}
	p := cur.project
	delete(s.running, name)
	s.logf("   ♻️  %s exited on its own — restarting", name)
	return s.startLocked(p)
}

// Shutdown stops every project. Called once, on the way out.
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, name := range sortedKeys(s.running) {
		s.stopLocked(name, "stopped")
	}
}

func (s *Supervisor) logf(format string, args ...interface{}) {
	if s.log == nil {
		return
	}
	_, _ = fmt.Fprintf(s.log, format+"\n", args...)
}

// sortedKeys keeps every listing and every log line in one order, so two runs
// of the same configuration read the same way.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
