package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ensureAgentsGuide must funnel an assistant to `crofty agent`, be idempotent,
// and never clobber an author's own AGENTS.md content.
func TestEnsureAgentsGuide(t *testing.T) {
	t.Run("creates the file when absent", func(t *testing.T) {
		dir := t.TempDir()
		status, err := ensureAgentsGuide(dir)
		if err != nil || status != guideCreated {
			t.Fatalf("status=%v err=%v, want guideCreated nil", status, err)
		}
		got := readGuide(t, dir)
		if !strings.Contains(got, "crofty agent") {
			t.Errorf("guide does not point at `crofty agent`:\n%s", got)
		}
		if !strings.Contains(got, agentsBeginMark) || !strings.Contains(got, agentsEndMark) {
			t.Errorf("guide is not fenced by the managed-block markers:\n%s", got)
		}
	})

	t.Run("is idempotent — a managed file is left as guidePresent", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ensureAgentsGuide(dir); err != nil {
			t.Fatal(err)
		}
		first := readGuide(t, dir)
		status, err := ensureAgentsGuide(dir)
		if err != nil {
			t.Fatal(err)
		}
		if status != guidePresent {
			t.Errorf("status=%v on a managed file, want guidePresent", status)
		}
		if got := readGuide(t, dir); got != first {
			t.Errorf("second run altered the file:\n%s", got)
		}
	})

	t.Run("never touches an author's own file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, agentsFileName)
		authorText := "# My notes\n\nUse two spaces for indent.\n"
		if err := os.WriteFile(path, []byte(authorText), 0o644); err != nil {
			t.Fatal(err)
		}

		status, err := ensureAgentsGuide(dir)
		if err != nil || status != guideForeign {
			t.Fatalf("status=%v err=%v, want guideForeign nil", status, err)
		}
		if got := readGuide(t, dir); got != authorText {
			t.Errorf("author's file was modified:\n%s", got)
		}
	})
}

func readGuide(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, agentsFileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
