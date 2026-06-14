// Package runner shells out to external tools (Hugo). Output is streamed
// straight to the user's terminal rather than captured, so crofty never buffers
// tokens or secrets that pass through those tools.
package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Run executes name+args in dir, streaming stdio to the user. It returns an
// error if the tool is missing or exits non-zero.
func Run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Capture runs name+args in dir and returns combined output without streaming.
// Use for idempotent setup steps whose expected failures (e.g. "already exists")
// should not clutter the terminal.
func Capture(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// CaptureEnv is Capture with extra environment entries ("KEY=value") appended
// to the inherited environment.
func CaptureEnv(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// RunTee streams name+args output to the user (like Run) while also capturing
// stdout, so a caller can parse a result — e.g. a deploy URL — without hiding
// progress. Extra env entries ("KEY=value") are appended to the environment.
func RunTee(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("%s: %w", name, err)
	}
	return buf.String(), nil
}

// Look reports whether a tool is on PATH.
func Look(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
