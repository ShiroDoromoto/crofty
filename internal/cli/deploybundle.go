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
// given provider supports, and never has to name a file to decide what a deploy
// would leave behind.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// deployPart names one non-asset artifact a site can need. The value is the
// part's conventional name on disk, which is also how it is discovered.
type deployPart string

const (
	partHeaders   deployPart = "_headers"
	partRedirects deployPart = "_redirects"
	partRoutes    deployPart = "_routes.json"
	partFunctions deployPart = "functions"
	partWorker    deployPart = "_worker.js"
)

// partSpec says where a part is found, what shape it has on disk, and what it
// costs to leave it behind.
type partSpec struct {
	part deployPart

	// atRoot marks the parts that live at the project root rather than in the
	// build. Hugo never emits these — they are the host's own inputs, not
	// content — so the build output is the one place they can't be seen.
	atRoot bool

	// isDir distinguishes a directory part from a file part, so a directory
	// named _worker.js (or a file named functions) is not mistaken for one.
	isDir bool

	// live marks a part that answers requests. Leaving a live part behind takes
	// those routes offline; leaving an inert one behind (a host that ignores
	// _headers just serves without them) costs nothing that was working.
	live bool

	// elsewhere is how a live part reaches production when crofty can't carry
	// it — what to tell the author instead of just refusing.
	elsewhere string
}

// partSpecs is the vocabulary of parts, in the order they are shown to the user.
func partSpecs() []partSpec {
	const viaPages = "wrangler, or a Pages git build"
	return []partSpec{
		{part: partHeaders},
		{part: partRedirects},
		{part: partRoutes, atRoot: true},
		{part: partFunctions, atRoot: true, isDir: true, live: true, elsewhere: viaPages},
		{part: partWorker, atRoot: true, live: true, elsewhere: viaPages},
	}
}

// label is how a part is written to the user: a directory keeps its slash.
func (s partSpec) label() string {
	if s.isDir {
		return string(s.part) + "/"
	}
	return string(s.part)
}

func specOf(p deployPart) partSpec {
	for _, s := range partSpecs() {
		if s.part == p {
			return s
		}
	}
	return partSpec{part: p}
}

// partLabels writes a list of parts the way the user sees them.
func partLabels(parts []deployPart) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = specOf(p).label()
	}
	return out
}

// deployBundle is one site's artifacts, ready to hand to any Deployer: a
// directory of static assets plus whichever parts were found.
type deployBundle struct {
	assetsDir string                // directory whose contents are published as-is
	parts     map[deployPart]string // part → absolute path on disk (only those present)
}

// assembleBundle collects the artifacts of a site: the built assets, the parts
// beside them, and the parts that only ever live at the project root. A missing
// build is not an error — the deploy gate assembles before anything is built,
// and every part that can stop a deploy lives at the root.
func assembleBundle(root, distDir string) deployBundle {
	b := deployBundle{assetsDir: distDir, parts: rootParts(root)}
	for _, s := range partSpecs() {
		if s.atRoot {
			continue
		}
		if p, ok := partAt(distDir, s); ok {
			b.parts[s.part] = p
		}
	}
	return b
}

// rootParts collects the parts that live at the project root. Split out from
// assembleBundle because `crofty config` reports on them without a build.
func rootParts(root string) map[deployPart]string {
	found := map[deployPart]string{}
	for _, s := range partSpecs() {
		if !s.atRoot {
			continue
		}
		if p, ok := partAt(root, s); ok {
			found[s.part] = p
		}
	}
	return found
}

// partAt looks for one part in dir, insisting it has the shape the spec says: a
// name alone is not a part. Lstat, not Stat, so a symlink is never a part — the
// asset walks skip symlinks too, and both need to agree on what is a real file.
func partAt(dir string, s partSpec) (string, bool) {
	if dir == "" {
		return "", false
	}
	p := filepath.Join(dir, string(s.part))
	fi, err := os.Lstat(p)
	if err != nil {
		return "", false
	}
	if s.isDir != fi.IsDir() || (!s.isDir && !fi.Mode().IsRegular()) {
		return "", false
	}
	return p, true
}

// partsNotCarried returns the parts in this bundle that a destination carrying
// exactly `carried` cannot deliver, in partSpecs order.
func (b deployBundle) partsNotCarried(carried []deployPart) []deployPart {
	can := map[deployPart]bool{}
	for _, p := range carried {
		can[p] = true
	}
	var out []deployPart
	for _, s := range partSpecs() {
		if _, present := b.parts[s.part]; present && !can[s.part] {
			out = append(out, s.part)
		}
	}
	return out
}

// livePartsNotCarried narrows partsNotCarried to the parts whose absence takes
// working routes offline — the ones worth stopping a deploy over.
func (b deployBundle) livePartsNotCarried(carried []deployPart) []deployPart {
	var out []deployPart
	for _, p := range b.partsNotCarried(carried) {
		if specOf(p).live {
			out = append(out, p)
		}
	}
	return out
}

// liveParts returns the parts in this bundle that answer requests, whoever the
// destination is — what `--static-only` chooses to leave behind.
func (b deployBundle) liveParts() []deployPart {
	var out []deployPart
	for _, s := range partSpecs() {
		if _, present := b.parts[s.part]; present && s.live {
			out = append(out, s.part)
		}
	}
	return out
}

// withoutLiveParts is this bundle with the request-answering parts removed, so a
// destination that could carry them doesn't. It backs `--static-only`, whose
// meaning is "publish the static site and leave those behind on purpose" — a
// choice that must survive crofty learning to carry the part.
func (b deployBundle) withoutLiveParts() deployBundle {
	kept := make(map[deployPart]string, len(b.parts))
	for p, path := range b.parts {
		if !specOf(p).live {
			kept[p] = path
		}
	}
	return deployBundle{assetsDir: b.assetsDir, parts: kept}
}

// carriesPart reports whether a destination carrying exactly `carried` can
// deliver p.
func carriesPart(carried []deployPart, p deployPart) bool {
	for _, c := range carried {
		if c == p {
			return true
		}
	}
	return false
}

// functionsDirHoldsSource judges what a top-level functions/ directory *in a
// build* actually is, because the name alone cannot tell the two apart:
//
//   - Pages Functions source only reaches the build through static/, which
//     copies verbatim. It is JavaScript on its way to a runtime, never HTML.
//   - A content section named functions/ ("function reference", "features") is
//     rendered by Hugo, so every page under it is an index.html.
//
// So: a tree holding no HTML is source. Deciding by name dropped that section
// from the deploy without saying so, which is the worse of the two mistakes —
// publishing a page nobody asked for is visible, a missing page looks like a
// deploy that worked.
func functionsDirHoldsSource(dir string) bool {
	holdsHTML := false
	// An unreadable entry is skipped rather than fatal: this only chooses which
	// of two treatments applies, and the caller's own walk reports real errors.
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".html") {
			holdsHTML = true
			return fs.SkipAll
		}
		return nil
	})
	return !holdsHTML
}

// rootPartNamed matches a name and shape against the parts that belong at the
// project root. It is how a copy of one, found somewhere else, is recognised.
func rootPartNamed(name string, isDir bool) (partSpec, bool) {
	for _, s := range partSpecs() {
		if s.atRoot && string(s.part) == name && s.isDir == isDir {
			return s, true
		}
	}
	return partSpec{}, false
}
