package daemon

// Reconciling the fleet (#169). The rule these tests exist for: a project whose
// settings did not change is LEFT RUNNING. Editing one project's port must not
// rebuild the other three, and adding a fifth must not interrupt the four —
// otherwise "reload" is a restart wearing a different word.

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner records what it was asked to start and hands back processes a test
// can end on demand, so nothing here touches the process table.
type fakeRunner struct {
	mu      sync.Mutex
	started []string
	fail    map[string]bool
	procs   map[string]*fakeProcess
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{fail: map[string]bool{}, procs: map[string]*fakeProcess{}}
}

func (r *fakeRunner) Start(p Project) (Process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail[p.Name] {
		return nil, fmt.Errorf("refusing to start %s", p.Name)
	}
	r.started = append(r.started, p.Name)
	proc := &fakeProcess{done: make(chan struct{})}
	r.procs[p.Name] = proc
	return proc, nil
}

func (r *fakeRunner) startedNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.started...)
}

type fakeProcess struct {
	mu      sync.Mutex
	done    chan struct{}
	stopped bool
	exited  bool
}

func (p *fakeProcess) Done() <-chan struct{} { return p.done }

func (p *fakeProcess) Stop(time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped && !p.exited {
		p.stopped = true
		close(p.done)
	}
	return nil
}

// exit simulates a project dying on its own.
func (p *fakeProcess) exit() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.exited && !p.stopped {
		p.exited = true
		close(p.done)
	}
}

func (p *fakeProcess) wasStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func project(name string, port int) Project {
	return Project{Name: name, Dir: "/srv/" + name, Port: port, HTTP: port != 0}
}

// TestReloadLeavesUnchangedProjectsAlone: the whole point of the feature.
func TestReloadLeavesUnchangedProjectsAlone(t *testing.T) {
	r := newFakeRunner()
	sup := NewSupervisor(r, io.Discard, time.Second)
	defer sup.Shutdown()

	first := []Project{project("blog", 8801), project("shop", 8802), project("docs", 8803)}
	if err := sup.Apply(first); err != nil {
		t.Fatal(err)
	}
	blogProc, shopProc := r.procs["blog"], r.procs["shop"]

	// docs moves port, staging is added, blog and shop are untouched.
	second := []Project{project("blog", 8801), project("shop", 8802), project("docs", 8805), project("staging", 8804)}
	if err := sup.Apply(second); err != nil {
		t.Fatal(err)
	}

	if blogProc.wasStopped() || shopProc.wasStopped() {
		t.Error("an unchanged project must keep running — and keep its port")
	}
	// Starts are alphabetical, so two runs of one file read the same way: the
	// first Apply brings up blog, docs, shop; the second restarts only docs and
	// adds only staging.
	started := r.startedNames()
	want := []string{"blog", "docs", "shop", "docs", "staging"}
	if strings.Join(started, ",") != strings.Join(want, ",") {
		t.Errorf("started %v, want %v — only docs restarts, only staging is new", started, want)
	}
	if got := sup.Names(); strings.Join(got, ",") != "blog,docs,shop,staging" {
		t.Errorf("running = %v", got)
	}
}

// TestRemovedProjectIsStopped: taking a project out of the file takes it off
// the machine.
func TestRemovedProjectIsStopped(t *testing.T) {
	r := newFakeRunner()
	sup := NewSupervisor(r, io.Discard, time.Second)
	defer sup.Shutdown()

	if err := sup.Apply([]Project{project("blog", 8801), project("shop", 8802)}); err != nil {
		t.Fatal(err)
	}
	shopProc := r.procs["shop"]
	if err := sup.Apply([]Project{project("blog", 8801)}); err != nil {
		t.Fatal(err)
	}
	if !shopProc.wasStopped() {
		t.Error("a project removed from the file must be stopped")
	}
	if got := sup.Names(); strings.Join(got, ",") != "blog" {
		t.Errorf("running = %v, want blog alone", got)
	}
}

// TestAFailedProjectDoesNotStopTheFleet: three sites serving is better than
// none, and the one that failed is named.
func TestAFailedProjectDoesNotStopTheFleet(t *testing.T) {
	r := newFakeRunner()
	r.fail["shop"] = true
	var log strings.Builder
	sup := NewSupervisor(r, &log, time.Second)
	defer sup.Shutdown()

	err := sup.Apply([]Project{project("blog", 8801), project("shop", 8802), project("docs", 8803)})
	if err == nil {
		t.Error("the failure must be reported to the caller")
	}
	if got := sup.Names(); strings.Join(got, ",") != "blog,docs" {
		t.Errorf("running = %v, want the two that started", got)
	}
	if !strings.Contains(log.String(), "shop") {
		t.Errorf("the log must name the project that failed:\n%s", log.String())
	}
}

// TestExitedProjectIsFoundAndRestarted: a build that gave up on a bad config
// must not silently leave a site unserved.
func TestExitedProjectIsFoundAndRestarted(t *testing.T) {
	r := newFakeRunner()
	sup := NewSupervisor(r, io.Discard, time.Second)
	defer sup.Shutdown()

	if err := sup.Apply([]Project{project("blog", 8801), project("shop", 8802)}); err != nil {
		t.Fatal(err)
	}
	if got := sup.Exited(); got != "" {
		t.Fatalf("nothing has exited yet, got %q", got)
	}

	r.procs["shop"].exit()
	if got := sup.Exited(); got != "shop" {
		t.Fatalf("Exited() = %q, want shop", got)
	}
	if err := sup.Restart("shop"); err != nil {
		t.Fatal(err)
	}
	if got := sup.Exited(); got != "" {
		t.Errorf("the restarted project is alive again, got %q", got)
	}
	if got := r.startedNames(); strings.Join(got, ",") != "blog,shop,shop" {
		t.Errorf("started %v, want shop started twice", got)
	}
	// Restarting something that is not running is a no-op, not a panic.
	if err := sup.Restart("nothing"); err != nil {
		t.Errorf("Restart of an unknown project = %v", err)
	}
}

// TestShutdownStopsEverything, and refuses to start more afterwards.
func TestShutdownStopsEverything(t *testing.T) {
	r := newFakeRunner()
	sup := NewSupervisor(r, io.Discard, time.Second)
	if err := sup.Apply([]Project{project("blog", 8801), project("shop", 8802)}); err != nil {
		t.Fatal(err)
	}
	procs := []*fakeProcess{r.procs["blog"], r.procs["shop"]}
	sup.Shutdown()

	for i, p := range procs {
		if !p.wasStopped() {
			t.Errorf("project %d was left running", i)
		}
	}
	if got := sup.Names(); len(got) != 0 {
		t.Errorf("running = %v after shutdown", got)
	}
	if err := sup.Apply([]Project{project("blog", 8801)}); err == nil {
		t.Error("a shut-down supervisor must not start anything")
	}
}

// TestEmptySetStopsEverything: a projects file emptied of projects is a
// deliberate instruction, not a reason to keep serving.
func TestEmptySetStopsEverything(t *testing.T) {
	r := newFakeRunner()
	sup := NewSupervisor(r, io.Discard, time.Second)
	defer sup.Shutdown()

	if err := sup.Apply([]Project{project("blog", 8801)}); err != nil {
		t.Fatal(err)
	}
	blog := r.procs["blog"]
	if err := sup.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if !blog.wasStopped() || len(sup.Names()) != 0 {
		t.Error("an empty set stops the fleet")
	}
}

// TestGraceDefaults: a zero grace period would kill a watch loop before it
// released its port.
func TestGraceDefaults(t *testing.T) {
	if got := NewSupervisor(newFakeRunner(), io.Discard, 0).grace; got != 5*time.Second {
		t.Errorf("grace = %v, want a usable default", got)
	}
	if got := NewSupervisor(newFakeRunner(), io.Discard, 2*time.Second).grace; got != 2*time.Second {
		t.Errorf("grace = %v, want the configured value", got)
	}
}
