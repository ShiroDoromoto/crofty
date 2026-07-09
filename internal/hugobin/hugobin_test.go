package hugobin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExec creates an executable file at path, making its parents as needed.
func writeExec(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// bundledLayout builds an install tree for goos and returns crofty's path in it.
func bundledLayout(t *testing.T, goos string) string {
	t.Helper()
	root := t.TempDir()
	if goos == "windows" {
		writeExec(t, filepath.Join(root, "bin", "hugo.exe"))
		return filepath.Join(root, "bin", "crofty.exe")
	}
	writeExec(t, filepath.Join(root, "libexec", "crofty", "hugo"))
	return filepath.Join(root, "bin", "crofty")
}

func TestResolvePrefersOverride(t *testing.T) {
	mine := writeExec(t, filepath.Join(t.TempDir(), "my-hugo"))
	exe := bundledLayout(t, "darwin") // a bundled copy exists and must lose

	got, err := resolve(mine, exe, "darwin")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != mine {
		t.Errorf("resolve = %q, want the override %q", got, mine)
	}
}

// A broken override must fail loudly. Falling through to another Hugo would
// leave the author believing crofty runs the one they named.
func TestResolveRejectsBrokenOverride(t *testing.T) {
	exe := bundledLayout(t, "darwin")
	gone := filepath.Join(t.TempDir(), "not-there")

	_, err := resolve(gone, exe, "darwin")
	if err == nil {
		t.Fatal("resolve: want an error for an override that does not exist, got nil")
	}
	if !strings.Contains(err.Error(), EnvOverride) {
		t.Errorf("error %q does not name %s, so the author cannot tell what to fix", err, EnvOverride)
	}
}

func TestResolveFindsBundled(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			exe := bundledLayout(t, goos)
			// A hugo on PATH must not win over the bundled copy.
			t.Setenv("PATH", filepath.Dir(writeExec(t, filepath.Join(t.TempDir(), "hugo"))))

			got, err := resolve("", exe, goos)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if want := bundled(exe, goos); got != want {
				t.Errorf("resolve = %q, want the bundled copy %q", got, want)
			}
		})
	}
}

// Without a bundled copy — how brew, apt and the install script all leave it —
// crofty falls back to PATH.
func TestResolveFallsBackToPath(t *testing.T) {
	onPath := writeExec(t, filepath.Join(t.TempDir(), "hugo"))
	t.Setenv("PATH", filepath.Dir(onPath))

	got, err := resolve("", filepath.Join(t.TempDir(), "bin", "crofty"), "darwin")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != onPath {
		t.Errorf("resolve = %q, want the hugo on PATH %q", got, onPath)
	}
}

func TestResolveReportsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := resolve("", filepath.Join(t.TempDir(), "bin", "crofty"), "darwin")
	if err == nil {
		t.Fatal("resolve: want an error when no hugo exists anywhere, got nil")
	}
	// The author is told how to install one, and how to name one crofty missed.
	for _, want := range []string{"gohugo.io/installation", EnvOverride} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A directory named hugo is not a Hugo.
func TestExecutableRejectsDirectory(t *testing.T) {
	if executable(t.TempDir(), "darwin") {
		t.Error("executable: a directory must not pass")
	}
}

func TestExecutableRejectsNonExecutableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hugo")
	if err := os.WriteFile(path, []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	if executable(path, "darwin") {
		t.Error("executable: a file without an execute bit must not pass")
	}
	// Windows has no execute bit; the name is what carries the meaning there.
	if !executable(path, "windows") {
		t.Error("executable: a regular file must pass on windows")
	}
}
