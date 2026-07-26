package cli

import (
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// TestMain puts the whole package on an in-memory keychain before a single test
// runs. Several commands here reach for a credential, and on a developer's
// machine the real keychain is the one they use every day: a test that forgot to
// mock it would read — or overwrite — their own tokens. Doing it once, here, is
// the difference between a rule every new test has to remember and one it cannot
// break.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// freshKeychain empties the in-memory keychain for this test. The mock store is
// package-wide state, so a test that asserts what is (or is not) saved has to
// start from nothing rather than inherit what an earlier test wrote. This is
// about isolation; TestMain already took care of never touching the real one.
func freshKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInit()
}

// brokenKeychain makes every keychain call fail for the length of this test —
// the runner that has no keychain at all — and puts the working mock back
// afterwards, so the failure can't leak into whatever runs next.
func brokenKeychain(t *testing.T, reason string) {
	t.Helper()
	keyring.MockInitWithError(errors.New(reason))
	t.Cleanup(func() { keyring.MockInit() })
}

// mkProject makes dir a crofty project root. What marks a root is the config
// file crofty writes, not the .crofty/ directory (D-2), so fixtures must write
// it — a bare mkdir .crofty/ is exactly the non-project state.
func mkProject(t *testing.T, dir string) {
	t.Helper()
	if err := (&project.Project{Root: dir}).SaveConfig(&project.Config{Workspace: "test"}); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
