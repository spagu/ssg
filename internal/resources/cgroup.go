package resources

// The memory limit of the cgroup this process runs in (cgroup v2).
//
// Go fits GOMAXPROCS to a cgroup CPU limit by itself (1.25+), but nothing fits
// the heap to a memory limit: a container killed by the OOM killer at its
// limit dies without the garbage collector having tried harder first. Reading
// the limit lets the soft limit sit just under it.

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// The two files read, variables so a test can point them at a fake tree.
var (
	cgroupRoot     = "/sys/fs/cgroup"
	procSelfCgroup = "/proc/self/cgroup"
)

// CgroupMemoryLimit is the tightest memory.max on the path from this
// process's cgroup up to the root; ok is false when there is none ("max"
// everywhere, cgroup v1, or not Linux).
func CgroupMemoryLimit() (int64, bool) {
	dir, ok := ownCgroup()
	if !ok {
		return 0, false
	}
	var best int64
	for {
		if n, ok := readMemoryMax(filepath.Join(cgroupRoot, filepath.FromSlash(dir), "memory.max")); ok && (best == 0 || n < best) {
			best = n
		}
		if dir == "/" {
			break
		}
		dir = path.Dir(dir)
	}
	return best, best > 0
}

// ownCgroup is this process's cgroup v2 path, from the "0::" line.
func ownCgroup() (string, bool) {
	f, err := os.Open(procSelfCgroup)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if dir, ok := strings.CutPrefix(scanner.Text(), "0::"); ok {
			return path.Clean("/" + dir), true
		}
	}
	return "", false
}

// readMemoryMax reads one memory.max; "max" and anything unreadable mean no
// limit at that level.
func readMemoryMax(file string) (int64, bool) {
	raw, err := os.ReadFile(file) // #nosec G304 -- a fixed name under the cgroup mount
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
