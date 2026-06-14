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

// MarkerDir is the per-project crofty working directory. Its presence marks a
// project root. It holds tool state (and, later, keys) and is never part of the
// rendered output, so secrets cannot ride along to deploy.
const MarkerDir = ".crofty"

// ConfigFile holds crofty-specific settings as JSON inside MarkerDir. Hugo's own
// config (hugo.yaml) stays standard and untouched so the project remains a
// plain, ejectable Hugo project.
const ConfigFile = "config.json"

// Config is the crofty-specific project config. Hugo settings (baseURL, title)
// live in hugo.yaml and are read by Hugo directly, not here.
type Config struct {
	// Workspace is a stable id used to namespace keychain entries (A5). It is
	// assigned once and never contains secrets.
	Workspace string       `json:"workspace,omitempty"`
	Deploy    DeployConfig `json:"deploy"`
	// Targets holds non-secret per-destination settings; credentials live in the
	// keychain, never here.
	Targets map[string]TargetConfig `json:"targets,omitempty"`
}

// DeployConfig describes where a build is published.
type DeployConfig struct {
	// Provider is the deploy backend. M1 supports "cloudflare" (Pages).
	Provider string `json:"provider"`
	// Project is the Cloudflare Pages project name.
	Project string `json:"project"`
	// Branch is the Cloudflare Pages production branch to deploy to. Empty means
	// "main". This pins deploys to production regardless of the local git branch.
	Branch string `json:"branch,omitempty"`
	// AccountID pins the Cloudflare account a project deploys to. It is recorded
	// on the first deploy and the matching token is loaded from the keychain, so
	// the site can't be silently retargeted to another account. Non-secret (an
	// account id is not a key).
	AccountID string `json:"accountId,omitempty"`
}

// TargetConfig is the non-secret configuration of a syndication destination.
type TargetConfig struct {
	Type   string `json:"type"`             // "bluesky" | "mastodon"
	Handle string `json:"handle"`           // non-secret account identifier
	Server string `json:"server,omitempty"` // instance/PDS base URL
}

// Project is a resolved crofty project rooted at Root.
type Project struct {
	Root string
}

// ErrNotFound is returned when no MarkerDir is found walking up from a start dir.
var ErrNotFound = errors.New("not inside a crofty project (no .crofty/ found in this or any parent directory)")

// Find walks up from start looking for the MarkerDir, so build and deploy work
// from any subdirectory of a project.
func Find(start string) (*Project, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for {
		marker := filepath.Join(dir, MarkerDir)
		if fi, err := os.Stat(marker); err == nil && fi.IsDir() {
			return &Project{Root: dir}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
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

// StatePath is the publish ledger location; plain, non-secret, deploy-excluded.
func (p *Project) StatePath() string {
	return filepath.Join(p.Root, MarkerDir, "state.json")
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
