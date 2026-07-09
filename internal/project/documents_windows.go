//go:build windows

package project

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// documentsDir asks Windows where Documents is rather than assuming
// %USERPROFILE%\Documents. On a machine with OneDrive's Known Folder Move
// turned on — the default on many consumer and managed installs — Documents is
// %USERPROFILE%\OneDrive\Documents, and the assumed path is a folder that
// either does not exist or is not the one the author sees in Explorer.
//
// %USERPROFILE% still wins when it does not name the account's real profile:
// that is a test or a sandbox pinning home, and the known-folder API would
// answer for the account behind its back.
func documentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	fallback := filepath.Join(home, "Documents")

	profile, err := windows.KnownFolderPath(windows.FOLDERID_Profile, windows.KF_FLAG_DEFAULT)
	if err != nil || !sameDir(profile, home) {
		return fallback, nil
	}
	docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
	if err != nil || docs == "" {
		return fallback, nil
	}
	return docs, nil
}

// sameDir compares two Windows paths: separators and case don't distinguish
// them, and a trailing separator doesn't either.
func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
