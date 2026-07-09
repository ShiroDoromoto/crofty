package cli

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// newPreviewProject makes a minimal crofty project rooted at a temp dir and
// returns it. Only the config file is needed for the preview state paths and
// findProject to resolve.
func newPreviewProject(t *testing.T) *project.Project {
	t.Helper()
	dir := t.TempDir()
	mkProject(t, dir)
	return &project.Project{Root: dir}
}

func TestPreviewStateRoundTrip(t *testing.T) {
	proj := newPreviewProject(t)
	path := previewStatePath(proj)

	// Absent state reads as (nil, nil) — not an error — so callers can treat "no
	// preview recorded" and "no preview running" the same way.
	if st, err := readPreviewState(path); err != nil || st != nil {
		t.Fatalf("readPreviewState(absent) = %v, %v; want nil, nil", st, err)
	}

	want := &previewState{CroftyPID: 4242, HugoPID: 4243, Port: 1313, URL: "http://localhost:1313/", StartedAt: "2026-07-07T00:00:00Z"}
	if err := writePreviewState(path, want); err != nil {
		t.Fatalf("writePreviewState: %v", err)
	}
	got, err := readPreviewState(path)
	if err != nil {
		t.Fatalf("readPreviewState: %v", err)
	}
	if *got != *want {
		t.Fatalf("round-trip = %+v, want %+v", *got, *want)
	}

	removePreviewState(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("state file still present after remove: %v", err)
	}
	// Removing an already-absent file must be a silent no-op.
	removePreviewState(path)
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("processAlive(self) = false; want true")
	}
	if processAlive(0) || processAlive(-1) {
		t.Error("processAlive(<=0) = true; want false")
	}
	// A pid that is exceedingly unlikely to exist must read as not alive rather
	// than being reported running (which would wedge the singleton reap).
	if processAlive(0x7ffffffe) {
		t.Error("processAlive(bogus pid) = true; want false")
	}
}

func TestPreviewStatusNotRunning(t *testing.T) {
	proj := newPreviewProject(t)
	t.Chdir(proj.Root)

	// A stale state file (recorded pid no longer alive) must report not-running
	// AND be tidied away, so the next status is honest and fast.
	stale := &previewState{CroftyPID: 0x7ffffffe, HugoPID: 0x7ffffffd, Port: 1313, URL: "http://localhost:1313/"}
	if err := writePreviewState(previewStatePath(proj), stale); err != nil {
		t.Fatal(err)
	}

	var rep previewReport
	out, _ := captureOutput(t, func() {
		if err := runPreviewStatus([]string{"--json"}); err != nil {
			t.Fatalf("runPreviewStatus: %v", err)
		}
	})
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("status --json not valid JSON: %v\n%s", err, out)
	}
	if rep.Running {
		t.Errorf("status.running = true for a dead pid; want false")
	}
	if _, err := os.Stat(previewStatePath(proj)); !os.IsNotExist(err) {
		t.Errorf("stale state file not cleaned up by status")
	}
}

// A detached start waits for the port to answer rather than for a clock, so the
// probe must say "yes" only while something is actually listening.
func TestPreviewPortAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	if !previewPortAnswers(port) {
		t.Errorf("previewPortAnswers(%d) = false while listening; want true", port)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if previewPortAnswers(port) {
		t.Errorf("previewPortAnswers(%d) = true after close; want false", port)
	}
}

// A squatter on the port would answer the readiness probe while our own hugo
// dies unseen, so a detached start has to refuse a taken port up front.
func TestPreviewPortFree(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	err = previewPortFree(port)
	if err == nil {
		t.Fatalf("previewPortFree(%d) = nil while the port is taken; want an error", port)
	}
	if !strings.Contains(err.Error(), "already in use") || !strings.Contains(err.Error(), "--port") {
		t.Errorf("error must name the problem and the way out; got: %v", err)
	}

	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if err := previewPortFree(port); err != nil {
		t.Errorf("previewPortFree(%d) after close = %v; want nil", port, err)
	}
}

func TestIndentedLogTail(t *testing.T) {
	proj := newPreviewProject(t)
	path := previewLogPath(proj)

	// No log at all is the case where hugo never even started; it must still read
	// as prose rather than blowing up the error message.
	if got := indentedLogTail(path, 3); !strings.Contains(got, "no log") {
		t.Errorf("indentedLogTail(missing) = %q", got)
	}

	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := indentedLogTail(path, 2)
	if strings.Contains(got, "two") {
		t.Errorf("tail kept a line beyond the last 2: %q", got)
	}
	for _, want := range []string{"  three", "  four"} {
		if !strings.Contains(got, want) {
			t.Errorf("tail = %q; want it to contain %q (indented)", got, want)
		}
	}
}

func TestPreviewStopIdempotent(t *testing.T) {
	proj := newPreviewProject(t)
	t.Chdir(proj.Root)

	// Stopping when nothing is running is a success, not an error — an agent
	// calls it unconditionally when it's done showing the author.
	out, _ := captureOutput(t, func() {
		if err := runPreviewStop(nil); err != nil {
			t.Fatalf("runPreviewStop(none): %v", err)
		}
	})
	if !strings.Contains(out, "No preview is running") {
		t.Errorf("stop with none running said: %q", out)
	}
}
