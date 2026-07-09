package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what was
// written. printNoProjectHere writes straight to os.Stderr, so this is how we
// observe it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	fn()
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestPrintNoProjectHere_FirstTimer: with no known projects, the dead end points
// a first-timer at `crofty init`.
func TestPrintNoProjectHere_FirstTimer(t *testing.T) {
	isolateHome(t)

	out := captureStderr(t, printNoProjectHere)

	if !strings.Contains(out, "no crofty project here yet") {
		t.Errorf("first-timer message missing; got:\n%s", out)
	}
	if !strings.Contains(out, "crofty init") {
		t.Errorf("expected a 'crofty init' next step; got:\n%s", out)
	}
	if strings.Contains(out, "cd ") {
		t.Errorf("first-timer should not be told to cd anywhere; got:\n%s", out)
	}
}

// TestPrintNoProjectHere_HasProject: someone who already has a project (e.g. just
// ran `crofty init` and forgot to cd) is pointed at it with a ready cd line, not
// told to `crofty init` as the headline step.
func TestPrintNoProjectHere_HasProject(t *testing.T) {
	isolateHome(t)

	base, err := project.DefaultBase()
	if err != nil {
		t.Fatal(err)
	}
	projRoot := filepath.Join(base, "myproj")
	mkdir(t, projRoot)
	mkProject(t, projRoot)

	out := captureStderr(t, printNoProjectHere)

	if !strings.Contains(out, "no crofty project in this folder") {
		t.Errorf("expected the 'wrong folder' message; got:\n%s", out)
	}
	if !strings.Contains(out, "cd "+projRoot) {
		t.Errorf("expected a ready 'cd %s' line; got:\n%s", projRoot, out)
	}
	if strings.Contains(out, "no crofty project here yet") {
		t.Errorf("should not show the first-timer message when a project exists; got:\n%s", out)
	}
}

// A .crofty/ with no config.json must not be reported as "no project here" —
// the folder looks like a project to the person standing in it, so the message
// names the half-state and the one command that resolves it (D-2).
func TestPrintStrayMarker(t *testing.T) {
	out := captureStderr(t, func() {
		printStrayMarker(&project.StrayMarkerError{Dir: "/tmp/site"})
	})
	for _, want := range []string{"/tmp/site", ".crofty/config.json", "crofty init ."} {
		if !strings.Contains(out, want) {
			t.Errorf("message should mention %q; got:\n%s", want, out)
		}
	}
}
