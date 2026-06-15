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

// runPreview serves the site locally with Hugo's dev server so anyone can see
// their site in a browser before connecting any account — the first win that
// needs no Cloudflare, no keys, nothing but the folder on this machine. It
// blocks, streaming Hugo's output, until the user presses Control-C.
func runPreview(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("crofty preview — see your site in a browser (local, no account)")
		fmt.Println("\nUsage: crofty preview")
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
			"crofty wraps Hugo to build your site. Install it (e.g. 'brew install hugo'), then try again.")
	}

	themeDst := filepath.Join(proj.ThemesDir(), "crofty")
	if err := theme.Materialize(themeDst); err != nil {
		return fmt.Errorf("writing bundled theme: %w", err)
	}

	fmt.Println("Starting a local preview of your site.")
	fmt.Println("Open the http://localhost link printed just below in your web browser.")
	fmt.Println("Edits to content and styles reload automatically. If a change to hugo.yaml")
	fmt.Println("(or, rarely, a stylesheet) doesn't show, stop with Control-C and run this again.")
	fmt.Println("When you're done looking, press Control-C here to stop.")
	fmt.Println()

	// hugo server blocks until interrupted; runner.Run streams its output. A
	// Control-C exit is the normal way to stop, not a crofty failure.
	// --disableFastRender makes every edit trigger a full rebuild: a touch slower,
	// but edits (including assets) reliably appear — important when an agent writes
	// a file and checks the result, where a stale render reads as a real failure.
	err = runner.Run(proj.Root, "hugo", "server",
		"--source", proj.Root,
		"--themesDir", proj.ThemesDir(),
		"--theme", "crofty",
		"--disableFastRender",
	)
	if err != nil {
		return fmt.Errorf("preview stopped: %w", err)
	}
	return nil
}
