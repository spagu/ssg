//go:build !windows

package daemon

// Stopping a project on a platform with process groups.
//
// A watched project can have a child of its own — `watch_runner` starts one —
// and signalling only the parent leaves that child holding the port the next
// start needs. So the project gets its own process group and the group is what
// is signalled.

import (
	"os/exec"
	"syscall"
)

// isolateProcess puts the project in its own process group, so stopping it
// cannot signal the daemon or its siblings.
func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcess asks the whole group to leave.
func terminateProcess(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

// killProcess insists, once the grace period is spent.
func killProcess(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
