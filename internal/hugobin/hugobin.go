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
		if bin := bundled(exe, goos); executable(bin, goos) {
			return bin, nil
		}
	}
	if bin, err := exec.LookPath("hugo"); err == nil {
		return bin, nil
	}
	return "", errors.New(missing)
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
