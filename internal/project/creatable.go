package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// EnsureCreatable answers, before crofty writes anything, whether it may create
// a project at dir. crofty picks that location itself when given a bare name
// (DefaultBase), so the author never chose the folder that refuses the write —
// and on a locked-down or redirected Windows profile it does refuse. Asked
// afterwards, the question comes back as a bare "Access is denied." on top of a
// half-written site; asked first, it is a fork the author picks from (D-1).
func EnsureCreatable(dir string) error {
	anchor, err := anchorFor(dir)
	if err != nil {
		return err
	}
	if err := probeWrite(anchor); err != nil {
		if access.IsPermission(err) {
			return denyCreateSite(dir, anchor, err)
		}
		return fmt.Errorf("crofty cannot create %s: %w", dir, err)
	}
	return nil
}

// EnsureStateWritable answers whether crofty may write its own state — the
// registry that lets any session find a project from any folder. It reports the
// wall the write itself would report, with the same ways on (denyRegistryWrite),
// so that a caller wanting to know before it starts need not learn by starting.
//
// A wall here is never fatal on its own: the registry only powers discovery, and
// a site crofty cannot register is still a site (#13). Callers decide what to do
// with it; none of them should refuse to write a site over it.
func EnsureStateWritable() error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	anchor, err := anchorFor(filepath.Dir(path))
	if err != nil {
		return err
	}
	return denyRegistryWrite(path, probeWrite(anchor))
}

// anchorFor is the existing directory a write to dir would land in: dir itself
// when it is there already, otherwise the nearest ancestor that is — the folder
// MkdirAll would have to create inside.
func anchorFor(dir string) (string, error) {
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("crofty cannot write under %s: it is a file, not a folder", dir)
			}
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("crofty cannot write under %s: no folder above it exists", dir)
		}
		dir = parent
	}
}

// probeWrite asks the filesystem the only way it answers honestly: by writing.
// A Windows ACL, a read-only mount and a full disk all refuse writes that a
// permission bit permits. The probe is removed again.
func probeWrite(dir string) error {
	probe, err := os.CreateTemp(dir, ".crofty-write-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}

// denyCreateSite names the ways past a wall on the place a site would go. Two of
// them cost the author nothing: crofty chose this folder, so choosing another is
// as good an answer as granting access to this one.
func denyCreateSite(dir, anchor string, err error) error {
	return access.Deny("create the site "+dir, anchor, err,
		access.Choice{
			Do:         "let crofty write to " + anchor + ", then run the command again",
			Permission: "write access to " + anchor,
		},
		access.Choice{
			Do:      "put the site in the folder you are standing in instead",
			Command: "crofty init .",
		},
		access.Choice{
			Do:      "put the site anywhere crofty may write",
			Command: "crofty init <a folder crofty may write to>",
		},
	)
}
