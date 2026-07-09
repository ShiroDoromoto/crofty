package cli

import (
	"os"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

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
