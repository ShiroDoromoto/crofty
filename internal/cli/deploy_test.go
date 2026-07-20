package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The gate reads the project root, not dist/ — Hugo never copies Functions into
// the build output, so dist/ is where they can't be seen. A functions/ tree
// stops every provider, because carrying it would mean bundling it; a worker
// stops the ones that can't run one.
func TestPartsGateBlocksWhatDistCannotSee(t *testing.T) {
	for _, tc := range []struct {
		name    string
		make    func(root string)
		want    string
		carried []string // providers that take this part rather than stop
	}{
		{"functions dir", func(root string) {
			if err := os.MkdirAll(filepath.Join(root, "functions", "api"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "functions/", nil},
		{"_worker.js", func(root string) {
			mustWrite(t, root, "_worker.js", "export default {}")
		}, "_worker.js", []string{"cloudflare"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.make(root)

			if got := projectFunctions(root); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("projectFunctions = %v, want [%s]", got, tc.want)
			}
			carries := map[string]bool{}
			for _, p := range tc.carried {
				carries[p] = true
			}
			b := assembleBundle(root, filepath.Join(root, "dist"))
			for _, provider := range supportedProviders() {
				err := partsGate(b, provider, false)
				if carries[provider] && err != nil {
					t.Errorf("%s: carries this part, so the deploy should go on, got %v", provider, err)
				}
				if !carries[provider] && err == nil {
					t.Errorf("%s: deploy should stop while live routes would be replaced", provider)
				}
				if err := partsGate(b, provider, true); err != nil {
					t.Errorf("%s: --static-only should let the deploy through, got %v", provider, err)
				}
			}
		})
	}
}

// crofty carries a finished worker, not a build of one: a worker that imports
// stops the deploy where it can still be read, rather than uploading cleanly and
// failing on the first request. --static-only is the way past — it isn't going
// anywhere, so it needn't be carryable.
func TestWorkerGateStopsAnImportingWorker(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "_worker.js", "import { r } from './router.js'\nexport default {}")
	b := assembleBundle(root, filepath.Join(root, "dist"))

	if err := partsGate(b, "cloudflare", false); err != nil {
		t.Fatalf("partsGate = %v — the worker is carried, so this gate has no say", err)
	}
	if err := workerGate(b, "cloudflare", false); err == nil {
		t.Error("a worker with an import should stop the deploy")
	}
	if err := workerGate(b, "cloudflare", true); err != nil {
		t.Errorf("--static-only: workerGate = %v, want nil", err)
	}
	// A destination that can't take a worker at all has already stopped in
	// partsGate; this gate must not speak over it with the wrong reason.
	if err := workerGate(b, "sftp", false); err != nil {
		t.Errorf("sftp: workerGate = %v, want nil (partsGate owns that refusal)", err)
	}
}

// A self-contained worker is carried, and that is the whole point of the change.
func TestWorkerGatePassesSelfContainedWorker(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "_worker.js", "export default { fetch: () => new Response('ok') }")
	b := assembleBundle(root, filepath.Join(root, "dist"))

	if err := workerGate(b, "cloudflare", false); err != nil {
		t.Errorf("workerGate = %v, want nil", err)
	}
}

// --static-only means the live parts stay home, even the ones this destination
// could have carried — otherwise the flag would quietly change meaning the day
// crofty learned to deliver one.
func TestStaticOnlyDropsCarriableLiveParts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "_worker.js", "export default {}")
	mustWrite(t, root, "_routes.json", `{"include":["/api/*"]}`)
	mustWrite(t, root, "dist/_headers", "/*\n  X-Test: 1")

	b := assembleBundle(root, filepath.Join(root, "dist")).withoutLiveParts()
	if _, ok := b.parts[partWorker]; ok {
		t.Error("_worker.js should not travel with --static-only")
	}
	if _, ok := b.parts[partHeaders]; !ok {
		t.Error("_headers is inert and should still travel")
	}
	// _routes.json is inert on its own; the Cloudflare deploy leaves it out when
	// there is no worker to route to.
	if _, ok := b.parts[partRoutes]; !ok {
		t.Error("_routes.json should survive the drop as an inert part")
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
