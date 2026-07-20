package cli

// What a deploy sends, and where it is collected from.
//
// A site is more than the pages Hugo renders: it also needs the files that tell
// the host how to serve them. So a deploy carries a *bundle* — the static assets
// plus those extra parts — rather than one directory.
//
// The split this file exists to hold: *what* goes into a bundle is decided here,
// in the common layer, without knowing which provider will take it. *How* each
// part travels — which API field, in what wrapper — stays inside the provider
// (cfdeploy.go, sftpdeploy.go, ftpsdeploy.go). Each Deployer declares the parts
// it can deliver (Deployer.Carries), so the common layer never has to ask what a
// given provider supports.

import (
	"os"
	"path/filepath"
)

// deployPart names one non-asset artifact a site can need. The value is the
// file's conventional name at the root of the site, which is also how the part
// is discovered.
type deployPart string

const (
	partHeaders   deployPart = "_headers"
	partRedirects deployPart = "_redirects"
)

// knownParts lists every part crofty recognises, in a stable order so anything
// derived from it (warnings, error messages) reads the same on every run.
func knownParts() []deployPart { return []deployPart{partHeaders, partRedirects} }

// deployBundle is one site's artifacts, ready to hand to any Deployer: a
// directory of static assets plus whichever parts were found alongside them.
type deployBundle struct {
	assetsDir string                // directory whose contents are published as-is
	parts     map[deployPart]string // part → absolute path on disk (only those present)
}

// assembleBundle collects the artifacts of a built site. Parts are found by
// their conventional names at the root of the build; a site that has none
// deploys as plain assets, exactly as before.
func assembleBundle(distDir string) deployBundle {
	b := deployBundle{assetsDir: distDir, parts: map[deployPart]string{}}
	for _, p := range knownParts() {
		path := filepath.Join(distDir, string(p))
		// Lstat, not Stat: a symlink is not a part (nor an asset) — the asset
		// walks skip them too, so both agree on what a regular file is.
		if fi, err := os.Lstat(path); err == nil && fi.Mode().IsRegular() {
			b.parts[p] = path
		}
	}
	return b
}

// partsNotCarried returns the parts in this bundle that d cannot deliver, in
// knownParts order. A provider uses it to say so rather than dropping them
// silently.
func (b deployBundle) partsNotCarried(d Deployer) []deployPart {
	carried := map[deployPart]bool{}
	for _, p := range d.Carries() {
		carried[p] = true
	}
	var out []deployPart
	for _, p := range knownParts() {
		if _, present := b.parts[p]; present && !carried[p] {
			out = append(out, p)
		}
	}
	return out
}
