package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/runner"
	"github.com/shirodoromoto/crofty/internal/theme"
)

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("crofty build — render the site to ./dist")
		fmt.Println("\nUsage: crofty build")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	proj, err := project.Find(cwd)
	if err != nil {
		return err
	}

	if !runner.Look("hugo") {
		return fmt.Errorf("hugo not found on PATH.\n" +
			"crofty wraps Hugo to build your site. Install it (e.g. 'brew install hugo'), then run 'crofty build' again.")
	}

	// Materialize the bundled theme into .crofty/themes/crofty each build so the
	// copy embedded in the binary stays the single source of truth.
	themeDst := filepath.Join(proj.ThemesDir(), "crofty")
	if err := theme.Materialize(themeDst); err != nil {
		return fmt.Errorf("writing bundled theme: %w", err)
	}

	// Run Hugo against the project root. .crofty/ holds the theme and tool state
	// and is never rendered into the output, so nothing from it can ride along
	// to deploy.
	err = runner.Run(proj.Root, "hugo",
		"--source", proj.Root,
		"--themesDir", proj.ThemesDir(),
		"--theme", "crofty",
		"--destination", proj.DistDir(),
		"--cleanDestinationDir",
	)
	if err != nil {
		return fmt.Errorf("hugo build failed (your Markdown is untouched): %w", err)
	}

	fmt.Println()
	fmt.Println("✓ built →", proj.DistDir())
	fmt.Println("next: crofty deploy")
	return nil
}
