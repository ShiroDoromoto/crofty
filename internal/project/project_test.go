package project

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// SaveConfig/LoadConfig must round-trip the full deploy config, including the
// SFTP/FTPS fields, so a configured destination survives across runs.
func TestConfigRoundTrip(t *testing.T) {
	proj := &Project{Root: t.TempDir()}
	want := &Config{
		Workspace: "ws123",
		Deploy: DeployConfig{
			Provider: "sftp",
			Host:     "example.com",
			Port:     2222,
			User:     "deploy",
			Path:     "/var/www/site",
			KeyPath:  "/home/me/.ssh/id_ed25519",
		},
	}
	if err := proj.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := proj.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Deploy, want.Deploy) {
		t.Errorf("deploy round-trip\n got %+v\nwant %+v", got.Deploy, want.Deploy)
	}
	if got.Workspace != want.Workspace {
		t.Errorf("workspace = %q; want %q", got.Workspace, want.Workspace)
	}
}

// A Cloudflare config (the common case) must still round-trip unchanged — the
// worker declarations included, since they are what a deploy checks the
// destination against and a dropped one turns into silence.
func TestConfigRoundTrip_Cloudflare(t *testing.T) {
	proj := &Project{Root: t.TempDir()}
	want := &Config{
		Workspace: "ws",
		Deploy: DeployConfig{Provider: "cloudflare", Project: "blog", Branch: "main", AccountID: "acc",
			Worker: WorkerConfig{CompatibilityDate: "2026-07-20", RequiredEnv: []string{"API_BASE", "SIGNING_KEY"}}},
	}
	if err := proj.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := proj.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Deploy, want.Deploy) {
		t.Errorf("deploy round-trip\n got %+v\nwant %+v", got.Deploy, want.Deploy)
	}
}

// The marker of a project root is the config file crofty writes, not the
// .crofty/ directory: a user who parks a binary at .crofty/bin/crofty.exe must
// not turn the parent folder into a "project" (D-2).
func TestIsProject(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		if IsProject(t.TempDir()) {
			t.Error("expected false for a bare directory")
		}
	})
	t.Run("marker dir without config", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, MarkerDir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if IsProject(dir) {
			t.Error("expected false: .crofty/ alone is not a project")
		}
	})
	t.Run("config present", func(t *testing.T) {
		dir := t.TempDir()
		if err := (&Project{Root: dir}).SaveConfig(&Config{Workspace: "ws"}); err != nil {
			t.Fatal(err)
		}
		if !IsProject(dir) {
			t.Error("expected true once config.json exists")
		}
	})
	t.Run("config is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, MarkerDir, ConfigFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if IsProject(dir) {
			t.Error("expected false: config.json must be a regular file")
		}
	})
}

// Find must walk up to a real project root, and must report a stray .crofty/
// (no config.json) as a distinct error rather than a bare "no project here".
func TestFind(t *testing.T) {
	t.Run("from a subdirectory", func(t *testing.T) {
		root := t.TempDir()
		if err := (&Project{Root: root}).SaveConfig(&Config{Workspace: "ws"}); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(root, "content", "posts")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		proj, err := Find(sub)
		if err != nil {
			t.Fatal(err)
		}
		// t.TempDir() can hand back a symlinked path (/var → /private/var on
		// macOS), so compare what the filesystem resolves to.
		if got, want := resolve(t, proj.Root), resolve(t, root); got != want {
			t.Errorf("root = %q; want %q", got, want)
		}
	})
	t.Run("stray marker dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, MarkerDir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Find(dir)
		var stray *StrayMarkerError
		if !errors.As(err, &stray) {
			t.Fatalf("err = %v; want *StrayMarkerError", err)
		}
		if got, want := resolve(t, stray.Dir), resolve(t, dir); got != want {
			t.Errorf("stray dir = %q; want %q", got, want)
		}
		// It wraps ErrNotFound: a stray marker is still "not a project".
		if !errors.Is(err, ErrNotFound) {
			t.Error("StrayMarkerError should wrap ErrNotFound")
		}
	})
	t.Run("config wins over a stray marker below it", func(t *testing.T) {
		root := t.TempDir()
		if err := (&Project{Root: root}).SaveConfig(&Config{Workspace: "ws"}); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(root, "vendor")
		if err := os.MkdirAll(filepath.Join(sub, MarkerDir), 0o755); err != nil {
			t.Fatal(err)
		}
		proj, err := Find(sub)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := resolve(t, proj.Root), resolve(t, root); got != want {
			t.Errorf("root = %q; want %q", got, want)
		}
	})
}

func resolve(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
