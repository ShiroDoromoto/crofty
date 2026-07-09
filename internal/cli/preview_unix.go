//go:build !windows

package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// processAlive reports whether a process with this pid currently exists. Signal
// 0 performs the existence/permission check without delivering a signal; EPERM
// means the process is there but owned by someone else, which still counts.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// signalTerminate asks a process to shut down gracefully (SIGTERM).
func signalTerminate(pid int) {
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

// signalKill forces a process to stop (SIGKILL), the last resort when SIGTERM
// was ignored.
func signalKill(pid int) {
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// processName returns the program name of a running pid. `ps -o comm=` is the
// one spelling that works on both macOS (a full path) and Linux (a bare, and on
// old kernels truncated, name), so the caller matches on a substring. An empty
// answer means the process is gone.
func processName(pid int) (string, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no process %d", pid)
	}
	return filepath.Base(name), nil
}

// detachedSysProcAttr puts a detached preview in its own session, so a Control-C
// in the terminal that launched it — which goes to the whole foreground process
// group — doesn't reach it, and closing that terminal doesn't hang up on it.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
