package generator

// Where the build's time goes (GO-097).
//
// Every phase of a build already passes through one seam — runStep — which
// logged what it was about to do and measured nothing. The only number the
// tool could report about its own work was a markdown-conversion counter, so
// a site whose build had grown to twelve seconds had no way to learn which
// part of it was slow; the answer lived in a blog post about one machine.
//
// A Profile is that missing accounting: a phase list in execution order, a
// per-page cost table, and named counters that the code already had reason to
// keep. It is created only when asked for, and every method is safe on a nil
// receiver, so the instrumented call sites read the same whether profiling is
// on or off and the off path adds a nil check.

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Profile modes, as the --profile flag and the `profile:` config key spell them.
const (
	ProfileText = "text"
	ProfileJSON = "json"
)

// StepTiming is one measured phase of the build.
type StepTiming struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"-"`
	// Millis is the duration as the artifact carries it: whole milliseconds,
	// because a JSON consumer comparing two builds wants a number it can
	// subtract, not a Go duration string.
	Millis float64 `json:"ms"`
}

// PageTiming is what one output page cost to render.
type PageTiming struct {
	Path     string        `json:"path"`
	Duration time.Duration `json:"-"`
	Millis   float64       `json:"ms"`
}

// pageShards is how many independent slices collect per-page timings.
//
// Pages render on a worker pool, so a single mutex here would put the profiler
// on the hot path of the thing it measures. Each record takes a shard by an
// atomic counter, which spreads the writers and keeps the lock uncontended.
const pageShards = 8

// pageBucket is one shard of the per-page table.
type pageBucket struct {
	mu   sync.Mutex
	list []PageTiming
	// pad keeps two buckets off one cache line, so writers on different
	// workers do not invalidate each other's line for a lock they do not share.
	pad [40]byte //nolint:unused // padding, deliberately never read
}

// Profile collects one build's measurements.
type Profile struct {
	mu       sync.Mutex
	steps    []StepTiming
	counters map[string]int64
	order    []string // counter names in first-seen order

	pages [pageShards]pageBucket
	next  atomic.Uint64

	start time.Time
	total time.Duration
	// now is the clock, injectable so a test can assert exact durations.
	now func() time.Time
}

// NewProfile starts a profile. mode selects the report; an empty mode means
// the caller does not want one and returns nil, which every method accepts.
func NewProfile(mode string) *Profile {
	if mode == "" {
		return nil
	}
	return newProfileWithClock(time.Now)
}

// newProfileWithClock is NewProfile with the clock supplied, for tests.
func newProfileWithClock(now func() time.Time) *Profile {
	p := &Profile{counters: map[string]int64{}, now: now}
	p.start = now()
	return p
}

// Measure runs fn and records how long it took under name. On a nil profile it
// simply runs fn, so a call site does not branch on whether profiling is on.
func (p *Profile) Measure(name string, fn func() error) error {
	if p == nil {
		return fn()
	}
	started := p.now()
	err := fn()
	p.Add(name, p.now().Sub(started))
	return err
}

// Add records a phase directly, for work whose timing the caller already has.
// A repeated name accumulates rather than appearing twice: `runStep` is called
// again on a watch rebuild, and two "Loading content" rows would be a lie
// about the shape of the build.
func (p *Profile) Add(name string, d time.Duration) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.steps {
		if p.steps[i].Name == name {
			p.steps[i].Duration += d
			return
		}
	}
	p.steps = append(p.steps, StepTiming{Name: name, Duration: d})
}

// Count adds n to a named counter, creating it on first use.
func (p *Profile) Count(name string, n int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, seen := p.counters[name]; !seen {
		p.order = append(p.order, name)
	}
	p.counters[name] += n
}

// Page records the cost of rendering one output page.
func (p *Profile) Page(path string, d time.Duration) {
	if p == nil {
		return
	}
	b := &p.pages[p.next.Add(1)%pageShards]
	b.mu.Lock()
	b.list = append(b.list, PageTiming{Path: path, Duration: d})
	b.mu.Unlock()
}

// Finish stops the clock. Calling it twice keeps the first total, so a nested
// caller cannot shorten the build it is part of.
func (p *Profile) Finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.total == 0 {
		p.total = p.now().Sub(p.start)
	}
}

// Total is the whole build's duration, or the elapsed time so far when Finish
// has not run.
func (p *Profile) Total() time.Duration {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.total != 0 {
		return p.total
	}
	return p.now().Sub(p.start)
}

// Steps returns the phases in execution order.
func (p *Profile) Steps() []StepTiming {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]StepTiming, len(p.steps))
	copy(out, p.steps)
	for i := range out {
		out[i].Millis = millis(out[i].Duration)
	}
	return out
}

// Counters returns the named counters in first-seen order.
func (p *Profile) Counters() []Counter {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Counter, 0, len(p.order))
	for _, name := range p.order {
		out = append(out, Counter{Name: name, Value: p.counters[name]})
	}
	return out
}

// Counter is one named tally the build kept.
type Counter struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// Pages returns every page timing, slowest first. Ties break on path so two
// runs of the same site report the same order.
func (p *Profile) Pages() []PageTiming {
	if p == nil {
		return nil
	}
	var out []PageTiming
	for i := range p.pages {
		p.pages[i].mu.Lock()
		out = append(out, p.pages[i].list...)
		p.pages[i].mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Duration != out[j].Duration {
			return out[i].Duration > out[j].Duration
		}
		return out[i].Path < out[j].Path
	})
	for i := range out {
		out[i].Millis = millis(out[i].Duration)
	}
	return out
}

// PageCost returns what one output path cost, and whether it was measured.
func (p *Profile) PageCost(path string) (time.Duration, bool) {
	for _, pt := range p.Pages() {
		if pt.Path == path {
			return pt.Duration, true
		}
	}
	return 0, false
}

// millis renders a duration as milliseconds with one decimal, the precision a
// build report can actually stand behind.
func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
