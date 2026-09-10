package daemon

// The supervisor when a project misbehaves or nobody is listening (#169): a
// project that will not stop must not wedge a reload, and a daemon with no log
// must still run the fleet.

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubbornRunner hands out processes whose Stop always fails, which is what a
// project that could not be ended looks like from up here.
type stubbornRunner struct {
	mu      sync.Mutex
	started int
}

func (r *stubbornRunner) Start(Project) (Process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started++
	return &stubbornProcess{done: make(chan struct{})}, nil
}

func (r *stubbornRunner) starts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

type stubbornProcess struct{ done chan struct{} }

func (p *stubbornProcess) Done() <-chan struct{} { return p.done }

func (p *stubbornProcess) Stop(time.Duration) error {
	return fmt.Errorf("killing pid 4242: operation not permitted")
}

// TestAProjectThatWouldNotStopIsReportedAndForgotten: the reload has to carry
// on. Keeping a project that refused to die in the running set would mean the
// next Apply thinks it is still serving and never starts it again — the site
// would be down with the daemon insisting it is up. Saying so in the log is the
// operator's only clue that a stray process is still holding the port.
func TestAProjectThatWouldNotStopIsReportedAndForgotten(t *testing.T) {
	r := &stubbornRunner{}
	var log strings.Builder
	sup := NewSupervisor(r, &log, time.Millisecond)
	defer sup.Shutdown()

	if err := sup.Apply([]Project{project("blog", 8801)}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Apply(nil); err != nil {
		t.Fatal(err)
	}

	if got := log.String(); !strings.Contains(got, "blog did not stop cleanly") {
		t.Errorf("the log must name the project that would not stop:\n%s", got)
	}
	if strings.Contains(log.String(), "⏹️") {
		t.Errorf("a project that would not stop must not be reported as stopped:\n%s", log.String())
	}
	if got := sup.Names(); len(got) != 0 {
		t.Errorf("running = %v, want it forgotten so the next reload can start it again", got)
	}
	// And the next reload does start it again, rather than assuming it is up.
	if err := sup.Apply([]Project{project("blog", 8801)}); err != nil {
		t.Fatal(err)
	}
	if got := r.starts(); got != 2 {
		t.Errorf("started %d times, want the project brought back after the failed stop", got)
	}
}

// TestASupervisorWithNoLogStillRunsTheFleet: the log is an io.Writer the caller
// may not have — `ssg daemon` piped to nothing, or an embedding that wants a
// quiet supervisor. Narration is a courtesy, so a nil log must be silence, not
// a nil dereference in the middle of a reload.
func TestASupervisorWithNoLogStillRunsTheFleet(t *testing.T) {
	r := newFakeRunner()
	r.fail["shop"] = true
	sup := NewSupervisor(r, nil, time.Millisecond)

	// Every line the supervisor would have written: a start, a failed start, a
	// restart, an exit and a stop.
	if err := sup.Apply([]Project{project("blog", 8801), project("shop", 8802)}); err == nil {
		t.Error("the failed start must still be reported to the caller")
	}
	r.procs["blog"].exit()
	if got := sup.Exited(); got != "blog" {
		t.Fatalf("Exited() = %q, want blog", got)
	}
	if err := sup.Restart("blog"); err != nil {
		t.Fatal(err)
	}
	sup.Shutdown()

	if got := sup.Names(); len(got) != 0 {
		t.Errorf("running = %v after shutdown", got)
	}
	if got := r.startedNames(); strings.Join(got, ",") != "blog,blog" {
		t.Errorf("started %v, want blog started and restarted", got)
	}
}
