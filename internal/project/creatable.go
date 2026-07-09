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
//
// It probes for real, by writing: a Windows ACL, a read-only mount and a full
// disk all deny writes that a permission bit says nothing about.
func EnsureCreatable(dir string) error {
	// The folder itself when it exists ('crofty init .'), otherwise the nearest
	// ancestor that does — the one MkdirAll would have to create inside.
	anchor := dir
	for {
		if info, err := os.Stat(anchor); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("crofty cannot create %s: %s is a file, not a folder", dir, anchor)
			}
			break
		}
		parent := filepath.Dir(anchor)
		if parent == anchor {
			return fmt.Errorf("crofty cannot create %s: no folder above it exists", dir)
		}
		anchor = parent
	}

	probe, err := os.CreateTemp(anchor, ".crofty-write-probe-*")
	if err != nil {
		if access.IsPermission(err) {
			return denyCreateSite(dir, anchor, err)
		}
		return fmt.Errorf("crofty cannot create %s: %w", dir, err)
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
