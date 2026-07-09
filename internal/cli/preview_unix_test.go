//go:build !windows

package cli

import (
	"os/exec"
	"testing"
)

// startStranger runs a process that is neither crofty nor hugo — a stand-in for
// the unrelated program the OS has since handed a recorded pid to.
func startStranger(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd.Process.Pid
}

// The pids in preview.json were written long ago. A live process at one of those
// numbers proves nothing on its own, so neither half may read as the preview.
func TestPreviewAliveIgnoresARecycledPID(t *testing.T) {
	pid := startStranger(t)
	if w, h := previewAlive(&previewState{CroftyPID: pid, HugoPID: pid}); w || h {
		t.Errorf("previewAlive(recycled pid) = (%v, %v); want (false, false)", w, h)
	}
}

// Killing by a stale pid would kill a stranger. Leaving a hugo running is the
// lesser harm, so a name that doesn't match means hands off.
func TestTerminatePIDRefusesAStranger(t *testing.T) {
	pid := startStranger(t)
	terminatePID(pid, "hugo")
	if !processAlive(pid) {
		t.Fatal("terminatePID killed a process whose name did not match; a recycled pid must be left alone")
	}
}
