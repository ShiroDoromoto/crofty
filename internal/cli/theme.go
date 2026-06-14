package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/theme"
)

// runTheme groups commands that bring the bundled theme onto disk so it — and
// the customization an agent does — is visible and editable. The default theme
// is embedded in the binary; eject makes it concrete.
func runTheme(args []string) error {
	if len(args) == 0 {
		themeUsage()
		return nil
	}
	switch args[0] {
	case "eject":
		return runThemeEject(args[1:])
	case "-h", "--help", "help":
		themeUsage()
		return nil
	default:
		return fmt.Errorf("unknown theme subcommand %q (try: crofty theme eject)", args[0])
	}
}

func themeUsage() {
	fmt.Println("crofty theme — bring the theme onto disk so you can customize it")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty theme eject          # write the design tokens to assets/css/custom.css")
	fmt.Println("  crofty theme eject --full   # write the whole theme (layouts + CSS) into the project")
	fmt.Println()
	fmt.Println("Edit the ejected file(s), then 'crofty preview' to see the change.")
}

func runThemeEject(args []string) error {
	fs := flag.NewFlagSet("theme eject", flag.ContinueOnError)
	full := fs.Bool("full", false, "write the entire theme (layouts + CSS), not just the design tokens")
	force := fs.Bool("force", false, "overwrite files that already exist")
	fs.Usage = func() {
		fmt.Println("crofty theme eject — make the theme editable")
		fmt.Println("\nUsage: crofty theme eject [--full] [--force]")
		fmt.Println("\nWithout --full, writes assets/css/custom.css: the design tokens")
		fmt.Println("(colour, type, reading width) with their defaults, ready to edit.")
		fmt.Println("This is the safe, documented way to restyle — 'crofty doctor' still")
		fmt.Println("guarantees your site owns its content no matter how you change it.")
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

	if *full {
		return ejectFull(proj, *force)
	}
	return ejectTokens(proj, *force)
}

// ejectTokens writes the starter token override (assets/css/custom.css). It is
// the everyday path: a handful of variables an agent or a person can edit, that
// cannot break the output contract.
func ejectTokens(proj *project.Project, force bool) error {
	target := filepath.Join(proj.Root, filepath.FromSlash(theme.CustomCSSPath))
	if !force {
		if _, err := os.Stat(target); err == nil {
			fmt.Printf("%s already exists — edit it, or pass --force to reset it to defaults.\n", theme.CustomCSSPath)
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(theme.CustomCSS), 0o644); err != nil {
		return err
	}

	fmt.Println("✓ wrote", theme.CustomCSSPath)
	fmt.Println()
	fmt.Println("These design tokens load after the theme, so your edits win. Change the")
	fmt.Println("colours, fonts, or reading width and the whole site follows.")
	fmt.Println()
	fmt.Println("next:")
	fmt.Println("  crofty preview     # see your changes locally")
	fmt.Println("  crofty theme eject --full   # later, to edit layouts and markup too")
	return nil
}

// ejectFull writes the entire theme into the project so layouts and markup can
// be edited directly. The project stays a crofty project (build, doctor, deploy
// still work); the ejected files just override the bundled ones.
func ejectFull(proj *project.Project, force bool) error {
	written, skipped, err := theme.EjectFull(proj.Root, force)
	if err != nil {
		return fmt.Errorf("writing theme: %w", err)
	}

	if len(written) == 0 && len(skipped) > 0 {
		fmt.Println("The theme is already on disk — nothing written.")
		fmt.Printf("%d file(s) left untouched. Pass --force to reset them to the bundled theme.\n", len(skipped))
		return nil
	}

	fmt.Printf("✓ wrote %d theme file(s) into %s\n", len(written), proj.Root)
	for _, p := range written {
		fmt.Println("    " + p)
	}
	if len(skipped) > 0 {
		fmt.Printf("\n%d existing file(s) left untouched (use --force to overwrite).\n", len(skipped))
	}
	fmt.Println()
	fmt.Println("These override the bundled theme. Note: an ejected theme no longer")
	fmt.Println("tracks improvements from 'brew upgrade' — that's the trade for full control.")
	fmt.Println()
	fmt.Println("next:")
	fmt.Println("  crofty preview     # see your changes locally")
	fmt.Println("  crofty doctor      # confirm the output still meets the contract")
	return nil
}
