package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// The update notifier nudges a user on a stale binary toward the upgrade command
// for however they installed crofty. It exists because crofty is distributed
// through routes (installers, Homebrew, Scoop, .deb/.rpm) that do NOT silently
// auto-upgrade an installed binary: without a nudge, someone who installed once
// can sit on an old version forever, never learning a fix or feature shipped.
// We deliberately do NOT self-update the binary — that would fight the package
// manager's bookkeeping; we only print the one command to run.
//
// It is quiet by construction: it never runs for source builds (Version=="dev"),
// it can be turned off with CROFTY_NO_UPDATE_CHECK, it talks to the network at
// most once a day (cached), it fails silently when offline, and the human line
// only prints to an interactive terminal — scripts, pipes and agents get the
// same fact through the `updateAvailable` field of `crofty agent --json`
// instead, never as noise on stderr.

const (
	updateRepoAPI   = "https://api.github.com/repos/ShiroDoromoto/crofty/releases/latest"
	updateCacheFile = "update-check.json"
	updateInterval  = 24 * time.Hour
	updateTimeout   = 1500 * time.Millisecond
)

// updateCache is the on-disk record of the last successful network check.
type updateCache struct {
	CheckedAt int64  `json:"checked_at"` // unix seconds of the last network check
	Latest    string `json:"latest"`     // latest release version, no leading "v"
}

// updateInfo reports the latest released version and whether it is newer than
// the running binary. It reads a once-a-day cache and only touches the network
// when that cache is stale, so repeated calls within a day are free. It returns
// ("", false) — never an error — whenever a check is undesired or impossible
// (source build, opted out, offline, unparsable), so callers can stay terse.
func updateInfo() (latest string, newer bool) {
	if Version == "dev" || os.Getenv("CROFTY_NO_UPDATE_CHECK") != "" {
		return "", false
	}

	cache, _ := readUpdateCache()
	if cache.Latest == "" || time.Since(time.Unix(cache.CheckedAt, 0)) > updateInterval {
		if got, err := fetchLatestRelease(); err == nil && got != "" {
			cache = updateCache{CheckedAt: time.Now().Unix(), Latest: got}
			_ = writeUpdateCache(cache)
		}
		// On fetch failure we keep whatever cache we had (possibly empty); we
		// never block on, or surface, a network problem.
	}
	if cache.Latest == "" {
		return "", false
	}
	return cache.Latest, semverLess(Version, cache.Latest)
}

// maybeNotifyUpdate prints a single upgrade nudge to stderr when a newer release
// exists. It is called once at the end of every crofty run. It stays silent
// unless stderr is an interactive terminal, so it can never corrupt the stdout
// of a `--json` command nor add noise to scripted or agent-driven runs.
func maybeNotifyUpdate() {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return
	}
	latest, newer := updateInfo()
	if !newer {
		return
	}
	fmt.Fprintf(os.Stderr, "\ncrofty %s is available (you have %s).\nUpdate with: %s\n",
		latest, Version, upgradeHint())
}

// fetchLatestRelease asks GitHub for the latest *stable* release tag (the
// /releases/latest endpoint already excludes drafts and pre-releases) and
// returns its version with any leading "v" stripped.
func fetchLatestRelease() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateRepoAPI, nil)
	if err != nil {
		return "", err
	}
	// GitHub requires a User-Agent; identify ourselves so the call is traceable.
	req.Header.Set("User-Agent", "crofty/"+Version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v"), nil
}

// upgradeHint returns the exact command to update crofty, inferred from where
// the running binary lives. The inference degrades gracefully: anything
// unrecognized falls back to the releases page, which is always correct.
func upgradeHint() string {
	exe, err := os.Executable()
	if err == nil {
		if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
			exe = resolved
		}
	}
	return upgradeHintFor(exe, runtime.GOOS)
}

// upgradeHintFor maps a resolved binary path (and OS) to the right upgrade
// command. Split out from upgradeHint so the classification is testable without
// depending on where the test binary happens to live.
func upgradeHintFor(exe, goos string) string {
	low := strings.ToLower(exe)
	switch {
	case strings.Contains(low, "/cellar/") || strings.Contains(low, "/homebrew/"):
		return "brew upgrade crofty"
	case strings.Contains(low, "scoop"):
		return "scoop update crofty"
	case strings.Contains(low, `\appdata\local\`):
		// per-user install.ps1 target (%LOCALAPPDATA%\<project>\bin): re-run the script
		return "re-run: irm https://github.com/ShiroDoromoto/crofty/releases/latest/download/install.ps1 | iex"
	case strings.Contains(low, "/.local/"):
		// per-user install.sh target ($HOME/.local/bin): re-run the script
		return "re-run: curl -fsSL https://crofty.site/install.sh | sh"
	case goos == "linux" && strings.HasPrefix(exe, "/usr/"):
		// Installed from the .deb/.rpm. There is no apt/yum repo behind them, so
		// `apt upgrade` will never see the new version: name the package.
		return "install the new .deb/.rpm from https://github.com/ShiroDoromoto/crofty/releases over this one"
	default:
		return "download the latest from https://github.com/ShiroDoromoto/crofty/releases"
	}
}

// semverLess reports whether version a is strictly older than version b. Both
// are dotted numeric versions ("0.9.0"), optionally with a leading "v" or a
// pre-release/build suffix, which we ignore. On any parse trouble it returns
// false, so an unrecognizable version never triggers a nudge.
func semverLess(a, b string) bool {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

// parseVersion turns "v1.2.3" (or "1.2.3-rc1", "1.2") into [1,2,3]. Missing
// trailing components default to 0; a non-numeric component fails the parse.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 { // drop "-rc1" / "+build"
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func updateCachePath() (string, error) {
	dir, err := project.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, updateCacheFile), nil
}

func readUpdateCache() (updateCache, error) {
	var c updateCache
	path, err := updateCachePath()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

func writeUpdateCache(c updateCache) error {
	dir, err := project.GlobalDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, updateCacheFile), b, 0o644)
}
