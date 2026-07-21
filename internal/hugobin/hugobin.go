// Package hugobin decides which Hugo binary crofty runs.
//
// crofty wraps Hugo, and the theme it ships is frozen against a known Hugo: the
// extended build (the stylesheets are SCSS), of a recent enough generation. A
// `hugo` on PATH proves neither. It can be years old, it can be the plain build,
// it can be an unrelated program that happens to share the name — and crofty
// cannot tell any of that apart from the one it was tested against.
//
// So the click installers, which exist for authors who have no Hugo at all,
// bundle the Hugo crofty was tested against, and that copy wins. They never
// write over an existing hugo: the bundled copy sits next to crofty and is found
// from the running executable, never from PATH (D-3). PATH is the last resort,
// for the installs that carry no Hugo of their own.
//
// The "running executable" is the real body, not whatever PATH entry launched
// it. A click install (D-339) keeps the body in a user-writable place and drops
// a link for it on PATH; os.Executable() on macOS reports that link, so crofty
// has to follow it to the body before it looks beside itself — otherwise it
// searches the shared system dir the link lives in and picks up the wrong Hugo.
package hugobin

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnvOverride points crofty at a Hugo of the author's choosing, ahead of
// everything else — the way out when crofty picks the wrong one.
const EnvOverride = "CROFTY_HUGO"

const missing = "hugo not found.\n" +
	"crofty wraps Hugo to build your site. Install the extended build\n" +
	"(https://gohugo.io/installation/), then run the command again.\n" +
	"If you already have one somewhere crofty can't see, name it: " + EnvOverride + "=/path/to/hugo"

// Resolve returns the Hugo executable crofty should run, looking in three
// places in turn:
//
//  1. $CROFTY_HUGO, when set — the author said which one, so nothing else is consulted
//  2. the copy a click installer bundled next to crofty
//  3. hugo on PATH — how a package manager or a manual install supplies it
func Resolve() (string, error) {
	// A crofty that cannot locate itself still has PATH to fall back on, so a
	// failure here is not fatal — it only means there is no bundled copy to find.
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	return resolve(os.Getenv(EnvOverride), exe, runtime.GOOS)
}

func resolve(override, exe, goos string) (string, error) {
	// An override that doesn't run is an error, never a reason to quietly fall
	// through: the author would go on believing crofty uses the Hugo they named.
	if override != "" {
		if !executable(override, goos) {
			return "", fmt.Errorf("%s is set to %q, but that is not a program crofty can run.\n"+
				"Point it at a hugo binary, or unset it to let crofty find one itself.", EnvOverride, override)
		}
		return override, nil
	}
	if exe != "" {
		if bin := bundled(resolveLink(exe), goos); executable(bin, goos) {
			return bin, nil
		}
	}
	if bin, err := exec.LookPath("hugo"); err == nil {
		return bin, nil
	}
	return "", errors.New(missing)
}

// Bundled reports whether a click installer's copy of Hugo sits next to the
// crofty binary at exe. It is exported because that copy is also the only thing
// that distinguishes the two macOS install routes: the .pkg and install.sh both
// put crofty in /usr/local/bin, but only the .pkg leaves a Hugo behind.
func Bundled(exe, goos string) bool {
	if exe == "" {
		return false
	}
	return executable(bundled(resolveLink(exe), goos), goos)
}

// resolveLink follows an entry link to the real file behind it, so the bundled
// Hugo is sought beside crofty's body rather than beside the link that launched
// it. When the path resolves to nothing — a crofty that cannot find its own
// file — the original is handed back and the bundled lookup simply misses,
// leaving PATH to answer. A direct launch resolves to itself, so this is a
// no-op there.
func resolveLink(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// bundled locates the Hugo a click installer put next to crofty. The two layouts
// differ because the install trees do: on Windows the installer owns
// %LOCALAPPDATA%\crofty\bin outright, so hugo.exe just sits beside crofty.exe.
// On macOS crofty lands in /usr/local/bin, a directory shared with every other
// program — so the bundled Hugo goes to /usr/local/libexec/crofty/ and stays off
// PATH, where it cannot shadow a hugo the author installed themselves.
func bundled(exe, goos string) string {
	dir := filepath.Dir(exe)
	if goos == "windows" {
		return filepath.Join(dir, "hugo.exe")
	}
	return filepath.Join(dir, "..", "libexec", "crofty", "hugo")
}

func executable(path string, goos string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return fi.Mode().Perm()&0o111 != 0
}
