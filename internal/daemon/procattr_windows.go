//go:build windows

package daemon

// Stopping a project on Windows, which has no process groups to signal and no
// SIGTERM to send. Kill is the only ending available, so the grace period costs
// nothing here and the behaviour is honest rather than pretended.

import "os/exec"

// isolateProcess is a no-op: there is no process group to create.
func isolateProcess(*exec.Cmd) {}

// terminateProcess has no graceful form on Windows; killProcess does the work.
func terminateProcess(*exec.Cmd) {}

// killProcess ends the project.
func killProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
