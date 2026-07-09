package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// DefaultBase is the OS-standard, user-visible folder where crofty creates a
// project when given a bare name. It deliberately ignores the current working
// directory: a first-timer (and their agent, started in some arbitrary dir)
// can't be assumed to know or control where the shell is, so projects go to one
// predictable, announced place instead (see 07 O2).
//
// It is ~/Documents/Crofty, for whatever the OS says Documents is —
// documentsDir answers that per-OS (documents_windows.go, documents_other.go).
func DefaultBase() (string, error) {
	docs, err := documentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(docs, "Crofty"), nil
}

// HomeEnv names the environment variable that relocates crofty's state
// directory. Sandboxed environments (notably Windows ones) can refuse writes to
// the OS config dir; without this the only way past would be to rewrite
// %APPDATA% itself, which is exactly the kind of workaround crofty tells an
// agent never to invent on its own (D-1). So it offers a door instead.
const HomeEnv = "CROFTY_HOME"

// GlobalDir is crofty's per-user state directory (it holds the project
// registry). Removing it is part of a clean uninstall (`crofty reset --all`).
func GlobalDir() (string, error) {
	if home := os.Getenv(HomeEnv); home != "" {
		return filepath.Abs(home)
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "crofty"), nil
}

// registryPath is the global, non-secret list of project locations. It lets any
// agent find projects from any directory, across sessions, without relying on
// the agent's memory or on the user knowing a path (see 07 O3).
func registryPath() (string, error) {
	dir, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects.json"), nil
}

type registry struct {
	Projects []string `json:"projects"`
}

func readRegistry(path string) registry {
	var reg registry
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &reg)
	}
	return reg
}

// RegisterProject records an absolute project path in the global registry so
// later sessions can discover it. It is idempotent and holds no secrets.
//
// A permission wall here comes back as an access.Denied carrying the ways on,
// because registration is a convenience, not the site: callers are expected to
// say what could not be recorded and carry on (see init).
func RegisterProject(abs string) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	reg := readRegistry(path)
	for _, p := range reg.Projects {
		if p == abs {
			return nil
		}
	}
	reg.Projects = append(reg.Projects, abs)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return denyRegistryWrite(path, err)
	}
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return denyRegistryWrite(path, os.WriteFile(path, append(b, '\n'), 0o644))
}

// denyRegistryWrite names the two honest ways past a wall on the registry: move
// crofty's state somewhere writable, or accept that discovery is off. Naming
// them keeps an agent from inventing a worse one — rewriting %APPDATA%, or
// dropping the registry inside the project where crofty would never read it.
func denyRegistryWrite(path string, err error) error {
	return access.Deny("record this project so crofty can find it from anywhere", path, err,
		access.Choice{
			Do:         "keep crofty's state somewhere it may write, then run the command again",
			Command:    HomeEnv + "=<a folder crofty may write to> crofty init …",
			Permission: "setting " + HomeEnv + " in your environment",
		},
		access.Choice{
			// True whether the site is already written (init reports this after
			// the fact) or not yet (the preflight reports it before): registration
			// is never what makes a site a site.
			Do: "leave it — the registry only powers discovery; crofty won't find the site from other folders, so cd into it first",
		},
	)
}

// KnownProjects returns existing crofty project roots, merging the global
// registry with a scan of DefaultBase and pruning entries whose .crofty/ marker
// is gone. The registry covers projects created at custom paths; the scan is a
// fault-tolerant fallback for the default location if the registry is lost.
func KnownProjects() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			return
		}
		if !IsProject(abs) {
			return
		}
		seen[abs] = true
		out = append(out, abs)
	}
	if path, err := registryPath(); err == nil {
		for _, p := range readRegistry(path).Projects {
			add(p)
		}
	}
	if base, err := DefaultBase(); err == nil {
		if entries, err := os.ReadDir(base); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					add(filepath.Join(base, e.Name()))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}
