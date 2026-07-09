package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// A folder crofty may create is not reported as a wall, whether it exists
// already ('crofty init .') or is several levels below one that does.
func TestEnsureCreatable_Writable(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{root, filepath.Join(root, "Crofty", "my-site")} {
		if err := EnsureCreatable(dir); err != nil {
			t.Errorf("EnsureCreatable(%s) = %v, want nil", dir, err)
		}
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Errorf("the probe left something behind: %v %v", entries, err)
	}
}

// The wall is reported before anything is written, as a Denied naming the folder
// that refused and the ways on — one of which needs no permission at all.
func TestEnsureCreatable_Denied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only bit does not deny writes on Windows; ACLs do")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only folder")
	}
	root := t.TempDir()
	base := filepath.Join(root, "Documents")
	if err := os.Mkdir(base, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(base, 0o755) })

	site := filepath.Join(base, "Crofty", "my-site")
	err := EnsureCreatable(site)

	var d *access.Denied
	if !errors.As(err, &d) {
		t.Fatalf("EnsureCreatable = %v, want an access.Denied", err)
	}
	if !strings.Contains(d.Op, site) {
		t.Errorf("Op %q should name the site crofty was creating", d.Op)
	}
	if d.Path != base {
		t.Errorf("Path = %q, want the folder that refused (%s)", d.Path, base)
	}
	var free bool
	for _, c := range d.Choices {
		if c.Permission == "" && c.Command != "" {
			free = true
		}
	}
	if !free {
		t.Error("every way on demands a permission; the author can also just choose another folder")
	}
	if _, err := os.Stat(filepath.Join(base, "Crofty")); !os.IsNotExist(err) {
		t.Error("the probe created part of the site it said it could not create")
	}
}

// The site's own folder is the anchor when it exists; a file standing where the
// site would go is not a wall to ask permission for, it is a mistake to name.
func TestEnsureCreatable_FileInTheWay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-site")
	if err := os.WriteFile(path, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := EnsureCreatable(path)
	if err == nil {
		t.Fatal("EnsureCreatable = nil, want an error")
	}
	var d *access.Denied
	if errors.As(err, &d) {
		t.Errorf("a file in the way is not a permission wall: %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should name the path: %v", err)
	}
}
