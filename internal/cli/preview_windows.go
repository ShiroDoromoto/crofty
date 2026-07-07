//go:build windows

package cli

import (
	"os"
	"syscall"
)

// Windows has no SIGTERM/SIGKILL and no signal-0 liveness probe, so these use
// the Win32 primitives directly. The preview wrapper can't receive a graceful
// stop here (Go delivers only Ctrl-C on Windows), but `crofty preview stop`
// still ends both the wrapper and hugo by pid, so nothing is left orphaned.
const (
	processQueryInformation = 0x0400
	stillActive             = 259
)

// processAlive reports whether a process with this pid is still running by
// opening it and checking its exit code.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// signalTerminate stops a process. Windows console apps can't be asked to shut
// down gracefully by another process without a shared console, so this is a
// hard terminate — hugo flushes nothing important for a dev server.
func signalTerminate(pid int) {
	if pid <= 0 {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// signalKill is the same hard terminate; Windows has no gentler-then-harder
// escalation to model.
func signalKill(pid int) {
	signalTerminate(pid)
}
