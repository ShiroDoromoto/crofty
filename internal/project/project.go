// Package project resolves a crofty project on disk: the root, its crofty-
// specific config, and the working paths used by build and deploy.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MarkerDir is the per-project crofty working directory. It holds tool state
// (and, later, keys) and is never part of the rendered output, so secrets cannot
// ride along to deploy. Its mere presence does NOT mark a project root: anyone
// may drop a file under it (e.g. .crofty/bin/crofty.exe), and a folder must not
// become a project because of someone else's file.
const MarkerDir = ".crofty"

// ConfigFile holds crofty-specific settings as JSON inside MarkerDir. Hugo's own
// config (hugo.yaml) stays standard and untouched so the project remains a
// plain, ejectable Hugo project. crofty writes this file itself, which is why it
// — not MarkerDir — is the marker of a project root (D-2).
const ConfigFile = "config.json"

// Config is the crofty-specific project config. Hugo settings (baseURL, title)
// live in hugo.yaml and are read by Hugo directly, not here.
type Config struct {
	// Workspace is a stable id used to namespace keychain entries (A5). It is
	// assigned once and never contains secrets.
	Workspace string       `json:"workspace,omitempty"`
	Deploy    DeployConfig `json:"deploy"`

	// FooterCredit controls the optional "Made with crofty" footer line:
	// "on", "off", or "" (unset). It is a free, removable referral — never
	// forced. crofty asks once, neutrally, on the first interactive deploy
	// (before the build that renders it); a non-interactive deploy is never
	// asked, so unset always renders as off. Change it anytime with
	// `crofty credit on|off`. Never silently defaults to on.
	FooterCredit string `json:"footerCredit,omitempty"`
}

// Footer-credit values for Config.FooterCredit. "" means unset (renders off).
const (
	FooterCreditOn  = "on"
	FooterCreditOff = "off"
)

// DeployConfig describes where a build is published. Provider selects the
// backend; the remaining fields are read per provider. Secrets (API tokens,
// passwords, key passphrases) are never stored here — they live in the OS
// keychain — so this file can sit in a repo safely.
type DeployConfig struct {
	// Provider is the deploy backend: "cloudflare" (Pages), "sftp", or "ftps".
	Provider string `json:"provider"`

	// --- Cloudflare Pages ---
	// Project is the Cloudflare Pages project name.
	Project string `json:"project,omitempty"`
	// Branch is the Cloudflare Pages production branch to deploy to. Empty means
	// "main". This pins deploys to production regardless of the local git branch.
	Branch string `json:"branch,omitempty"`
	// AccountID pins the Cloudflare account a project deploys to. It is recorded
	// on the first deploy and the matching token is loaded from the keychain, so
	// the site can't be silently retargeted to another account. Non-secret (an
	// account id is not a key).
	AccountID string `json:"accountId,omitempty"`
	// Worker holds the author's declarations about the _worker.js crofty carries.
	Worker WorkerConfig `json:"worker,omitempty"`

	// --- SFTP / FTPS ---
	// Host is the server hostname (no scheme).
	Host string `json:"host,omitempty"`
	// Port is the server port. 0 means the protocol default (22 for SFTP, 21 for
	// FTPS).
	Port int `json:"port,omitempty"`
	// User is the login user.
	User string `json:"user,omitempty"`
	// Path is the remote target directory — the web root, which is usually NOT
	// the login home (e.g. /public_html, /var/www/site). dist/ is uploaded here.
	Path string `json:"path,omitempty"`
	// KeyPath points at an SFTP private key file (a pointer, not the key itself,
	// so it's safe here). Empty means password auth.
	KeyPath string `json:"keyPath,omitempty"`
	// TLSSkipVerify accepts a shared or self-signed TLS certificate for FTPS,
	// common on budget shared hosting. Off by default.
	TLSSkipVerify bool `json:"tlsSkipVerify,omitempty"`
}

// WorkerConfig is what the author declares about their worker's runtime.
// crofty carries the worker but does not decide how it runs: these are the
// author's answers, written down where they can be read and versioned, rather
// than defaults crofty invents.
type WorkerConfig struct {
	// CompatibilityDate pins the Workers runtime the worker is written against
	// (YYYY-MM-DD). Empty means undeclared, and crofty supplies nothing in its
	// place: an unpinned worker falls back to the Pages project's own setting,
	// and to the oldest runtime if there is none. That is worse than it sounds
	// but better than the alternative — a crofty default would mean upgrading
	// crofty silently changes the runtime the author's worker executes on.
	CompatibilityDate string `json:"compatibilityDate,omitempty"`

	// RequiredEnv names the environment variables the worker needs to answer a
	// request — names only, never values. A worker whose variables are unset
	// deploys perfectly and then fails one route at a time, which is the hardest
	// kind of failure to trace back to a deploy. Declaring the names lets crofty
	// compare them against the destination and say what is missing.
	//
	// Values stay out of this file on purpose. crofty keeps secrets in the OS
	// keychain, and it will not grow a path that reads them and hands them to
	// Cloudflare: carrying someone's secrets is a different job than publishing
	// their site.
	RequiredEnv []string `json:"requiredEnv,omitempty"`
}

// Project is a resolved crofty project rooted at Root.
type Project struct {
	Root string
}

// ErrNotFound is returned when no project root is found walking up from a start dir.
var ErrNotFound = errors.New("not inside a crofty project (no .crofty/config.json found in this or any parent directory)")

// StrayMarkerError says a .crofty/ directory exists without the config.json that
// would make it a project root. It wraps ErrNotFound — such a folder is not a
// project — but names the directory so the caller can explain the half-state
// instead of pretending nothing is there.
type StrayMarkerError struct{ Dir string }

func (e *StrayMarkerError) Error() string {
	return fmt.Sprintf("%s has a %s/ directory but no %s — it is not a crofty project",
		e.Dir, MarkerDir, ConfigFile)
}

func (e *StrayMarkerError) Unwrap() error { return ErrNotFound }

// IsProject reports whether dir is a crofty project root, i.e. holds the config
// file crofty itself writes.
func IsProject(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, MarkerDir, ConfigFile))
	return err == nil && fi.Mode().IsRegular()
}

// Find walks up from start looking for a project root, so build and deploy work
// from any subdirectory of a project. When nothing is found but a stray MarkerDir
// was passed on the way, the error names it: that folder looks like a project to
// a human and silence there is the worst answer.
func Find(start string) (*Project, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	stray := ""
	for {
		if IsProject(dir) {
			return &Project{Root: dir}, nil
		}
		if stray == "" {
			if fi, err := os.Stat(filepath.Join(dir, MarkerDir)); err == nil && fi.IsDir() {
				stray = dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			if stray != "" {
				return nil, &StrayMarkerError{Dir: stray}
			}
			return nil, ErrNotFound
		}
		dir = parent
	}
}

// ConfigPath is the absolute path to the crofty config file.
func (p *Project) ConfigPath() string {
	return filepath.Join(p.Root, MarkerDir, ConfigFile)
}

// ThemesDir is where crofty materializes the bundled theme at build time.
func (p *Project) ThemesDir() string {
	return filepath.Join(p.Root, MarkerDir, "themes")
}

// DistDir is the build output directory — the only thing deploy uploads.
func (p *Project) DistDir() string {
	return filepath.Join(p.Root, "dist")
}

// LoadConfig reads and parses .crofty/config.json.
func (p *Project) LoadConfig() (*Config, error) {
	b, err := os.ReadFile(p.ConfigPath())
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p.ConfigPath(), err)
	}
	return &c, nil
}

// SaveConfig writes the config back to .crofty/config.json, creating the marker
// directory if needed. It never contains secrets (those live in the keychain).
func (p *Project) SaveConfig(c *Config) error {
	if err := os.MkdirAll(filepath.Join(p.Root, MarkerDir), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.ConfigPath(), append(b, '\n'), 0o644)
}
