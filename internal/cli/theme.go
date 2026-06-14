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
	case "set":
		return runThemeSet(args[1:])
	case "-h", "--help", "help":
		themeUsage()
		return nil
	default:
		return fmt.Errorf("unknown theme subcommand %q (try: crofty theme eject | crofty theme set)", args[0])
	}
}

func themeUsage() {
	fmt.Println("crofty theme — bring the theme onto disk so you can customize it")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty theme set <name>     # apply a ready-made look (a token override)")
	fmt.Println("  crofty theme set            # list the looks crofty ships")
	fmt.Println("  crofty theme eject          # write the design tokens to assets/css/custom.css")
	fmt.Println("  crofty theme eject --full   # write the whole theme (layouts + CSS) into the project")
	fmt.Println()
	fmt.Println("Edit the ejected file(s), then 'crofty preview' to see the change.")
}

// runThemeSet applies a shipped preset by writing its tokens to custom.css — the
// same file `theme eject` writes, so a preset is just a pre-filled override the
// reader can then tweak.
func runThemeSet(args []string) error {
	fs := flag.NewFlagSet("theme set", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite custom.css even if you've hand-edited it")
	fs.Usage = func() {
		fmt.Println("crofty theme set <name> — apply a ready-made look")
		fmt.Println("\nUsage: crofty theme set [<name>] [--force]")
		fmt.Println("\nWith no name, lists the looks crofty ships. Each is a token override")
		fmt.Println("written to assets/css/custom.css, which you can then edit further.")
	}
	names, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	if len(names) == 0 {
		printPresets()
		return nil
	}
	name := names[0]
	preset, ok := theme.PresetByName(name)
	if !ok {
		fmt.Printf("crofty: no preset named %q.\n\n", name)
		printPresets()
		return errSilent
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	proj, err := project.Find(cwd)
	if err != nil {
		return err
	}

	target := filepath.Join(proj.Root, filepath.FromSlash(theme.CustomCSSPath))
	// Swapping presets is safe; clobbering hand-edits is not. Only block when the
	// existing custom.css is something the reader wrote themselves.
	if !*force {
		if existing, err := os.ReadFile(target); err == nil && !theme.IsShippedCSS(string(existing)) {
			fmt.Printf("%s has your own edits — 'theme set' would replace them.\n", theme.CustomCSSPath)
			fmt.Println("Re-run with --force to apply the preset anyway, or copy your edits aside first.")
			return errSilent
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(preset.CSS), 0o644); err != nil {
		return err
	}

	fmt.Printf("✓ applied the %q look → %s\n", name, theme.CustomCSSPath)
	fmt.Println()
	fmt.Println("It's a token override you can keep editing. To go back to the default")
	fmt.Println("theme, delete that file.")
	fmt.Println()
	fmt.Println("next:")
	fmt.Println("  crofty preview     # see it locally")
	return nil
}

func printPresets() {
	fmt.Println("Looks crofty ships (apply with 'crofty theme set <name>'):")
	fmt.Println()
	for _, p := range theme.Presets() {
		fmt.Printf("    %-14s %s\n", p.Name, p.Summary)
	}
	fmt.Println()
	fmt.Println("Each is a token override written to", theme.CustomCSSPath+";")
	fmt.Println("edit it afterwards, or delete it to return to the default theme.")
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
