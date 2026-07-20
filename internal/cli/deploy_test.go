package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The gate reads the project root, not dist/ — Hugo never copies Functions into
// the build output, so dist/ is where they can't be seen.
func TestFunctionsGateBlocksWhatDistCannotSee(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(root string)
		want string
	}{
		{"functions dir", func(root string) {
			if err := os.MkdirAll(filepath.Join(root, "functions", "api"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "functions/"},
		{"_worker.js", func(root string) {
			mustWrite(t, root, "_worker.js", "export default {}")
		}, "_worker.js"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.make(root)

			if got := projectFunctions(root); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("projectFunctions = %v, want [%s]", got, tc.want)
			}
			if err := functionsGate(root, false); err == nil {
				t.Error("deploy should stop while live Functions would be replaced")
			}
			if err := functionsGate(root, true); err != nil {
				t.Errorf("--static-only should let the deploy through, got %v", err)
			}
		})
	}
}

// A site without Functions must deploy exactly as before.
func TestFunctionsGatePassesPlainSite(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "content/posts/hello.md", "# hello")
	mustWrite(t, root, "dist/index.html", "<html></html>")

	if got := projectFunctions(root); len(got) != 0 {
		t.Fatalf("projectFunctions = %v, want none", got)
	}
	if err := functionsGate(root, false); err != nil {
		t.Errorf("functionsGate = %v, want nil", err)
	}
}

// A directory named _worker.js (or a file named functions) is not an entry
// point; the gate must not fire on a name alone.
func TestFunctionsGateIgnoresMismatchedKinds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "_worker.js"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "functions", "not a directory")

	if got := projectFunctions(root); len(got) != 0 {
		t.Fatalf("projectFunctions = %v, want none", got)
	}
}
