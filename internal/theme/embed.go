// Package theme carries the default Hugo theme, embedded in the binary so a user
// only writes Markdown — they never have to fetch or wire up a theme. The
// embedded copy is the single source of truth: build materializes it fresh each
// run. A user's own project-level layouts/ still override it (Hugo's normal
// layering), which is the theme's intended override layer.
package theme

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:files
var embedded embed.FS

// FS returns the embedded theme tree rooted at the theme directory.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "files")
	if err != nil {
		panic(err) // the embedded path is a compile-time constant
	}
	return sub
}

// Materialize writes the embedded theme into dst, replacing any existing tree.
func Materialize(dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	src := FS()
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, p)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
