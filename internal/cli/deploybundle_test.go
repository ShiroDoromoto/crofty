package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// assembleBundle must collect the parts that sit at the root of a build, and
// only those: a file with the same name deeper in the tree is an ordinary asset.
func TestAssembleBundle(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "home")
	mustWrite(t, dir, "_headers", "/*\n  X-Test: 1")
	mustWrite(t, dir, "docs/_redirects", "/a /b 301")

	b := assembleBundle(dir)
	if b.assetsDir != dir {
		t.Errorf("assetsDir = %q, want %q", b.assetsDir, dir)
	}
	if want := filepath.Join(dir, "_headers"); b.parts[partHeaders] != want {
		t.Errorf("_headers part = %q, want %q", b.parts[partHeaders], want)
	}
	if p, ok := b.parts[partRedirects]; ok {
		t.Errorf("a nested _redirects is an asset, not a part (got %q)", p)
	}
}

// A build with no special files still assembles — plain assets, no parts.
func TestAssembleBundleNoParts(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "home")

	b := assembleBundle(dir)
	if len(b.parts) != 0 {
		t.Errorf("parts = %+v, want none", b.parts)
	}
}

// A symlink is not a part: the asset walks skip symlinks, so treating one as a
// part here would smuggle it into a deployment by a different door.
func TestAssembleBundleIgnoresSymlink(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "real", "/*\n  X-Test: 1")
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "_headers")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if b := assembleBundle(dir); len(b.parts) != 0 {
		t.Errorf("parts = %+v, want none (a symlink is not a part)", b.parts)
	}
}

// partsNotCarried is how a provider learns what it would otherwise drop in
// silence — the common layer assembles without knowing the destination.
func TestPartsNotCarried(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "_headers", "/*\n  X-Test: 1")
	mustWrite(t, dir, "_redirects", "/a /b 301")
	b := assembleBundle(dir)

	if got := b.partsNotCarried(&cloudflareDeployer{}); len(got) != 0 {
		t.Errorf("Cloudflare Pages carries both parts, got %v left behind", got)
	}
	want := []deployPart{partHeaders, partRedirects}
	if got := b.partsNotCarried(&sftpDeployer{}); !reflect.DeepEqual(got, want) {
		t.Errorf("sftp parts not carried = %v, want %v", got, want)
	}
	if got := b.partsNotCarried(&ftpsDeployer{}); !reflect.DeepEqual(got, want) {
		t.Errorf("ftps parts not carried = %v, want %v", got, want)
	}
}
