package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirEntries returns the names of the entries directly under dir.
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(es))
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

// classifyInstall is the single route classification both the upgrade nudge and
// `crofty update` read, so it must map the same paths TestUpgradeHintFor covers
// to the routes update then branches on — the two can never be allowed to drift.
func TestClassifyInstall(t *testing.T) {
	cases := []struct {
		exe, goos   string
		bundledHugo bool
		want        installRoute
	}{
		{"/opt/homebrew/Cellar/crofty/0.9.0/bin/crofty", "darwin", false, routeHomebrew},
		{"/usr/local/Cellar/crofty/0.9.0/bin/crofty", "darwin", false, routeHomebrew},
		{`C:\Users\me\scoop\apps\crofty\current\crofty.exe`, "windows", false, routeScoop},
		{"/usr/bin/crofty", "linux", false, routeDebRpm},
		{"/home/me/go/bin/crofty", "linux", false, routeUnknown}, // go install
		{"/somewhere/odd/crofty", "darwin", false, routeUnknown},
		{"", "darwin", false, routeUnknown}, // self-location failed
		{"/Users/me/.local/bin/crofty", "darwin", false, routeScriptUser},
		{"/home/me/.local/bin/crofty", "linux", false, routeScriptUser},
		{"/usr/local/bin/crofty", "linux", false, routeScriptSystem},
		{"/usr/local/bin/crofty", "darwin", false, routeScriptSystem}, // install.sh, no bundled Hugo
		{"/usr/local/bin/crofty", "darwin", true, routePkgDarwin},     // .pkg body: Hugo alongside
		{`C:\Users\me\AppData\Local\crofty\bin\crofty.exe`, "windows", false, routeWindowsClick},
	}
	for _, c := range cases {
		if got := classifyInstall(c.exe, c.goos, c.bundledHugo); got != c.want {
			t.Errorf("classifyInstall(%q, %q, %v) = %d; want %d", c.exe, c.goos, c.bundledHugo, got, c.want)
		}
	}
}

// selfUpdates decides whether the nudge says "crofty update" or the manual,
// route-back way (D-326): the .pkg / Windows / per-user-script routes self-update;
// dead channels, a root-owned system-wide install, and the unknown fallback do not.
func TestSelfUpdates(t *testing.T) {
	self := map[installRoute]bool{
		routePkgDarwin:    true,
		routeWindowsClick: true,
		routeScriptUser:   true,
		routeScriptSystem: false,
		routeHomebrew:     false,
		routeScoop:        false,
		routeDebRpm:       false,
		routeUnknown:      false,
	}
	for route, want := range self {
		if got := route.selfUpdates(); got != want {
			t.Errorf("route %d selfUpdates() = %v; want %v", route, got, want)
		}
	}
}

func TestManifestPlatformKey(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "macos-arm64"},
		{"darwin", "amd64", "macos-x64"},
		{"linux", "amd64", "linux-x64"},
		{"linux", "arm64", "linux-arm64"},
		{"windows", "amd64", "windows-x64"},
		{"plan9", "amd64", ""}, // an OS the release doesn't ship
		{"darwin", "386", ""},  // an arch the release doesn't ship
	}
	for _, c := range cases {
		if got := manifestPlatformKey(c.goos, c.goarch); got != c.want {
			t.Errorf("manifestPlatformKey(%q, %q) = %q; want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("the update payload")
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])
	checksums := good + "  crofty-body-darwin-universal.tar.gz\n" +
		"0000000000000000000000000000000000000000000000000000000000000000  other.zip\n"

	if err := verifyChecksum(data, "crofty-body-darwin-universal.tar.gz", checksums); err != nil {
		t.Errorf("verifyChecksum on a matching hash = %v; want nil", err)
	}
	// Wrong bytes for a listed asset: a mismatch, never a silent pass.
	if err := verifyChecksum([]byte("tampered"), "crofty-body-darwin-universal.tar.gz", checksums); err == nil {
		t.Error("verifyChecksum on tampered data = nil; want a mismatch error")
	}
	// An asset the file never listed was never signed for — as much a failure.
	if err := verifyChecksum(data, "not-listed.tar.gz", checksums); err == nil {
		t.Error("verifyChecksum for an unlisted asset = nil; want an error")
	}
}

// tarEntry is one file to pack into an in-memory .tar.gz for the tests below.
type tarEntry struct {
	data []byte
	mode int64
}

func makeTarGz(t *testing.T, files map[string]tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, e := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: e.mode, Size: int64(len(e.data)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestFileFromTarGz covers the install.sh route's archive, which is flat: the
// crofty binary sits beside LICENSE/README, and update pulls just the binary out.
func TestFileFromTarGz(t *testing.T) {
	archive := makeTarGz(t, map[string]tarEntry{
		"LICENSE":   {[]byte("license text"), 0o644},
		"README.md": {[]byte("readme"), 0o644},
		"crofty":    {[]byte("new-crofty-binary"), 0o755},
	})
	body, mode, err := fileFromTarGz(archive, "crofty")
	if err != nil {
		t.Fatalf("fileFromTarGz(crofty) error: %v", err)
	}
	if string(body) != "new-crofty-binary" {
		t.Errorf("extracted body = %q; want the crofty binary bytes", body)
	}
	if mode&0o111 == 0 {
		t.Errorf("extracted mode = %o; the binary must keep its executable bit", mode)
	}
	if _, _, err := fileFromTarGz(archive, "hugo"); err == nil {
		t.Error("fileFromTarGz(hugo) = nil error; want not-found (this archive carries no Hugo)")
	}
}

// TestSwapDarwinBody is the .pkg route's replacement, end to end but offline: a
// staged body tree replaces an installed one in a temp dir, so both the crofty
// binary and its bundled Hugo land — no network, no live release.
func TestSwapDarwinBody(t *testing.T) {
	root := t.TempDir()
	// Lay down an "installed" body with old contents.
	writeFileMode(t, filepath.Join(root, "bin", "crofty"), "old-crofty", 0o755)
	writeFileMode(t, filepath.Join(root, "libexec", "crofty", "hugo"), "old-hugo", 0o755)

	archive := makeTarGz(t, map[string]tarEntry{
		"bin/crofty":                      {[]byte("new-crofty"), 0o755},
		"libexec/crofty/hugo":             {[]byte("new-hugo"), 0o755},
		"libexec/crofty/LICENSE-hugo.txt": {[]byte("hugo license"), 0o644},
	})
	if err := swapDarwinBody(archive, root); err != nil {
		t.Fatalf("swapDarwinBody: %v", err)
	}

	if got := readFile(t, filepath.Join(root, "bin", "crofty")); got != "new-crofty" {
		t.Errorf("crofty after swap = %q; want new-crofty", got)
	}
	if got := readFile(t, filepath.Join(root, "libexec", "crofty", "hugo")); got != "new-hugo" {
		t.Errorf("hugo after swap = %q; want new-hugo (bundled Hugo must update too)", got)
	}
	// The staging dir must not linger beside the body.
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Errorf("staging dir %q left behind after swap", e.Name())
		}
	}
}

// TestExtractTarGzRejectsEscape guards the unpack against a crafted archive that
// tries to write outside the target with a climbing path.
func TestExtractTarGzRejectsEscape(t *testing.T) {
	archive := makeTarGz(t, map[string]tarEntry{
		"../escape": {[]byte("evil"), 0o644},
	})
	if err := extractTarGz(archive, t.TempDir()); err == nil {
		t.Error("extractTarGz accepted an entry climbing out of the target; want a refusal")
	}
}

func makeZip(t *testing.T, files map[string]tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, e := range files {
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.SetMode(os.FileMode(e.mode))
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestSwapWindowsBody is the Windows click-installer replacement, offline: the
// running crofty.exe is moved aside and the new one dropped in, and the neighbour
// hugo.exe is replaced too — no network, no live release. (File semantics let
// this run on any OS; the Windows-specific behaviour it stands in for is that a
// running .exe can be renamed but not overwritten.)
func TestSwapWindowsBody(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "crofty.exe")
	writeFileMode(t, exe, "old-crofty", 0o755)
	writeFileMode(t, filepath.Join(root, "hugo.exe"), "old-hugo", 0o755)

	archive := makeZip(t, map[string]tarEntry{
		"crofty.exe":       {[]byte("new-crofty"), 0o755},
		"hugo.exe":         {[]byte("new-hugo"), 0o755},
		"LICENSE-hugo.txt": {[]byte("hugo license"), 0o644},
	})
	if err := swapWindowsBody(archive, exe); err != nil {
		t.Fatalf("swapWindowsBody: %v", err)
	}
	if got := readFile(t, exe); got != "new-crofty" {
		t.Errorf("crofty.exe after swap = %q; want new-crofty", got)
	}
	if got := readFile(t, filepath.Join(root, "hugo.exe")); got != "new-hugo" {
		t.Errorf("hugo.exe after swap = %q; want new-hugo (bundled Hugo must update too)", got)
	}
	for _, e := range dirEntries(t, root) {
		if strings.HasSuffix(e, ".old") || strings.HasPrefix(e, ".crofty-update-") {
			t.Errorf("%q left behind after a clean swap", e)
		}
	}
}

// TestSwapWindowsBodyRollsBackOnFailure forces the hugo.exe replace to fail (its
// target is a non-empty directory), and checks the crofty.exe swap is undone —
// the original binary is restored, so a half-applied update never leaves a broken
// install behind.
func TestSwapWindowsBodyRollsBackOnFailure(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "crofty.exe")
	writeFileMode(t, exe, "old-crofty", 0o755)
	// hugo.exe is a non-empty directory: renaming a file over it must fail.
	writeFileMode(t, filepath.Join(root, "hugo.exe", "blocker"), "x", 0o644)

	archive := makeZip(t, map[string]tarEntry{
		"crofty.exe": {[]byte("new-crofty"), 0o755},
		"hugo.exe":   {[]byte("new-hugo"), 0o755},
	})
	if err := swapWindowsBody(archive, exe); err == nil {
		t.Fatal("swapWindowsBody succeeded despite an unreplaceable hugo.exe; want an error")
	}
	if got := readFile(t, exe); got != "old-crofty" {
		t.Errorf("crofty.exe after a failed swap = %q; want the original restored (old-crofty)", got)
	}
	for _, e := range dirEntries(t, root) {
		if strings.HasSuffix(e, ".old") || strings.HasPrefix(e, ".crofty-update-") {
			t.Errorf("%q left behind after rollback", e)
		}
	}
}

func TestExtractZipRejectsEscape(t *testing.T) {
	archive := makeZip(t, map[string]tarEntry{
		"../escape": {[]byte("evil"), 0o644},
	})
	if err := extractZip(archive, t.TempDir()); err == nil {
		t.Error("extractZip accepted an entry climbing out of the target; want a refusal")
	}
}

// TestReplaceFile checks the atomic in-place swap the install.sh route uses, and
// that a binary always lands executable even if the archive mode said otherwise.
func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "crofty")
	writeFileMode(t, target, "old", 0o755)

	if err := replaceFile(target, []byte("new"), 0o644); err != nil {
		t.Fatalf("replaceFile: %v", err)
	}
	if got := readFile(t, target); got != "new" {
		t.Errorf("after replaceFile = %q; want new", got)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("replaced binary mode = %o; want the executable bit set", fi.Mode().Perm())
	}
	// No stray temp files left in the directory.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries after replaceFile; want 1 (only the binary)", len(entries))
	}
}

func writeFileMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
