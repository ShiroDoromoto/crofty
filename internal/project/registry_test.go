package project

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// CROFTY_HOME exists so a sandbox that refuses the OS config dir has a door
// other than rewriting %APPDATA%. It has to win over os.UserConfigDir.
func TestGlobalDirHonoursHomeEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(HomeEnv, dir)

	got, err := GlobalDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("GlobalDir() = %q, want %q", got, dir)
	}
}

// A relative CROFTY_HOME must land somewhere fixed: the registry is read from
// other directories, so a path that moves with the cwd would lose projects.
func TestGlobalDirMakesHomeEnvAbsolute(t *testing.T) {
	t.Setenv(HomeEnv, "state")

	got, err := GlobalDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("GlobalDir() = %q, want an absolute path", got)
	}
}

// Registering under CROFTY_HOME must be idempotent and readable back, since
// that is the whole point of the relocation.
func TestRegisterProjectUnderHomeEnv(t *testing.T) {
	t.Setenv(HomeEnv, t.TempDir())
	isolateDefaultBase(t)
	proj := newProjectDir(t)

	for i := 0; i < 2; i++ {
		if err := RegisterProject(proj); err != nil {
			t.Fatal(err)
		}
	}

	got := KnownProjects()
	if len(got) != 1 || got[0] != proj {
		t.Errorf("KnownProjects() = %v, want [%s]", got, proj)
	}
}

// A wall on the registry comes back as a Denied with the ways on, so the caller
// can show the author a choice instead of a bare OS error (D-1).
func TestRegisterProjectDeniesWithChoices(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not deny writes on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	home := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HomeEnv, filepath.Join(home, "crofty"))

	err := RegisterProject(newProjectDir(t))

	d, ok := access.From(err)
	if !ok {
		t.Fatalf("RegisterProject() = %v, want a permission wall", err)
	}
	if len(d.Choices) == 0 {
		t.Error("the wall offers no way on; an agent will invent one")
	}
}

// isolateDefaultBase points the home directory at a temp dir so KnownProjects'
// scan of DefaultBase cannot pick up the real sites on the machine running the
// test.
func isolateDefaultBase(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func newProjectDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".crofty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".crofty", "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
