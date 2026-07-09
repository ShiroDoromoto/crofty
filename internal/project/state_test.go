package project

import (
	"errors"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// The plain case: crofty names its state directory and says it may write there.
func TestState_Writable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(HomeEnv, dir)

	s, err := State()
	if err != nil {
		t.Fatalf("State() = %v", err)
	}
	if s.Dir != dir {
		t.Errorf("Dir = %q, want %q", s.Dir, dir)
	}
	if !s.FromEnv {
		t.Errorf("FromEnv = false, want true: %s chose the directory", HomeEnv)
	}
	if !s.Writable() {
		t.Errorf("Writable() = false over a writable temp dir: %v", s.Err)
	}
}

// A wall comes back as the wall init would have shown, ways on and all — the
// point of asking before writing (#25). It is reported, never returned as the
// call's error: a registry crofty cannot write costs the author discovery, and
// nothing else (#13).
func TestState_WallCarriesTheChoices(t *testing.T) {
	t.Setenv(HomeEnv, readOnlyDir(t))

	s, err := State()
	if err != nil {
		t.Fatalf("State() = %v, want the wall in the status, not an error", err)
	}
	if s.Writable() {
		t.Fatal("Writable() = true over a read-only directory")
	}
	var d *access.Denied
	if !errors.As(s.Err, &d) {
		t.Fatalf("Err = %v, want an *access.Denied", s.Err)
	}
	if len(d.Choices) != 2 {
		t.Fatalf("choices = %d, want %s and giving up on discovery", len(d.Choices), HomeEnv)
	}
	if d.Choices[0].Permission == "" {
		t.Error("the first choice needs the author's permission and must say so")
	}
}
