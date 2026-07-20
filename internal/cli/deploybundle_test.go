package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// assembleBundle must collect the parts that sit at the root of the build, and
// only those: a file with the same name deeper in the tree is an ordinary asset.
func TestAssembleBundleFromBuild(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "index.html", "home")
	mustWrite(t, dist, "_headers", "/*\n  X-Test: 1")
	mustWrite(t, dist, "docs/_redirects", "/a /b 301")

	b := assembleBundle(root, dist)
	if b.assetsDir != dist {
		t.Errorf("assetsDir = %q, want %q", b.assetsDir, dist)
	}
	if want := filepath.Join(dist, "_headers"); b.parts[partHeaders] != want {
		t.Errorf("_headers part = %q, want %q", b.parts[partHeaders], want)
	}
	if p, ok := b.parts[partRedirects]; ok {
		t.Errorf("a nested _redirects is an asset, not a part (got %q)", p)
	}
}

// The parts Hugo never emits are collected from the project root — the one place
// they can be seen — and not from the build beside them.
func TestAssembleBundleFromRoot(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "index.html", "home")
	mustWrite(t, root, "_worker.js", "export default {}")
	mustWrite(t, root, "_routes.json", `{"include":["/api/*"]}`)
	if err := os.MkdirAll(filepath.Join(root, "functions", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A same-named file inside the build is not the root part.
	mustWrite(t, dist, "_worker.js", "export default {}")

	b := assembleBundle(root, dist)
	for _, p := range []deployPart{partWorker, partRoutes, partFunctions} {
		want := filepath.Join(root, string(p))
		if b.parts[p] != want {
			t.Errorf("%s part = %q, want %q", p, b.parts[p], want)
		}
	}
}

// The gate assembles before Hugo has run, so a missing build must not be an
// error — the parts that can stop a deploy are all at the root anyway.
func TestAssembleBundleWithoutBuild(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "_worker.js", "export default {}")

	b := assembleBundle(root, filepath.Join(root, "dist"))
	if _, ok := b.parts[partWorker]; !ok {
		t.Error("the root _worker.js should be found with no build present")
	}
	if len(b.parts) != 1 {
		t.Errorf("parts = %+v, want only the root worker", b.parts)
	}
}

// A build with no special files still assembles — plain assets, no parts.
func TestAssembleBundleNoParts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "dist/index.html", "home")

	b := assembleBundle(root, filepath.Join(root, "dist"))
	if len(b.parts) != 0 {
		t.Errorf("parts = %+v, want none", b.parts)
	}
}

// A name alone is not a part: a directory called _worker.js, or a file called
// functions, is neither an entry point nor something to stop a deploy over.
func TestAssembleBundleChecksShape(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "_worker.js"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "functions", "not a directory")

	if b := assembleBundle(root, ""); len(b.parts) != 0 {
		t.Errorf("parts = %+v, want none", b.parts)
	}
}

// A symlink is not a part: the asset walks skip symlinks, so treating one as a
// part here would smuggle it into a deployment by a different door.
func TestAssembleBundleIgnoresSymlink(t *testing.T) {
	dist := t.TempDir()
	mustWrite(t, dist, "real", "/*\n  X-Test: 1")
	if err := os.Symlink(filepath.Join(dist, "real"), filepath.Join(dist, "_headers")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if b := assembleBundle(t.TempDir(), dist); len(b.parts) != 0 {
		t.Errorf("parts = %+v, want none (a symlink is not a part)", b.parts)
	}
}

// partsNotCarried is how a provider learns what it would otherwise drop in
// silence; livePartsNotCarried narrows that to what is worth stopping over.
func TestPartsNotCarried(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "_headers", "/*\n  X-Test: 1")
	mustWrite(t, dist, "_redirects", "/a /b 301")
	mustWrite(t, root, "_worker.js", "export default {}")
	b := assembleBundle(root, dist)

	// Cloudflare takes all three: the two inert parts and the worker.
	if got := b.partsNotCarried(providerCarries("cloudflare")); len(got) != 0 {
		t.Errorf("cloudflare parts not carried = %v, want none", got)
	}
	want := []deployPart{partHeaders, partRedirects, partWorker}
	if got := b.partsNotCarried(providerCarries("sftp")); !reflect.DeepEqual(got, want) {
		t.Errorf("sftp parts not carried = %v, want %v", got, want)
	}
	// Only the worker answers requests: dropping _headers costs nothing that
	// was working, so it must not be a reason to stop a deploy. And a plain file
	// store can't run one, which is exactly what is worth stopping over.
	for _, provider := range []string{"sftp", "ftps"} {
		if got := b.livePartsNotCarried(providerCarries(provider)); !reflect.DeepEqual(got, []deployPart{partWorker}) {
			t.Errorf("%s live parts not carried = %v, want [%v]", provider, got, partWorker)
		}
	}
	if got := b.livePartsNotCarried(providerCarries("cloudflare")); len(got) != 0 {
		t.Errorf("cloudflare live parts not carried = %v, want none — it carries the worker", got)
	}
}

// Every Deployer's own declaration must agree with the one the gate reads from
// the provider name — they are the same answer at two different moments.
func TestCarriesMatchesProviderCarries(t *testing.T) {
	byProvider := map[string]Deployer{
		"cloudflare": &cloudflareDeployer{},
		"sftp":       &sftpDeployer{},
		"ftps":       &ftpsDeployer{},
	}
	for _, p := range supportedProviders() {
		d, ok := byProvider[p]
		if !ok {
			t.Fatalf("provider %q has no Deployer in this test — add it", p)
		}
		if got, want := d.Carries(), providerCarries(p); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Carries() = %v, providerCarries = %v", p, got, want)
		}
	}
}

// A part is written the way the user sees it on disk: a directory keeps its
// slash, which is also what `crofty config --json` reports.
func TestPartLabels(t *testing.T) {
	got := partLabels([]deployPart{partFunctions, partWorker})
	want := []string{"functions/", "_worker.js"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("partLabels = %v, want %v", got, want)
	}
}
