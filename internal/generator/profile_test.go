package generator

// Where the build's time goes (GO-097).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock advances by a fixed step on every reading, so a test can assert
// exact durations instead of hoping the machine is fast.
func fakeClock(step time.Duration) func() time.Time {
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	n := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t := base.Add(time.Duration(n) * step)
		n++
		return t
	}
}

// TestNilProfileIsTheOffSwitch: every method accepts a nil receiver, which is
// what lets the instrumented call sites read the same whether profiling is on
// or off. Measure must still run the work.
func TestNilProfileIsTheOffSwitch(t *testing.T) {
	var p *Profile
	ran := false
	if err := p.Measure("x", func() error { ran = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("a nil profile must still run the work it is not measuring")
	}
	p.Add("x", time.Second)
	p.Count("y", 1)
	p.Page("/a/", time.Second)
	p.Finish()
	if p.Total() != 0 || p.Steps() != nil || p.Counters() != nil || p.Pages() != nil {
		t.Error("a nil profile must report nothing")
	}
	if _, ok := p.PageCost("/a/"); ok {
		t.Error("a nil profile has no page costs")
	}
	if path, err := p.WriteJSON(t.TempDir(), "v", time.Now()); err != nil || path != "" {
		t.Errorf("nil WriteJSON = %q, %v", path, err)
	}
	var buf bytes.Buffer
	p.WriteText(&buf, time.Now())
	if buf.Len() != 0 {
		t.Errorf("nil WriteText wrote %q", buf.String())
	}
	if NewProfile("") != nil {
		t.Error("an empty mode must not create a profile")
	}
}

// TestMeasureRecordsAndAccumulates: a phase is timed, its error is returned
// unchanged, and a repeated name accumulates rather than appearing twice — a
// watch rebuild must not turn one phase into a list.
func TestMeasureRecordsAndAccumulates(t *testing.T) {
	p := newProfileWithClock(fakeClock(time.Millisecond))
	if err := p.Measure("Load", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := p.Measure("Load", func() error { return os.ErrClosed }); err != os.ErrClosed {
		t.Errorf("Measure must return the work's error, got %v", err)
	}
	p.Add("Render", 3*time.Millisecond)
	steps := p.Steps()
	if len(steps) != 2 || steps[0].Name != "Load" || steps[1].Name != "Render" {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[0].Duration != 2*time.Millisecond {
		t.Errorf("two Load phases should accumulate, got %v", steps[0].Duration)
	}
	if steps[1].Millis != 3 {
		t.Errorf("Millis = %v", steps[1].Millis)
	}
}

// TestFinishKeepsTheFirstTotal: Total ticks while the build runs and freezes
// when it ends, so a nested caller cannot shorten the build it is part of.
func TestFinishKeepsTheFirstTotal(t *testing.T) {
	p := newProfileWithClock(fakeClock(time.Second))
	if p.Total() == 0 {
		t.Error("Total must report elapsed time before Finish")
	}
	p.Finish()
	first := p.Total()
	p.Finish()
	if p.Total() != first {
		t.Errorf("a second Finish changed the total: %v → %v", first, p.Total())
	}
}

// TestCountersKeepFirstSeenOrder: the report reads in the order the build
// learned things, not in map order, so two runs print the same table.
func TestCountersKeepFirstSeenOrder(t *testing.T) {
	p := newProfileWithClock(fakeClock(time.Millisecond))
	p.Count("pages rendered", 2)
	p.Count("markdown conversions", 5)
	p.Count("pages rendered", 3)
	got := p.Counters()
	if len(got) != 2 || got[0].Name != "pages rendered" || got[0].Value != 5 || got[1].Value != 5 {
		t.Errorf("counters = %+v", got)
	}
}

// TestPagesAreSortedSlowestFirst: including across shards, and ties break on
// path so the report is stable between runs.
func TestPagesAreSortedSlowestFirst(t *testing.T) {
	p := newProfileWithClock(fakeClock(time.Millisecond))
	for i := 0; i < pageShards*3; i++ {
		p.Page("/p"+string(rune('a'+i))+"/", time.Duration(i)*time.Millisecond)
	}
	p.Page("/tie-b/", 5*time.Millisecond)
	p.Page("/tie-a/", 5*time.Millisecond)
	pages := p.Pages()
	if len(pages) != pageShards*3+2 {
		t.Fatalf("lost timings across shards: %d", len(pages))
	}
	for i := 1; i < len(pages); i++ {
		if pages[i-1].Duration < pages[i].Duration {
			t.Fatalf("not sorted at %d: %+v", i, pages[i-1:i+1])
		}
	}
	var tieA, tieB int
	for i, pt := range pages {
		switch pt.Path {
		case "/tie-a/":
			tieA = i
		case "/tie-b/":
			tieB = i
		}
	}
	if tieA > tieB {
		t.Error("equal durations must order by path")
	}
	cost, ok := p.PageCost("/tie-a/")
	if !ok || cost != 5*time.Millisecond {
		t.Errorf("PageCost = %v, %v", cost, ok)
	}
	if _, ok := p.PageCost("/absent/"); ok {
		t.Error("an unmeasured page must report so")
	}
}

// TestPageRecordingIsConcurrencySafe: pages render on a worker pool, so the
// collector is written from many goroutines at once.
func TestPageRecordingIsConcurrencySafe(t *testing.T) {
	p := newProfileWithClock(time.Now)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.Page("/p/", time.Millisecond)
			p.Count("pages rendered", 1)
		}(i)
	}
	wg.Wait()
	if len(p.Pages()) != 50 {
		t.Errorf("recorded %d of 50", len(p.Pages()))
	}
	if c := p.Counters(); len(c) != 1 || c[0].Value != 50 {
		t.Errorf("counters = %+v", c)
	}
}

// TestTextReportShapesTheTable: phases in order with their share, counters on
// one line, and the slowest pages capped at ten.
func TestTextReportShapesTheTable(t *testing.T) {
	p := newProfileWithClock(fakeClock(500 * time.Millisecond))
	p.Add("Loading content", 400*time.Millisecond)
	p.Add("Generating site", 1600*time.Millisecond)
	p.Add("A very long phase name that overflows", time.Microsecond)
	p.Count("markdown conversions", 4812)
	for i := 0; i < 15; i++ {
		p.Page("/page-"+string(rune('a'+i))+"/", time.Duration(15-i)*time.Millisecond)
	}
	p.Page("/a/very/deeply/nested/section/that/will/not/fit/in/the/column/", 99*time.Millisecond)
	p.Finish()

	var buf bytes.Buffer
	p.WriteText(&buf, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC))
	out := buf.String()
	for _, want := range []string{"Build profile (2026-09-10 12:00:00)", "Total", "Loading content", "1.60 s", "markdown conversions 4812", "Slowest pages (10 of 16)"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "/page-") > 10 {
		t.Error("the text report must cap the page list")
	}
	if !strings.Contains(out, "…") {
		t.Error("an over-long path should be trimmed to the column")
	}
}

// TestTextReportWithoutPagesOrTotal: a report can be printed before anything
// was measured without dividing by zero or inventing rows.
func TestTextReportWithoutPagesOrTotal(t *testing.T) {
	p := newProfileWithClock(fakeClock(0))
	p.Add("Only phase", time.Millisecond)
	p.Finish()
	var buf bytes.Buffer
	p.WriteText(&buf, time.Now())
	out := buf.String()
	if strings.Contains(out, "Slowest pages") {
		t.Error("no pages, no page table")
	}
	if strings.Contains(out, "%") {
		t.Errorf("a zero total must not print a share:\n%s", out)
	}
}

// TestJSONReportRoundTrips: the artifact carries the same numbers back, and is
// stable enough for CI to diff between commits.
func TestJSONReportRoundTrips(t *testing.T) {
	dir := t.TempDir()
	p := newProfileWithClock(fakeClock(time.Millisecond))
	p.Add("Generating site", 42*time.Millisecond)
	p.Count("pages rendered", 7)
	p.Page("/a/", 3*time.Millisecond)
	p.Finish()
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	path, err := p.WriteJSON(dir, "1.8.60", at)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != ProfileFileName {
		t.Errorf("wrote %s", path)
	}
	got, err := LoadProfileReport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != ProfileSchema || got.Version != "1.8.60" || !got.Time.Equal(at) {
		t.Errorf("stamp lost: %+v", got)
	}
	if len(got.Steps) != 1 || got.Steps[0].Millis != 42 || len(got.Pages) != 1 || got.Pages[0].Millis != 3 {
		t.Errorf("numbers lost: %+v", got)
	}
	if len(got.Counters) != 1 || got.Counters[0].Value != 7 {
		t.Errorf("counters lost: %+v", got.Counters)
	}
	// Two writes of the same profile produce the same bytes.
	first, _ := os.ReadFile(path)
	if _, err := p.WriteJSON(dir, "1.8.60", at); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Error("the report is not stable between writes")
	}
}

// TestLoadProfileReportErrors: missing, unparsable and a schema this build does
// not read are each reported as themselves.
func TestLoadProfileReportErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadProfileReport(dir); !os.IsNotExist(err) {
		t.Errorf("missing report: %v", err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, ProfileFileName), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("{not json")
	if _, err := LoadProfileReport(dir); err == nil || !strings.Contains(err.Error(), ProfileFileName) {
		t.Errorf("unparsable: %v", err)
	}
	write(`{"schema": 99}`)
	if _, err := LoadProfileReport(dir); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Errorf("foreign schema: %v", err)
	}
}

// TestWriteJSONReportsAnUnwritableTarget: a report that cannot be written says
// so rather than being silently skipped.
func TestWriteJSONReportsAnUnwritableTarget(t *testing.T) {
	p := newProfileWithClock(fakeClock(time.Millisecond))
	if _, err := p.WriteJSON(filepath.Join(t.TempDir(), "missing"), "v", time.Now()); err == nil {
		t.Error("a missing directory must fail the write")
	}
}

// TestStepNameStripsTheLogDecoration: the report row is the phase, not the log
// line the person watching the build sees.
func TestStepNameStripsTheLogDecoration(t *testing.T) {
	cases := map[string]string{
		"🔄 Loading content...":      "Loading content",
		"📝 Loading templates":       "Loading templates",
		"Plain step":                "Plain step",
		"☁️  Generating Cloudflare": "Generating Cloudflare",
	}
	for in, want := range cases {
		if got := stepName(in); got != want {
			t.Errorf("stepName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestProfiledBuildMeasuresItself: with the profile on, a real build reports
// its phases, counts its pages and names them by URL — and with it off, the
// generator carries no profile at all.
func TestProfiledBuildMeasuresItself(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md":     "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout us.\n",
		"posts/news/one.md":  "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\n---\n\nOne.\n",
		"posts/news/two.md":  "---\ntitle: Two\nslug: two\nstatus: publish\ntype: post\ndate: 2024-01-03\n---\n\nTwo.\n",
		"pages/repeated.md":  "---\ntitle: Repeat\nslug: repeat\nstatus: publish\ntype: page\n---\n\nAbout us.\n",
		"pages/repeated2.md": "---\ntitle: Repeat2\nslug: repeat2\nstatus: publish\ntype: page\n---\n\nAbout us.\n",
	}, nil)
	cfg.Profile = ProfileText

	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if gen.Profile() == nil {
		t.Fatal("Profile() must exist when the config asks for one")
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	prof := gen.Profile()
	prof.Finish()

	names := map[string]bool{}
	for _, s := range prof.Steps() {
		names[s.Name] = true
	}
	for _, want := range []string{"Loading content", "Generating site", "Assets and checks", "Sitemap and robots"} {
		if !names[want] {
			t.Errorf("phase %q was not measured; got %v", want, names)
		}
	}
	pages := prof.Pages()
	if len(pages) == 0 {
		t.Fatal("no page timings")
	}
	for _, p := range pages {
		if !strings.HasPrefix(p.Path, "/") || strings.Contains(p.Path, cfg.OutputDir) {
			t.Errorf("page named by disk path, not URL: %q", p.Path)
		}
	}
	counters := map[string]int64{}
	for _, c := range prof.Counters() {
		counters[c.Name] = c.Value
	}
	if counters["pages rendered"] != int64(len(pages)) {
		t.Errorf("pages rendered %d, timings %d", counters["pages rendered"], len(pages))
	}
	if counters["markdown conversions"] == 0 {
		t.Error("markdown conversions were not counted")
	}
	if counters["markdown cache hits"] == 0 {
		t.Error("three pages share one body; the cache hit was not counted")
	}

	// And the off switch: no profile, no measurement, same site.
	off := cfg
	off.OutputDir = t.TempDir()
	off.Profile = ""
	genOff, err := New(off)
	if err != nil {
		t.Fatal(err)
	}
	if genOff.Profile() != nil {
		t.Error("profiling must be off by default")
	}
	if err := genOff.Generate(); err != nil {
		t.Fatal(err)
	}
}

// TestProfileDoesNotChangeTheOutput: the instrument must not move the needle it
// reads. Two builds of one site, one profiled, byte-identical.
func TestProfileDoesNotChangeTheOutput(t *testing.T) {
	files := map[string]string{
		"pages/about.md":    "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\n# Hi\n\nAbout us.\n",
		"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ntags: [go]\n---\n\nOne.\n",
	}
	plain := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, nil)
	buildSiteFixture(t, plain)

	profiled := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, nil)
	profiled.Profile = ProfileJSON
	buildSiteFixture(t, profiled)

	compareTrees(t, plain.OutputDir, profiled.OutputDir)
}

// compareTrees fails with the first file that differs between two output trees.
func compareTrees(t *testing.T, a, b string) {
	t.Helper()
	seen := map[string]bool{}
	walk := func(root string, record func(rel string, data []byte)) {
		if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			data, err := os.ReadFile(p) // #nosec G304 -- test fixture
			if err != nil {
				return err
			}
			record(filepath.ToSlash(rel), data)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	first := map[string][]byte{}
	walk(a, func(rel string, data []byte) { first[rel] = data })
	walk(b, func(rel string, data []byte) {
		seen[rel] = true
		other, ok := first[rel]
		if !ok {
			t.Errorf("%s exists only in the profiled build", rel)
			return
		}
		if !bytes.Equal(other, data) {
			t.Errorf("%s differs between a profiled and an unprofiled build", rel)
		}
	})
	for rel := range first {
		if !seen[rel] {
			t.Errorf("%s exists only in the unprofiled build", rel)
		}
	}
}

// TestReportJSONIsSelfDescribing: the artifact says which schema it is, so a
// consumer can tell a format change from a site change.
func TestReportJSONIsSelfDescribing(t *testing.T) {
	p := newProfileWithClock(fakeClock(time.Millisecond))
	p.Add("Step", time.Millisecond)
	data, err := json.Marshal(p.Report("1.8.60", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema":1`) {
		t.Errorf("no schema in %s", data)
	}
	if strings.Contains(string(data), `"Duration"`) {
		t.Error("the Go duration must not reach the artifact; ms is the contract")
	}
}

// BenchmarkProfileRecording measures what the instrument costs per page, which
// is the number that has to stay far below what it measures. Run it with
// `go test ./internal/generator -bench ProfileRecording -run '^$'`.
func BenchmarkProfileRecording(b *testing.B) {
	p := newProfileWithClock(time.Now)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.Page("/some/page/", time.Millisecond)
			p.Count("pages rendered", 1)
		}
	})
}
