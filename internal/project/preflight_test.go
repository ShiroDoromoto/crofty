package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// readOnlyDir is a folder nobody but root may write into. The tests that need
// one are about permission bits, which Windows does not enforce this way.
func readOnlyDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a read-only bit does not deny writes on Windows; ACLs do")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only folder")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return dir
}

// Nothing to ask for: init proceeds without a word.
func TestPreflightInit_Clear(t *testing.T) {
	t.Setenv(HomeEnv, t.TempDir())
	if err := PreflightInit(filepath.Join(t.TempDir(), "my-site")); err != nil {
		t.Errorf("PreflightInit = %v, want nil", err)
	}
}

// A wall on crofty's own state is not a wall on the site. init writes the site
// and says afterwards what it could not record (#13), so the preflight must not
// stop it — nor ask for a permission crofty is about to prove it does not need.
func TestPreflightInit_StateWallAloneIsSilent(t *testing.T) {
	t.Setenv(HomeEnv, readOnlyDir(t))
	if err := PreflightInit(filepath.Join(t.TempDir(), "my-site")); err != nil {
		t.Errorf("PreflightInit = %v, want nil: a site crofty cannot register is still a site", err)
	}
}

// Both walls: the author is asked for both permissions at once, rather than
// granting one, re-running, and meeting the next.
func TestPreflightInit_BothWallsComeBackTogether(t *testing.T) {
	t.Setenv(HomeEnv, readOnlyDir(t))
	site := filepath.Join(readOnlyDir(t), "Crofty", "my-site")

	err := PreflightInit(site)

	var walls access.Denials
	if !errors.As(err, &walls) {
		t.Fatalf("PreflightInit = %v, want access.Denials", err)
	}
	if len(walls) != 2 {
		t.Fatalf("walls = %d, want the site and the state", len(walls))
	}
}

// One wall comes back as one wall, not a list of one.
func TestPreflightInit_SiteWallAlone(t *testing.T) {
	t.Setenv(HomeEnv, t.TempDir())
	site := filepath.Join(readOnlyDir(t), "Crofty", "my-site")

	err := PreflightInit(site)

	var walls access.Denials
	if errors.As(err, &walls) {
		t.Fatalf("PreflightInit = %v, want a single *access.Denied", err)
	}
	var d *access.Denied
	if !errors.As(err, &d) {
		t.Fatalf("PreflightInit = %v, want an access.Denied", err)
	}
}
