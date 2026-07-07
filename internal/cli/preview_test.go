package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// newPreviewProject makes a minimal crofty project rooted at a temp dir and
// returns it. Only the .crofty marker is needed for the preview state paths and
// findProject to resolve.
func newPreviewProject(t *testing.T) *project.Project {
	t.Helper()
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, project.MarkerDir))
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
