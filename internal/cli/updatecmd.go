package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ShiroDoromoto/crofty/internal/hugobin"
)

// `crofty update` is the one way crofty updates itself (D-339). The author (or
// the AI driving crofty) runs it deliberately — there is no implicit auto-update
// at startup; the decision to update stays theirs. It swaps only the body in the
// user's home, never re-running the installer: a body crofty fetched itself
// carries no quarantine/Mark-of-the-Web flag, so Gatekeeper/SmartScreen stay
// quiet, and the entry link on PATH (root-owned, placed once at install) is
// never touched, so no update needs root again.
//
// Which install this is decides what update does. The macOS .pkg body, the
// Windows click-installer body and the per-user install.sh binary self-update
// here; every other route (dead channels, a root-owned system-wide install, or
// one crofty does not recognize) is handed back the same route-specific hint the
// upgrade nudge prints — classifyInstall is the single source both stand on, so
// they can never disagree about what kind of install this is.

const (
	// releaseDownloadBase is the GitHub release download root. Fixed-name assets
	// hang off /latest/download (no tag lookup needed); versioned ones off
	// /download/v<version>.
	releaseDownloadBase = "https://github.com/ShiroDoromoto/crofty/releases"

	// updateManifestURL is wharfy's per-release manifest — the latest version and
	// a per-platform archive URL. update reads it fresh (unlike the cached, silent
	// notifier) so a network problem surfaces as a real, classified error.
	updateManifestURL = releaseDownloadBase + "/latest/download/latest.json"

	// The .pkg / .exe bodies and their shared checksums have fixed names, so
	// /latest/download reaches them without first learning the version.
	updateBodyDarwinURL    = releaseDownloadBase + "/latest/download/crofty-body-darwin-universal.tar.gz"
	updateBodyWindowsURL   = releaseDownloadBase + "/latest/download/crofty-body-windows-amd64.zip"
	updateBodyChecksumsURL = releaseDownloadBase + "/latest/download/crofty-body-checksums.txt"
	updateBodyDarwinAsset  = "crofty-body-darwin-universal.tar.gz"
	updateBodyWindowsAsset = "crofty-body-windows-amd64.zip"

	// updateDownloadTimeout bounds the whole fetch. The bodies carry Hugo (~47MB),
	// so this is minutes — not the notifier's second-and-a-half.
	updateDownloadTimeout = 5 * time.Minute
)

// releaseManifest mirrors the fields update needs from wharfy's latest.json: the
// newest version and a download URL per platform key ("macos-arm64", "linux-x64").
type releaseManifest struct {
	Version string            `json:"version"`
	Assets  map[string]string `json:"assets"`
}

func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("crofty update — fetch the latest crofty and replace this one in place")
		fmt.Println("\nUsage:\n  crofty update")
		fmt.Println("\nUpdates the install you have (the macOS .pkg body, or the install.sh")
		fmt.Println("binary). Other routes are told how to update by hand.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// A source build (or `go install`) matches no release asset — Version is the
	// giveaway, left "dev" whenever the release ldflags were not applied. Refuse
	// rather than fetch a build that isn't this one.
	if Version == "dev" {
		fmt.Fprintln(os.Stderr, "crofty update updates a released binary, but this is a source build (version \"dev\").")
		fmt.Fprintln(os.Stderr, "To update a source build, pull and rebuild: git pull && go build .")
		return errSilent
	}

	exe := resolveSelfPath()
	bundled := hugobin.Bundled(exe, runtime.GOOS)
	switch classifyInstall(exe, runtime.GOOS, bundled) {
	case routePkgDarwin:
		return updateDarwinBody(exe)
	case routeWindowsClick:
		return updateWindowsBody(exe)
	case routeScriptUser:
		return updateScriptBinary(exe)
	default:
		// Every other route can't self-update here: dead channels (leave),
		// root-owned system-wide install (needs sudo), or one crofty doesn't
		// recognize. Hand back the route's own hint.
		fmt.Fprintf(os.Stderr, "crofty update can't update this install automatically.\nTo update: %s\n", upgradeHintFor(exe, runtime.GOOS, bundled))
		return errSilent
	}
}

// resolveSelfPath returns the real file behind the running binary, following the
// entry link a click install drops on PATH so the route (and the body to swap)
// is judged from where crofty actually lives — not from the link that launched
// it. An empty string on failure classifies as routeUnknown, which refuses
// safely.
func resolveSelfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		return real
	}
	return exe
}

// updateScriptBinary updates a per-user install.sh install: that route carries no
// Hugo, so it reuses wharfy's own binary archive and swaps just the crofty binary
// in place.
func updateScriptBinary(bin string) error {
	m, err := latestFromManifest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "crofty:", err)
		return errSilent
	}
	if !semverLess(Version, m.Version) {
		fmt.Printf("crofty is already up to date (%s).\n", Version)
		return nil
	}
	key := manifestPlatformKey(runtime.GOOS, runtime.GOARCH)
	url := m.Assets[key]
	if url == "" {
		fmt.Fprintf(os.Stderr, "crofty: the release has no archive for this platform (%s).\nUpdate by hand: %s\n", key, upgradeHintFor(bin, runtime.GOOS, false))
		return errSilent
	}
	// Fail fast on a directory we can't write, before a multi-MB download.
	if err := ensureWritable(filepath.Dir(bin)); err != nil {
		return updateWriteRefusal(filepath.Dir(bin), err)
	}
	fmt.Printf("Updating crofty %s → %s …\n", Version, m.Version)

	archive, err := fetchBytes(url)
	if err != nil {
		return downloadRefusal("the update", err)
	}
	sums, err := fetchBytes(scriptChecksumsURL(m.Version))
	if err != nil {
		return downloadRefusal("the checksums", err)
	}
	if err := verifyChecksum(archive, path.Base(url), string(sums)); err != nil {
		return checksumRefusal(err)
	}
	body, mode, err := fileFromTarGz(archive, "crofty")
	if err != nil {
		fmt.Fprintf(os.Stderr, "crofty: the update archive was unreadable (%v). Nothing was changed.\n", err)
		return errSilent
	}
	if err := replaceFile(bin, body, mode); err != nil {
		fmt.Fprintf(os.Stderr, "crofty: couldn't replace %s (%v). It may need write permission there.\n", bin, err)
		return errSilent
	}
	fmt.Printf("Updated to crofty %s. What changed: %s\n", m.Version, releaseNotesURL)
	return nil
}

// updateDarwinBody updates a macOS .pkg install: the body is a tree in the user's
// home (bin/crofty beside libexec/crofty/hugo), so update swaps both the binary
// and its bundled Hugo.
func updateDarwinBody(bin string) error {
	m, err := latestFromManifest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "crofty:", err)
		return errSilent
	}
	if !semverLess(Version, m.Version) {
		fmt.Printf("crofty is already up to date (%s).\n", Version)
		return nil
	}
	// bin is <root>/bin/crofty; the body root holds bin/ and libexec/crofty/.
	root := filepath.Dir(filepath.Dir(bin))
	if err := ensureWritable(root); err != nil {
		return updateWriteRefusal(root, err)
	}
	fmt.Printf("Updating crofty %s → %s …\n", Version, m.Version)

	archive, err := fetchBytes(updateBodyDarwinURL)
	if err != nil {
		return downloadRefusal("the update", err)
	}
	sums, err := fetchBytes(updateBodyChecksumsURL)
	if err != nil {
		return downloadRefusal("the checksums", err)
	}
	if err := verifyChecksum(archive, updateBodyDarwinAsset, string(sums)); err != nil {
		return checksumRefusal(err)
	}
	if err := swapDarwinBody(archive, root); err != nil {
		fmt.Fprintf(os.Stderr, "crofty: %v\n", err)
		return errSilent
	}
	fmt.Printf("Updated to crofty %s (Hugo refreshed too). What changed: %s\n", m.Version, releaseNotesURL)
	return nil
}

// swapDarwinBody extracts the body archive into a staging dir on the body's own
// filesystem, then renames each file over its target — Hugo and its licence
// first, the crofty binary last, so a write destined to fail fails before the
// binary is touched (a working crofty stays in place). Renames within one
// filesystem are atomic; the running process keeps its open inode, so replacing
// the file it is executing is safe on macOS/Linux. A full rollback of a
// mid-sequence failure is Windows's problem (3b), where an in-use .exe cannot be
// overwritten at all — here the writability pre-check makes it improbable.
func swapDarwinBody(archive []byte, root string) error {
	staging, err := os.MkdirTemp(root, ".crofty-update-")
	if err != nil {
		return fmt.Errorf("couldn't stage the update in %s (%v). Nothing was changed", root, err)
	}
	defer os.RemoveAll(staging)
	if err := extractTarGz(archive, staging); err != nil {
		return fmt.Errorf("the update archive was unreadable (%v). Nothing was changed", err)
	}
	order := []string{
		filepath.Join("libexec", "crofty", "hugo"),
		filepath.Join("libexec", "crofty", "LICENSE-hugo.txt"),
		filepath.Join("bin", "crofty"), // last: everything before it must land first
	}
	for _, rel := range order {
		src := filepath.Join(staging, rel)
		if _, err := os.Stat(src); err != nil {
			continue // the archive may legitimately omit a file (e.g. the licence)
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("couldn't prepare %s (%v)", filepath.Dir(dst), err)
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("couldn't replace %s (%v). It may need write permission there", dst, err)
		}
	}
	return nil
}

// updateWindowsBody updates a %LOCALAPPDATA%\crofty\bin install (the click
// installer / install.ps1). The body is flat — crofty.exe beside hugo.exe — so
// update swaps both. That directory is per-user, so no admin is needed; the extra
// work over the other routes is that a running .exe cannot be overwritten on
// Windows, only renamed out of the way (handled in swapWindowsBody).
func updateWindowsBody(exe string) error {
	m, err := latestFromManifest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "crofty:", err)
		return errSilent
	}
	if !semverLess(Version, m.Version) {
		fmt.Printf("crofty is already up to date (%s).\n", Version)
		return nil
	}
	dir := filepath.Dir(exe)
	if err := ensureWritable(dir); err != nil {
		return updateWriteRefusal(dir, err)
	}
	fmt.Printf("Updating crofty %s → %s …\n", Version, m.Version)

	archive, err := fetchBytes(updateBodyWindowsURL)
	if err != nil {
		return downloadRefusal("the update", err)
	}
	sums, err := fetchBytes(updateBodyChecksumsURL)
	if err != nil {
		return downloadRefusal("the checksums", err)
	}
	if err := verifyChecksum(archive, updateBodyWindowsAsset, string(sums)); err != nil {
		return checksumRefusal(err)
	}
	if err := swapWindowsBody(archive, exe); err != nil {
		fmt.Fprintf(os.Stderr, "crofty: %v\n", err)
		return errSilent
	}
	fmt.Printf("Updated to crofty %s (Hugo refreshed too). What changed: %s\n", m.Version, releaseNotesURL)
	return nil
}

// swapWindowsBody replaces the running crofty.exe and its neighbour hugo.exe from
// the body zip. A .exe that is executing can't be overwritten on Windows, but it
// can be renamed, so the running binary is moved aside (to .old) before the new
// one is dropped in; the process keeps running from the renamed file, and the new
// exe is read on the next launch. hugo.exe isn't running, so it's replaced
// directly. Every step past the first is undone on failure — the original exe is
// restored — so a half-applied update never leaves a broken install behind. The
// stale .old can't be deleted while the process holds it open, so it's cleared
// best-effort here and again at the start of the next update.
func swapWindowsBody(archive []byte, exe string) error {
	dir := filepath.Dir(exe)
	staging, err := os.MkdirTemp(dir, ".crofty-update-")
	if err != nil {
		return fmt.Errorf("couldn't stage the update in %s (%v). Nothing was changed", dir, err)
	}
	defer os.RemoveAll(staging)
	if err := extractZip(archive, staging); err != nil {
		return fmt.Errorf("the update archive was unreadable (%v). Nothing was changed", err)
	}

	newExe := filepath.Join(staging, "crofty.exe")
	if _, err := os.Stat(newExe); err != nil {
		return fmt.Errorf("the update archive carried no crofty.exe (%v). Nothing was changed", err)
	}

	backup := exe + ".old"
	os.Remove(backup) // clear a locked leftover from a previous update, best-effort

	// 1. Move the running exe aside. Renaming an in-use .exe is allowed on Windows.
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("couldn't move the running crofty aside (%v). Nothing was changed", err)
	}
	// 2. Drop the new exe where the old one was. On failure, restore the original.
	if err := os.Rename(newExe, exe); err != nil {
		os.Rename(backup, exe)
		return fmt.Errorf("couldn't place the new crofty (%v). The old one was kept", err)
	}
	// 3. Replace hugo.exe beside it — not running, so a direct replace is fine. On
	//    failure, undo the exe swap too (the new exe isn't in use, so it can go).
	newHugo := filepath.Join(staging, "hugo.exe")
	if _, err := os.Stat(newHugo); err == nil {
		if err := os.Rename(newHugo, filepath.Join(dir, "hugo.exe")); err != nil {
			os.Remove(exe)
			os.Rename(backup, exe)
			return fmt.Errorf("couldn't replace hugo.exe (%v). The old crofty was kept", err)
		}
	}
	os.Remove(backup) // best-effort; still open by the running process, so likely a no-op
	return nil
}

// extractZip unpacks every file in a zip under dest, creating parent directories
// and rejecting an entry whose path escapes dest. It is the Windows body's
// counterpart to extractTarGz (the .exe payload ships as a .zip, as the NSIS
// installer lays it).
func extractZip(data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		clean := path.Clean(f.Name)
		if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return fmt.Errorf("refusing archive entry outside the target: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o755 // a zip made on Windows carries no unix mode
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			rc.Close()
			return err
		}
		_, cErr := io.Copy(out, rc)
		rc.Close()
		if closeErr := out.Close(); cErr == nil {
			cErr = closeErr
		}
		if cErr != nil {
			return cErr
		}
	}
	return nil
}

// latestFromManifest fetches wharfy's release manifest and returns it, or a
// download error phrased for the author to read as-is.
func latestFromManifest() (releaseManifest, error) {
	var m releaseManifest
	data, err := fetchBytes(updateManifestURL)
	if err != nil {
		return m, fmt.Errorf("couldn't reach the release info (%v). Check your connection and try again", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("the release info didn't parse (%v). Try again shortly", err)
	}
	if m.Version == "" {
		return m, errors.New("the release info named no version. Try again shortly")
	}
	return m, nil
}

// scriptChecksumsURL builds the URL of the versioned checksums file that lists
// wharfy's binary archives (crofty_<version>_checksums.txt).
func scriptChecksumsURL(version string) string {
	return fmt.Sprintf("%s/download/v%s/crofty_%s_checksums.txt", releaseDownloadBase, version, version)
}

// manifestPlatformKey maps a Go OS/arch to the key wharfy uses in latest.json
// ("macos-arm64", "linux-x64", …). Empty for a pair the release doesn't ship.
func manifestPlatformKey(goos, goarch string) string {
	o := map[string]string{"darwin": "macos", "linux": "linux", "windows": "windows"}[goos]
	a := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if o == "" || a == "" {
		return ""
	}
	return o + "-" + a
}

// verifyChecksum confirms data's SHA-256 matches the entry for asset in a
// `shasum -a 256` checksums file (each line "<hex>  <name>"). A missing entry is
// as much a failure as a mismatch — an unlisted asset was never signed for.
func verifyChecksum(data []byte, asset, checksums string) error {
	want := ""
	for _, line := range strings.Split(checksums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == asset {
			want = strings.ToLower(f[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum listed for %s", asset)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s (got %s, want %s)", asset, got, want)
	}
	return nil
}

// fileFromTarGz returns the contents and mode of a single file (by its path
// inside the archive) from a gzip'd tar. It serves the install.sh route, whose
// archive is flat: the `crofty` binary sits beside LICENSE and README.
func fileFromTarGz(data []byte, name string) ([]byte, os.FileMode, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if path.Clean(h.Name) == name && h.Typeflag == tar.TypeReg {
			b, err := io.ReadAll(tr)
			return b, os.FileMode(h.Mode), err
		}
	}
	return nil, 0, fmt.Errorf("%s not found in the archive", name)
}

// extractTarGz unpacks every regular file in a gzip'd tar under dest, preserving
// modes and creating parent directories. An entry whose path escapes dest (an
// absolute path, or one climbing out with "..") is rejected rather than followed.
func extractTarGz(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := path.Clean(h.Name)
		if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return fmt.Errorf("refusing archive entry outside the target: %s", h.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(clean))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode).Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}

// replaceFile atomically replaces path with new contents at the given mode,
// writing a temp file in the same directory first so the rename stays within one
// filesystem (and a failure leaves the original untouched). Replacing the running
// binary's own file is safe on macOS/Linux: the executing process holds the old
// inode open, and the next launch reads the new one.
func replaceFile(path string, contents []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".crofty-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // a no-op once the rename moves it away
	if _, err := tmp.Write(contents); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if mode&0o111 == 0 {
		mode = 0o755 // a binary must stay executable, whatever the archive said
	}
	if err := os.Chmod(tmpName, mode.Perm()); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ensureWritable fails — before any slow download — when dir can't be written.
// The update replaces files there, and a per-user install crofty doesn't own is
// the case worth catching early, not after fetching tens of MB.
func ensureWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".crofty-writable-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// fetchBytes GETs a URL with a generous timeout (the bodies carry Hugo and run to
// tens of MB) and returns the whole body. Unlike the notifier's cached probe, the
// update path wants the real error so callers can classify a failure.
func fetchBytes(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "crofty/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// The three refusals below print a classified failure and return errSilent, so
// `crofty update` never fails silently and always exits non-zero when it did not
// update — the AI driving crofty can tell "updated" from "didn't" by the code.

func downloadRefusal(what string, err error) error {
	fmt.Fprintf(os.Stderr, "crofty: couldn't download %s (%v). Check your connection and try again.\n", what, err)
	return errSilent
}

func checksumRefusal(err error) error {
	fmt.Fprintf(os.Stderr, "crofty: %v — the download was not what the release signed for. Nothing was changed.\n", err)
	return errSilent
}

func updateWriteRefusal(path string, err error) error {
	fmt.Fprintf(os.Stderr, "crofty: can't write the update to %s (%v).\n", path, err)
	fmt.Fprintln(os.Stderr, "You may not own that location — re-run the installer, or update by hand.")
	return errSilent
}
