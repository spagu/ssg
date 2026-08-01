package main

import (
	"runtime"
	"testing"
)

// TestResolveBuildWorkers pins the --workers semantics: unset ⇒ one per CPU,
// an explicit 0 (or negative) ⇒ off/sequential (1), N ⇒ exactly N.
func TestResolveBuildWorkers(t *testing.T) {
	if got := resolveBuildWorkers(nil); got != runtime.NumCPU() {
		t.Errorf("unset = %d, want NumCPU %d", got, runtime.NumCPU())
	}
	zero := 0
	if got := resolveBuildWorkers(&zero); got != 1 {
		t.Errorf("explicit 0 (off) = %d, want 1", got)
	}
	neg := -3
	if got := resolveBuildWorkers(&neg); got != 1 {
		t.Errorf("negative = %d, want 1", got)
	}
	for _, n := range []int{1, 2, 5, 10} {
		v := n
		if got := resolveBuildWorkers(&v); got != n {
			t.Errorf("explicit %d = %d, want exactly %d", n, got, n)
		}
	}
}
