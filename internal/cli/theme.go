package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ShiroDoromoto/crofty/internal/access"
	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/theme"
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
	case "tokens":
		printThemeTokens()
		return nil
	case "-h", "--help", "help":
		themeUsage()
		return nil
	default:
		return fmt.Errorf("unknown theme subcommand %q (try: crofty theme tokens | set | eject)", args[0])
	}
}

// themeToken is one design variable the theme exposes — the editable surface a
// person or agent can change without touching layouts. Listed by `theme tokens`
// so you can see what's adjustable before ejecting anything.
type themeToken struct {
	Name    string
	Role    string
	Default string
}

// themeTokens is the catalogue `crofty theme tokens` prints. The names must match
// the :root variables in the bundled crofty.css (theme.CustomCSS); theme_test
// guards against drift.
func themeTokens() []themeToken {
	return []themeToken{
		{"--bg", "page background", "#fcfbf7"},
		{"--ink", "body text and headings", "#211e1a"},
		{"--muted", "dates, captions, secondary text", "#6f6a61"},
		{"--line", "rules, borders, link underlines", "#e4e0d8"},
		{"--accent", "links on hover, active marks", "#5c4b37"},
		{"--code-bg", "inline code and code blocks", "#f2efe7"},
		{"--measure", "reading-column width", "34rem"},
		{"--font-body", "reading column (body text)", "Charter, …, serif"},
		{"--font-chrome", "header / footer / meta face", "system-ui, sans-serif"},
		{"--font-mono", "code", "ui-monospace, …, monospace"},
	}
}

func printThemeTokens() {
	fmt.Println("Theme tokens — the variables you can change without touching layouts.")
	fmt.Println("Each follows light/dark; 'crofty theme eject' writes them to a file to edit.")
	fmt.Println()
	for _, t := range themeTokens() {
		fmt.Printf("    %-14s %-34s %s\n", t.Name, t.Role, t.Default)
	}
	fmt.Println()
	fmt.Println("next:")
	fmt.Println("  crofty theme eject          # write these to assets/css/custom.css to edit")
	fmt.Println("  crofty theme eject --print  # print them to stdout (don't touch any file)")
	fmt.Println("  crofty theme set <name>     # apply a ready-made palette instead")
}

func themeUsage() {
	fmt.Println("crofty theme — bring the theme onto disk so you can customize it")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty theme tokens         # list the colour/type tokens you can change")
	fmt.Println("  crofty theme set <name>     # apply a ready-made look (a token override)")
	fmt.Println("  crofty theme set            # list the looks crofty ships")
	fmt.Println("  crofty theme eject          # write the design tokens to assets/css/custom.css")
	fmt.Println("  crofty theme eject --print  # print the tokens to stdout (touch no file)")
	fmt.Println("  crofty theme eject --full   # write the whole theme (layouts + CSS) into the project")
	fmt.Println()
	fmt.Println("Customize in the smallest step that does the job:")
	fmt.Println("  colour / type      → tokens (theme set, or eject + edit custom.css)")
	fmt.Println("  a bit of extra CSS → assets/css/custom.css, or params.crofty.head_raw")
	fmt.Println("  one template       → override just that file under layouts/")
	fmt.Println("  everything         → theme eject --full (you then own it; no upstream updates)")
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

	proj, err := findProject()
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
	printOnly := fs.Bool("print", false, "print the tokens to stdout instead of writing a file")
	fs.Usage = func() {
		fmt.Println("crofty theme eject — make the theme editable")
		fmt.Println("\nUsage: crofty theme eject [--full] [--force] [--print]")
		fmt.Println("\nWithout --full, writes assets/css/custom.css: the design tokens")
		fmt.Println("(colour, type, reading width) with their defaults, ready to edit.")
		fmt.Println("This is the safe, documented way to restyle — 'crofty doctor' still")
		fmt.Println("guarantees your site owns its content no matter how you change it.")
		fmt.Println("\n--print writes nothing — useful when you already have a custom.css and")
		fmt.Println("just want the token block to copy from.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// --print needs no project and touches nothing: just emit the starter tokens.
	if *printOnly {
		fmt.Print(theme.CustomCSS)
		return nil
	}

	proj, err := findProject()
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
		return denyTokenWrite(target, err)
	}
	if err := os.WriteFile(target, []byte(theme.CustomCSS), 0o644); err != nil {
		return denyTokenWrite(target, err)
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

// denyTokenWrite turns a read-only project folder into a choice. The tokens are
// plain CSS, so there is a real way on that costs the author nothing — --print
// hands them the block to paste. Naming it here keeps an agent from inventing a
// worse one, like writing the file somewhere crofty never reads (D-1).
func denyTokenWrite(target string, err error) error {
	return access.Deny("write the design tokens to "+theme.CustomCSSPath, target, err,
		access.Choice{
			Do:         "let crofty write into the project folder, then run the command again",
			Command:    "crofty theme eject",
			Permission: "write access to " + filepath.Dir(target),
		},
		access.Choice{
			Do:      "print the tokens instead and paste them into your own CSS — writes nothing",
			Command: "crofty theme eject --print",
		},
	)
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
