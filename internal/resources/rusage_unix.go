//go:build unix

package resources

import (
	"runtime"
	"syscall"
)

// PeakRSS is the most resident memory the process has held, in bytes, or 0
// when the system cannot say. getrusage reports kilobytes everywhere but on
// Apple systems, which report bytes.
func PeakRSS() int64 {
	var ru syscall.Rusage
	// RUSAGE_SELF on the calling process cannot fail; were it to, Maxrss
	// stays 0 and so does the answer.
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return maxRSSBytes(int64(ru.Maxrss), runtime.GOOS)
}

// maxRSSBytes converts ru_maxrss to bytes for the given system.
func maxRSSBytes(v int64, goos string) int64 {
	if goos == "darwin" || goos == "ios" {
		return v
	}
	return v * 1024
}
