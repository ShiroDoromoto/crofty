package cli

// Shared helpers for the plain-server deploy backends (SFTP, FTPS): scanning the
// built dist/ into an upload list, prompting for credentials on a TTY (never
// through an assistant), and the warnings both backends share — chiefly that a
// plain static host can't run Cloudflare's edge files.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// serverFile is one file to upload: its slash-relative path under dist and its
// absolute path on disk.
type serverFile struct {
	rel string
	abs string
}

// scanDistTree walks dir and returns every regular file (slash-relative paths,
// no leading slash) plus whether the build carries Cloudflare-only edge files
// (_headers / _redirects / _worker.js / functions/) that won't run on a plain
// host. The edge files are still uploaded — harmless static text — but the
// caller warns they're inert here.
func scanDistTree(dir string) (files []serverFile, hasEdgeFiles bool, err error) {
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		name := filepath.ToSlash(rel)
		if d.IsDir() {
			if name == "functions" {
				hasEdgeFiles = true
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, sockets, etc.
		}
		if !strings.Contains(name, "/") {
			switch name {
			case "_headers", "_redirects", "_worker.js":
				hasEdgeFiles = true
			}
		}
		files = append(files, serverFile{rel: name, abs: p})
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return files, hasEdgeFiles, nil
}

// remoteDirs returns the set of ancestor directories (slash-relative to the
// remote root) implied by files, ordered shallowest-first so each can be created
// after its parent.
func remoteDirs(files []serverFile) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		parts := strings.Split(f.rel, "/")
		for i := 1; i < len(parts); i++ {
			d := strings.Join(parts[:i], "/")
			if !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
		}
	}
	// Shallowest-first: a path with fewer separators is an ancestor.
	sortByDepth(dirs)
	return dirs
}

// sortByDepth orders paths so shorter (shallower) ones come first. A simple
// insertion sort keeps this dependency-free and the lists are tiny.
func sortByDepth(dirs []string) {
	for i := 1; i < len(dirs); i++ {
		for j := i; j > 0 && strings.Count(dirs[j], "/") < strings.Count(dirs[j-1], "/"); j-- {
			dirs[j], dirs[j-1] = dirs[j-1], dirs[j]
		}
	}
}

// warnInPlace tells the user a plain-host deploy overwrites in place — there's a
// brief window mid-upload where the site is a mix of old and new files (no atomic
// swap, unlike Cloudflare). Shown before any upload starts.
func warnInPlace(progress func(string)) {
	progress("⚠ in-place upload: while files transfer, the live site is briefly mixed old+new.")
	progress("  (A plain host has no atomic switch — re-run if a transfer is interrupted.)")
}

// warnEdgeFiles tells the user the Cloudflare-only edge files in the build do
// nothing on a plain static host.
func warnEdgeFiles(progress func(string)) {
	progress("⚠ this build has Cloudflare edge files (_headers/_redirects/_worker.js);")
	progress("  a plain SFTP/FTPS host serves static files only, so those won't take effect.")
}

// requireServerConfig checks the non-secret fields SFTP/FTPS need are set,
// returning a single actionable error naming what's missing and how to set it.
func requireServerConfig(cfg *deployServerConfig, configPath string) error {
	var missing []string
	if cfg.host == "" {
		missing = append(missing, "host")
	}
	if cfg.user == "" {
		missing = append(missing, "user")
	}
	if cfg.path == "" {
		missing = append(missing, "path (remote web root)")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("deploy config is missing %s in %s\n"+
		"  Set it with 'crofty connect' (interactive) or re-run 'crofty init' with\n"+
		"  --host --user --path (and --port/--key as needed).",
		strings.Join(missing, ", "), configPath)
}

// deployServerConfig is the resolved, non-secret connection info SFTP/FTPS share.
type deployServerConfig struct {
	host    string
	port    int
	user    string
	path    string
	keyPath string // SFTP only; "" means password auth
}

// promptSecretTTY reads a secret (password / passphrase) from a hidden terminal
// prompt. Like the Cloudflare token flow, a secret is never accepted through a
// flag or pipe, so it can't pass through an assistant's context.
func promptSecretTTY(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("%s must be typed in a terminal, never through an assistant — run 'crofty deploy' (or 'crofty connect') yourself", label)
	}
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Printf("%s: ", label)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
		fmt.Println("  (nothing entered — try again)")
	}
	return "", fmt.Errorf("nothing entered for %s", label)
}
