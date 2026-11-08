// Package resources caps what one ssg process may take from the machine (#384):
// how many CPUs the Go scheduler uses, a soft ceiling on memory, and how much
// heap headroom the garbage collector allows.
//
// Every command applies the same limits, resolved once from the `resources:`
// block, its older aliases (`mem_limit`, `build_workers`) and the command line.
// They are ceilings, not reservations: Go's memory limit is soft — the
// collector works harder as the heap nears it, and a single allocation larger
// than the limit still succeeds — so a limit keeps a build polite on a shared
// machine, it does not make a too-large site fit.
package resources

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// cgroupShare is the part of a cgroup memory limit taken when none is
// configured: the same headroom idea as Go's own CPU auto-fit, leaving the
// rest for the stacks, the runtime and the external encoders.
const cgroupShare = 0.9

// NoLimit is the `memory:` value that turns the cgroup auto-fit off.
const NoLimit = "none"

// Request is what the configuration and the command line asked for.
type Request struct {
	CPU int // 0 = no cap
	// Memory is a size ("512MiB"), NoLimit, or "" to fit a cgroup limit.
	Memory string
	// MemoryField names where Memory came from, for an error message.
	MemoryField string
	GCPercent   *int // nil = Go's default
}

// Limits is a resolved request: what the process will run under.
type Limits struct {
	CPU int // GOMAXPROCS cap, 0 = none
	// Memory is the soft limit in bytes, 0 = none; FromCgroup reports that it
	// was fitted to the container rather than configured.
	Memory     int64
	FromCgroup bool
	GCPercent  *int
}

// Resolve validates a request and fills the memory limit from the cgroup when
// none is configured.
func Resolve(r Request) (Limits, error) {
	if r.CPU < 0 {
		return Limits{}, fmt.Errorf("resources.cpu: %d is not a CPU count (0 means no cap)", r.CPU)
	}
	if r.GCPercent != nil && *r.GCPercent < -1 {
		return Limits{}, fmt.Errorf("resources.gc_percent: %d is below -1 (-1 turns the collector off)", *r.GCPercent)
	}
	l := Limits{CPU: r.CPU, GCPercent: r.GCPercent}
	memory := strings.TrimSpace(r.Memory)
	switch {
	case strings.EqualFold(memory, NoLimit):
	case memory == "":
		if limit, ok := CgroupMemoryLimit(); ok {
			l.Memory, l.FromCgroup = int64(float64(limit)*cgroupShare), true
		}
	default:
		n, err := ParseSize(memory)
		if err != nil || n <= 0 {
			field := r.MemoryField
			if field == "" {
				field = "resources.memory"
			}
			return Limits{}, fmt.Errorf("%s: %q is not a size (e.g. 512MiB, 2GiB, or %q)", field, memory, NoLimit)
		}
		l.Memory = n
	}
	return l, nil
}

// Apply puts the limits into effect for the whole process. A CPU cap above the
// machine's count is lowered to it: GOMAXPROCS beyond the CPUs buys nothing.
func (l Limits) Apply() {
	if l.CPU > 0 {
		runtime.GOMAXPROCS(min(l.CPU, runtime.NumCPU()))
	}
	if l.Memory > 0 {
		debug.SetMemoryLimit(l.Memory)
	}
	if l.GCPercent != nil {
		debug.SetGCPercent(*l.GCPercent)
	}
}

// Workers caps a worker pool at the CPU limit, so external encoders started
// one per worker stay inside it too. Without a CPU limit n is kept as given.
func (l Limits) Workers(n int) int {
	if l.CPU > 0 && n > l.CPU {
		return l.CPU
	}
	return n
}

// Set reports whether any limit is in effect.
func (l Limits) Set() bool {
	return l.CPU > 0 || l.Memory > 0 || l.GCPercent != nil
}

// String is the one startup line naming the effective limits.
func (l Limits) String() string {
	var parts []string
	if l.Memory > 0 {
		s := "🧠 memory limit " + FormatSize(l.Memory)
		if l.FromCgroup {
			s += " (90% of the cgroup)"
		}
		parts = append(parts, s)
	}
	if l.CPU > 0 {
		parts = append(parts, fmt.Sprintf("⚙️ %d CPU(s)", min(l.CPU, runtime.NumCPU())))
	}
	if l.GCPercent != nil {
		parts = append(parts, fmt.Sprintf("♻️ gc_percent %d", *l.GCPercent))
	}
	return strings.Join(parts, " · ")
}

// sizeUnits are the suffixes ParseSize knows, longest first so "MiB" is not
// read as "B".
var sizeUnits = []struct {
	suffix string
	mult   int64
}{
	{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
	{"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3}, {"B", 1},
}

// ParseSize reads sizes like "512MiB", "1GiB", "256MB" or a plain byte count.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	for _, u := range sizeUnits {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err != nil {
				return 0, err
			}
			return int64(n * float64(u.mult)), nil
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// FormatSize prints a byte count in binary units, one decimal when needed.
func FormatSize(n int64) string {
	for _, u := range sizeUnits[:3] {
		if n >= u.mult {
			v := float64(n) / float64(u.mult)
			if v == float64(int64(v)) {
				return fmt.Sprintf("%d %s", int64(v), u.suffix)
			}
			return fmt.Sprintf("%.1f %s", v, u.suffix)
		}
	}
	return fmt.Sprintf("%d B", n)
}
