package access

import (
	"errors"
	"io/fs"
	"testing"
)

func pathErr(op, path string, err error) error {
	return &fs.PathError{Op: op, Path: path, Err: err}
}

// TestDeny_WrapsOnlyPermission: Deny is meant to be called on every write
// without a type switch first, so it must pass non-permission errors straight
// through (and keep nil as nil).
func TestDeny_WrapsOnlyPermission(t *testing.T) {
	if err := Deny("write it", "/x", nil); err != nil {
		t.Fatalf("nil error became %v", err)
	}

	other := pathErr("open", "/x", fs.ErrNotExist)
	if got := Deny("write it", "/x", other); got != other {
		t.Fatalf("a non-permission error was rewritten: %v", got)
	}

	denied := Deny("write it", "/x", pathErr("open", "/x", fs.ErrPermission))
	var d *Denied
	if !errors.As(denied, &d) {
		t.Fatalf("a permission error was not wrapped: %v", denied)
	}
	if !errors.Is(denied, fs.ErrPermission) {
		t.Error("Denied must unwrap to the OS error")
	}
}

// TestDeny_FillsPathFromError: a call site that doesn't know the path (the OS
// call built it) still gets one in the output.
func TestDeny_FillsPathFromError(t *testing.T) {
	err := Deny("write it", "", pathErr("mkdir", "/nope/deep", fs.ErrPermission))
	d, ok := From(err)
	if !ok {
		t.Fatal("not denied")
	}
	if d.Path != "/nope/deep" {
		t.Errorf("path = %q, want /nope/deep", d.Path)
	}
}

// TestFrom_PromotesBareOSError is the guard the whole task rests on: a call site
// nobody has wrapped yet must still surface as a permission branch, never as a
// bare "permission denied" an agent would route around.
func TestFrom_PromotesBareOSError(t *testing.T) {
	d, ok := From(pathErr("mkdir", "/etc/nope", fs.ErrPermission))
	if !ok {
		t.Fatal("a bare OS permission error was not recognized")
	}
	if d.Op != "mkdir" || d.Path != "/etc/nope" {
		t.Errorf("op/path = %q/%q, want mkdir//etc/nope", d.Op, d.Path)
	}
	if len(d.Choices) != 0 {
		t.Error("a promoted error must not invent choices")
	}

	if _, ok := From(errors.New("something else")); ok {
		t.Error("an unrelated error was reported as denied")
	}
}

// TestReason_DropsPathErrorPreamble: crofty prints the path on its own line, so
// the reason must not repeat it.
func TestReason_DropsPathErrorPreamble(t *testing.T) {
	if got := Reason(pathErr("open", "/x", fs.ErrPermission)); got != "permission denied" {
		t.Errorf("reason = %q, want the OS words alone", got)
	}
	if got := Reason(errors.New("plain")); got != "plain" {
		t.Errorf("reason = %q, want plain", got)
	}
}

// TestPayload_StatesNeedsPermission: --json must say outright which choices need
// the author, rather than leave a reader to infer it from an empty field.
func TestPayload_StatesNeedsPermission(t *testing.T) {
	d := &Denied{
		Op:   "write the design tokens",
		Path: "/site/assets/css/custom.css",
		Err:  pathErr("open", "/site/assets/css/custom.css", fs.ErrPermission),
		Choices: []Choice{
			{Do: "grant access", Command: "crofty theme eject", Permission: "write access to /site"},
			{Do: "print instead", Command: "crofty theme eject --print"},
		},
	}
	p := d.Payload()

	if p.Error != "permission_denied" {
		t.Errorf("error = %q", p.Error)
	}
	if p.Reason != "permission denied" {
		t.Errorf("reason = %q", p.Reason)
	}
	if p.AgentRule != AgentRule {
		t.Error("the payload must carry the norm the human text states")
	}
	if !p.Choices[0].NeedsPermission || p.Choices[0].Permission == "" {
		t.Error("choice 1 needs the author's permission and must say so")
	}
	if p.Choices[1].NeedsPermission {
		t.Error("choice 2 needs nothing from the author")
	}
}
