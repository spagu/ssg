package resources

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// fakeCgroup points the cgroup reader at a temporary tree: the process sits
// in /a/b, and limits holds memory.max per directory ("" = no file).
func fakeCgroup(t *testing.T, self string, limits map[string]string) {
	t.Helper()
	root := t.TempDir()
	for dir, v := range limits {
		d := filepath.Join(root, filepath.FromSlash(dir))
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "memory.max"), []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	proc := filepath.Join(root, "cgroup")
	if err := os.WriteFile(proc, []byte(self), 0o600); err != nil {
		t.Fatal(err)
	}
	oldRoot, oldProc := cgroupRoot, procSelfCgroup
	cgroupRoot, procSelfCgroup = root, proc
	t.Cleanup(func() { cgroupRoot, procSelfCgroup = oldRoot, oldProc })
}

// restoreRuntime puts back what Apply changes, so tests do not leak a limit.
func restoreRuntime(t *testing.T) {
	t.Helper()
	procs := runtime.GOMAXPROCS(0)
	mem := debug.SetMemoryLimit(-1)
	gc := debug.SetGCPercent(-1)
	debug.SetGCPercent(gc)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(procs)
		debug.SetMemoryLimit(mem)
		debug.SetGCPercent(gc)
	})
}

func TestCgroupMemoryLimit(t *testing.T) {
	// The tightest limit on the path wins; "max" is no limit.
	fakeCgroup(t, "1:memory:/v1\n0::/a/b\n", map[string]string{"a/b": "max", "a": "1073741824", "": "2147483648"})
	if n, ok := CgroupMemoryLimit(); !ok || n != 1<<30 {
		t.Errorf("limit = %d %v", n, ok)
	}
	fakeCgroup(t, "0::/\n", map[string]string{"": "max"})
	if _, ok := CgroupMemoryLimit(); ok {
		t.Error("max at the root is no limit")
	}
	fakeCgroup(t, "0::/x\n", map[string]string{"x": "garbage"})
	if _, ok := CgroupMemoryLimit(); ok {
		t.Error("an unreadable value is no limit")
	}
	fakeCgroup(t, "3:cpu:/legacy\n", nil) // cgroup v1 only
	if _, ok := CgroupMemoryLimit(); ok {
		t.Error("cgroup v1 is not read")
	}
	procSelfCgroup = filepath.Join(t.TempDir(), "none")
	if _, ok := CgroupMemoryLimit(); ok {
		t.Error("no /proc/self/cgroup is no limit")
	}
}

func TestResolve(t *testing.T) {
	fakeCgroup(t, "0::/c\n", map[string]string{"c": "1000"})
	l, err := Resolve(Request{})
	if err != nil || l.Memory != 900 || !l.FromCgroup {
		t.Errorf("cgroup fit: %+v %v", l, err)
	}
	if l, _ := Resolve(Request{Memory: "None"}); l.Memory != 0 || l.Set() {
		t.Errorf("none: %+v", l)
	}
	l, err = Resolve(Request{CPU: 2, Memory: "512MiB"})
	if err != nil || l.Memory != 512<<20 || l.FromCgroup || l.CPU != 2 {
		t.Errorf("configured: %+v %v", l, err)
	}

	gc := -2
	for name, r := range map[string]Request{
		"resources.cpu":        {CPU: -1},
		"resources.gc_percent": {GCPercent: &gc},
		"resources.memory":     {Memory: "lots"},
		"mem_limit":            {Memory: "0", MemoryField: "mem_limit"},
	} {
		if _, err := Resolve(r); err == nil || !strings.HasPrefix(err.Error(), name+":") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestApply(t *testing.T) {
	restoreRuntime(t)
	gc := 50
	Limits{CPU: 1, Memory: 256 << 20, GCPercent: &gc}.Apply()
	if runtime.GOMAXPROCS(0) != 1 {
		t.Errorf("GOMAXPROCS = %d", runtime.GOMAXPROCS(0))
	}
	if got := debug.SetMemoryLimit(-1); got != 256<<20 {
		t.Errorf("memory limit = %d", got)
	}
	if got := debug.SetGCPercent(gc); got != 50 {
		t.Errorf("gc percent = %d", got)
	}
	// More CPUs than the machine has is the machine.
	Limits{CPU: runtime.NumCPU() + 8}.Apply()
	if runtime.GOMAXPROCS(0) != runtime.NumCPU() {
		t.Errorf("GOMAXPROCS = %d", runtime.GOMAXPROCS(0))
	}
	Limits{}.Apply() // nothing to do
}

func TestWorkersAndString(t *testing.T) {
	if (Limits{CPU: 2}).Workers(8) != 2 || (Limits{CPU: 2}).Workers(1) != 1 || (Limits{}).Workers(8) != 8 {
		t.Error("Workers")
	}
	gc := 50
	s := Limits{CPU: 1, Memory: 1536 << 20, FromCgroup: true, GCPercent: &gc}.String()
	for _, want := range []string{"memory limit 1.5 GiB (90% of the cgroup)", "1 CPU(s)", "gc_percent 50"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q lacks %q", s, want)
		}
	}
	if (Limits{}).String() != "" {
		t.Error("no limits, no line")
	}
}

func TestParseAndFormatSize(t *testing.T) {
	for in, want := range map[string]int64{
		"512MiB": 512 << 20, "1GiB": 1 << 30, "256KiB": 256 << 10, "1048576": 1048576,
		"10MB": 10_000_000, "1KB": 1000, "2GB": 2e9, "7B": 7, " 1.5 GiB ": 3 << 29,
	} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d %v, want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"nonsense", "lots-of-MiB"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) accepted", bad)
		}
	}
	for n, want := range map[int64]string{512 << 20: "512 MiB", 3 << 29: "1.5 GiB", 2048: "2 KiB", 100: "100 B"} {
		if got := FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPeakRSS(t *testing.T) {
	if runtime.GOOS != "windows" && PeakRSS() <= 0 {
		t.Error("a running process has resident memory")
	}
	if maxRSSBytes(3, "darwin") != 3 || maxRSSBytes(3, "linux") != 3072 {
		t.Error("maxRSSBytes units")
	}
}
