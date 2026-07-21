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

	"github.com/ShiroDoromoto/crofty/internal/hugobin"
	"github.com/ShiroDoromoto/crofty/internal/project"
)

// The update notifier nudges a user on a stale binary toward the upgrade command
// for however they installed crofty. It exists because crofty is distributed
// through routes (click installers, install scripts) that do NOT silently
// auto-upgrade an installed binary: without a nudge, someone who installed once
// can sit on an old version forever, never learning a fix or feature shipped.
//
// This file is the notifier, not the updater — it only points the way. Since
// D-339 the updater exists too: `crofty update` (updatecmd.go) self-fetches the
// release and swaps the binary in place, on the routes that can. But crofty still
// never updates on its own — no auto-upgrade at startup; the nudge runs at the
// end of a command to remind a stale binary, and the decision to act stays the
// author's. Both read the install route from the one classifier here, so they
// can never disagree about which way out to name.
//
// It is quiet by construction: it never runs for source builds (Version=="dev"),
// it can be turned off with CROFTY_NO_UPDATE_CHECK, it talks to the network at
// most once a day (cached), it fails silently when offline, and the human line
// only prints to an interactive terminal — scripts, pipes and agents get the
// same fact through the `updateAvailable` / `releaseNotesURL` fields of
// `crofty agent --json` instead, never as noise on stderr.

const (
	updateRepoAPI   = "https://api.github.com/repos/ShiroDoromoto/crofty/releases/latest"
	updateCacheFile = "update-check.json"
	updateInterval  = 24 * time.Hour
	updateTimeout   = 1500 * time.Millisecond

	// releaseNotesURL is where an upgrade nudge sends someone to find out what
	// actually changed. crofty never upgrades on its own, so every upgrade starts
	// as a decision the author makes — and "a newer version exists" is not enough
	// to make one on. It points at crofty.site rather than the GitHub releases page
	// because that one reads: it has both languages, and patches folded in.
	releaseNotesURL = "https://crofty.site/releases/"
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
	fmt.Fprintf(os.Stderr, "\ncrofty %s is available (you have %s).\nWhat changed: %s\nUpdate with: %s\n",
		latest, Version, releaseNotesURL, upgradeHint())
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

// upgradeHint returns the one line telling someone how to move to the latest
// release from where this binary is installed. On a route `crofty update` can
// self-update (D-339), that line is simply `crofty update`; on every other route
// it is the manual, route-back way — a dead channel to leave, a script to re-run —
// honouring D-326. The inference degrades gracefully: anything unrecognized falls
// back to the releases page, which is always correct.
func upgradeHint() string {
	exe, err := os.Executable()
	if err == nil {
		if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
			exe = resolved
		}
	}
	goos := runtime.GOOS
	bundled := hugobin.Bundled(exe, goos)
	// A dev build can't self-update; it never reaches here (the nudge is inert for
	// "dev"), but guard anyway so nothing ever tells a source build to run update.
	if Version != "dev" && classifyInstall(exe, goos, bundled).selfUpdates() {
		return "crofty update"
	}
	return upgradeHintFor(exe, goos, bundled)
}

// installRoute is how this binary got onto the machine, inferred from where it
// lives. It is the one classification both the upgrade nudge and `crofty update`
// stand on: the nudge turns a route into the sentence a human reads, and update
// turns the same route into a decision — self-fetch and swap, or hand back the
// nudge because this route can't (a dead channel, a root-owned install, Windows
// until 3b lands, or an install crofty doesn't recognize).
type installRoute int

const (
	routeUnknown      installRoute = iota // unrecognized (incl. `go install`) → releases page
	routeHomebrew                         // Homebrew (dead channel)
	routeScoop                            // Scoop (dead channel)
	routeDebRpm                           // .deb/.rpm on Linux (dead channel)
	routeWindowsClick                     // %LOCALAPPDATA%\crofty\bin (click installer / install.ps1)
	routeScriptUser                       // install.sh, per-user ($HOME/.local)
	routeScriptSystem                     // install.sh, system-wide (PREFIX=/usr/local, root-owned)
	routePkgDarwin                        // macOS .pkg body (user-writable, bundled Hugo alongside)
)

// selfUpdates reports whether `crofty update` can update this route in place —
// the routes with a user-writable body crofty can fetch and swap without root.
// Every other route (dead channels, a root-owned system-wide install, one crofty
// doesn't recognize) is sent to its manual, route-back way instead.
func (r installRoute) selfUpdates() bool {
	switch r {
	case routePkgDarwin, routeWindowsClick, routeScriptUser:
		return true
	default:
		return false
	}
}

// classifyInstall maps a resolved binary path (and OS) to its installRoute. It
// is split from the hint and the update logic so the one classification they
// share lives in a single place — which is also why the one fact it needs from
// the filesystem, whether a click installer's Hugo sits next to the binary,
// arrives as an argument rather than being looked up here.
//
// The order matters: the macOS routes both answer to /usr/local, and only the
// bundled Hugo tells the .pkg body apart from a system-wide install.sh.
func classifyInstall(exe, goos string, bundledHugo bool) installRoute {
	low := strings.ToLower(exe)
	switch {
	case strings.Contains(low, "/cellar/") || strings.Contains(low, "/homebrew/"):
		return routeHomebrew
	case strings.Contains(low, "scoop"):
		return routeScoop
	case strings.Contains(low, `\appdata\local\`):
		return routeWindowsClick
	case strings.Contains(low, "/.local/"):
		return routeScriptUser
	case strings.HasPrefix(low, "/usr/local/"):
		if goos == "darwin" && bundledHugo {
			return routePkgDarwin
		}
		return routeScriptSystem
	case goos == "linux" && strings.HasPrefix(exe, "/usr/"):
		return routeDebRpm
	default:
		return routeUnknown
	}
}

// upgradeHintFor returns the route-specific command that updates crofty from
// where this binary lives — the sentence the nudge and the `crofty update`
// refusal both print. It reads its route from classifyInstall so the two can
// never disagree about what kind of install this is.
func upgradeHintFor(exe, goos string, bundledHugo bool) string {
	switch classifyInstall(exe, goos, bundledHugo) {
	// crofty no longer ships to Homebrew or Scoop, so the tap and the bucket are
	// frozen: `brew upgrade` would find nothing and say nothing. Whoever installed
	// that way has to leave, and this notice is the one place they hear about it.
	case routeHomebrew:
		return "run 'brew uninstall crofty', then install from https://crofty.site — crofty no longer ships to Homebrew"
	case routeScoop:
		return "run 'scoop uninstall crofty', then install from https://crofty.site — crofty no longer ships to Scoop"
	case routeWindowsClick:
		// Both Windows routes land in %LOCALAPPDATA%\crofty\bin — the click
		// installer and install.ps1 — so the path cannot tell them apart. Name the
		// installer: it overwrites this binary in place either way, and it is the
		// route that works. What we must not say is "re-run irm ... | iex": on a
		// real Windows box that pipeline died inside schannel, and an update notice
		// that sends someone back through a known failure is worse than silence.
		return "install crofty-setup.exe from https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty-setup.exe over this one"
	case routeScriptUser:
		// per-user install.sh target ($HOME/.local/bin): re-run the script
		return "re-run: curl -fsSL https://crofty.site/install.sh | sh"
	case routePkgDarwin:
		// Send .pkg users back to the .pkg: they came in without opening a
		// terminal, and install.sh replaces the binary only, leaving the .pkg's
		// Hugo behind for hugobin.Resolve to keep preferring over PATH forever
		// after. (This is only reached when self-update can't run; `crofty update`
		// swaps this body itself.)
		return "install crofty.pkg from https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty.pkg over this one"
	case routeScriptSystem:
		// system-wide install.sh target (PREFIX=/usr/local): re-run it the same way
		return "re-run: curl -fsSL https://crofty.site/install.sh | sudo PREFIX=/usr/local sh"
	case routeDebRpm:
		// /usr/bin on Linux means the old .deb/.rpm, which crofty no longer builds.
		// No repo ever stood behind them, so nothing will tell these users to move:
		// this notice is the only place they hear it.
		return "run 'sudo apt remove crofty' (or 'sudo dnf remove crofty'), then install from https://crofty.site — crofty no longer ships a .deb/.rpm"
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
