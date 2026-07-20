package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The gate reads the project root, not dist/ — Hugo never copies Functions into
// the build output, so dist/ is where they can't be seen. It stops for every
// provider: none of them can run a worker.
func TestPartsGateBlocksWhatDistCannotSee(t *testing.T) {
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
			b := assembleBundle(root, filepath.Join(root, "dist"))
			for _, provider := range supportedProviders() {
				if err := partsGate(b, provider, false); err == nil {
					t.Errorf("%s: deploy should stop while live routes would be replaced", provider)
				}
				if err := partsGate(b, provider, true); err != nil {
					t.Errorf("%s: --static-only should let the deploy through, got %v", provider, err)
				}
			}
		})
	}
}

// A site without Functions must deploy exactly as before.
func TestPartsGatePassesPlainSite(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "content/posts/hello.md", "# hello")
	mustWrite(t, root, "dist/index.html", "<html></html>")

	if got := projectFunctions(root); len(got) != 0 {
		t.Fatalf("projectFunctions = %v, want none", got)
	}
	if err := partsGate(assembleBundle(root, filepath.Join(root, "dist")), "cloudflare", false); err != nil {
		t.Errorf("partsGate = %v, want nil", err)
	}
}

// An inert part is not a reason to stop: a plain host that ignores _headers
// serves the site without them, so nothing that was working goes offline. The
// provider warns about those itself.
func TestPartsGatePassesInertParts(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "index.html", "<html></html>")
	mustWrite(t, dist, "_headers", "/*\n  X-Test: 1")
	mustWrite(t, root, "_routes.json", `{"include":["/api/*"]}`)

	for _, provider := range supportedProviders() {
		if err := partsGate(assembleBundle(root, dist), provider, false); err != nil {
			t.Errorf("%s: partsGate = %v, want nil", provider, err)
		}
	}
}

// A directory named _worker.js (or a file named functions) is not an entry
// point; the gate must not fire on a name alone.
func TestPartsGateIgnoresMismatchedKinds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "_worker.js"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "functions", "not a directory")

	if got := projectFunctions(root); len(got) != 0 {
		t.Fatalf("projectFunctions = %v, want none", got)
	}
	if err := partsGate(assembleBundle(root, ""), "cloudflare", false); err != nil {
		t.Errorf("partsGate = %v, want nil", err)
	}
}
