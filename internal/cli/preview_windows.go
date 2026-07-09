//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// Windows has no SIGTERM/SIGKILL and no signal-0 liveness probe, so these use
// the Win32 primitives directly. The preview wrapper can't receive a graceful
// stop here (Go delivers only Ctrl-C on Windows), but `crofty preview stop`
// still ends both the wrapper and hugo by pid, so nothing is left orphaned.
const (
	processQueryInformation = 0x0400
	stillActive             = 259

	// detachedProcess gives the child no console of its own, so it neither steals
	// this one nor flashes a window; createNewProcessGroup keeps a Ctrl-C in this
	// console from reaching it.
	detachedProcess = 0x00000008
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

// processName returns the executable name behind a pid, so a recycled pid can't
// be mistaken for the preview it once was.
func processName(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return filepath.Base(windows.UTF16ToString(buf[:size])), nil
}

// detachedSysProcAttr detaches a background preview from this console. It is the
// flag pair an agent would otherwise be reaching for `Start-Process` or
// `cmd /c start` to get — and getting wrong.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
