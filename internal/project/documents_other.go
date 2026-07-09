//go:build !windows

package project

import (
	"os"
	"path/filepath"
)

// documentsDir is ~/Documents on macOS and Linux.
func documentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents"), nil
}
