package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/spec"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit structured JSON (for tools)")
	fs.Usage = func() {
		fmt.Println("crofty validate — check content against the crofty spec (v0)")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty validate [path ...]   files or directories; default: ./content")
		fmt.Println("\nFlags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	targets := fs.Args()
	var contentRoot string
	if len(targets) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		proj, err := project.Find(cwd)
		if err != nil {
			return err
		}
		contentRoot = filepath.Join(proj.Root, "content")
		targets = []string{contentRoot}
	}

	files, err := collectMarkdown(targets)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no Markdown files found in: %s", strings.Join(targets, ", "))
	}

	reports := make([]spec.FileReport, 0, len(files))
	anyError := false
	for _, f := range files {
		r := spec.ValidateFile(f, contentRoot)
		reports = append(reports, r)
		if !r.OK {
			anyError = true
		}
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			OK    bool              `json:"ok"`
			Files []spec.FileReport `json:"files"`
		}{OK: !anyError, Files: reports}); err != nil {
			return err
		}
	} else {
		renderHuman(reports)
	}

	if anyError {
		// Non-zero exit so validate can gate build/publish, but the report above
		// is the message — suppress the generic "crofty: ..." wrapper.
		return errSilent
	}
	return nil
}

// collectMarkdown expands files and directories into a sorted, de-duplicated
// list of Markdown files, skipping tool/output directories.
func collectMarkdown(targets []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for _, t := range targets {
		fi, err := os.Stat(t)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			add(t)
			continue
		}
		err = filepath.WalkDir(t, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case ".crofty", "dist", "public", "node_modules":
					return filepath.SkipDir
				}
				return nil
			}
			if isMarkdown(p) {
				add(p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func isMarkdown(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

func renderHuman(reports []spec.FileReport) {
	totErr, totWarn := 0, 0
	for _, r := range reports {
		if len(r.Issues) == 0 {
			fmt.Printf("✓ %s\n", relCwd(r.File))
			continue
		}
		fmt.Println(relCwd(r.File))
		for _, is := range r.Issues {
			mark, label := severityDisplay(is.Severity)
			fmt.Printf("  %s %-5s %s — %s\n", mark, label, is.Field, is.FixHint)
			switch is.Severity {
			case spec.SeverityError:
				totErr++
			case spec.SeverityWarn:
				totWarn++
			}
		}
	}

	fmt.Println()
	if totErr == 0 && totWarn == 0 {
		fmt.Printf("✓ all good — %s\n", countLabel(len(reports), "file"))
		return
	}
	fmt.Printf("%s, %s across %s\n",
		countLabel(totErr, "error"), countLabel(totWarn, "warning"), countLabel(len(reports), "file"))
	fmt.Println("\nFix these by hand, or hand the notes to any assistant you use.")
}

func severityDisplay(s spec.Severity) (mark, label string) {
	switch s {
	case spec.SeverityError:
		return "✗", "error"
	case spec.SeverityWarn:
		return "⚠", "warn"
	default:
		return "·", "info"
	}
}

func countLabel(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// relCwd shortens a path relative to the working directory for display.
func relCwd(p string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}
