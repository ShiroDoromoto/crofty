package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shirodoromoto/crofty/internal/project"
)

// Version is the crofty CLI version. Releases inject the git tag at build time
// via -ldflags -X (see .goreleaser.yaml), so the tag is the single source of
// truth; plain `go build` from source reports "dev".
var Version = "dev"

// errSilent lets a command signal a non-zero exit when it has already printed
// its own report (e.g. validate's findings), suppressing the generic wrapper.
var errSilent = errors.New("")

// command is one subcommand and its handler.
type command struct {
	name    string
	summary string
	run     func(args []string) error
}

func commands() []command {
	return []command{
		{"init", "Create a new project (a website you own)", runInit},
		{"features", "List what crofty can do and how to turn each on", runFeatures},
		{"add", "Turn on a capability (mermaid, abc, highlight, raw-html)", runAdd},
		{"lang", "Add or list the languages your site is written in", runLang},
		{"preview", "See your site in a browser (local, no account)", runPreview},
		{"build", "Render the site to ./dist with Hugo", runBuild},
		{"connect", "Save the Cloudflare API token used to deploy", runConnect},
		{"deploy", "Publish ./dist to your Cloudflare Pages project", runDeploy},
		{"reset", "Remove saved credentials (keychain) and state", runReset},
		{"validate", "Check content against the crofty spec (v0)", runValidate},
		{"doctor", "Check the built site against the output contract", runDoctor},
		{"share", "Print a ready-to-post fragment (text + link) for any SNS", runShare},
		{"theme", "Bring the theme onto disk to customize (eject tokens or full)", runTheme},
		{"eject", "Convert to a plain Hugo project (later)", runEject},
	}
}

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		// A bare `crofty` is the cwd-independent entry point: it lists the
		// author's projects (with absolute paths) so an agent started anywhere,
		// in any session, can find them and continue — or points a first-timer at
		// `crofty init` (07 O3).
		discover()
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return 0
	case "-v", "--version", "version":
		fmt.Println("crofty", Version)
		return 0
	}
	for _, c := range commands() {
		if c.name == args[0] {
			if err := c.run(args[1:]); err != nil {
				switch {
				case errors.Is(err, errSilent):
					// command already printed its own report
				case errors.Is(err, project.ErrNotFound):
					// Turn the dead end into a doorway. A first-timer can't process
					// much here, so emphasize one single next step.
					fmt.Fprintln(os.Stderr, "\nThere's no crofty project here yet.")
					fmt.Fprintln(os.Stderr, "\nTo start one, type this and press Enter:")
					fmt.Fprintln(os.Stderr, "\n    crofty init")
					fmt.Fprintln(os.Stderr, "\n(Already have a folder of Markdown? Run 'crofty init .' inside it.)")
				default:
					fmt.Fprintf(os.Stderr, "\ncrofty: %v\n", err)
				}
				return 1
			}
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "crofty: unknown command %q\n\n", args[0])
	usage()
	return 2
}

// findProject locates the crofty project containing the current directory, the
// common preamble for commands that operate on "the project here".
func findProject() (*project.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.Find(cwd)
}

// parseArgs parses a flag set while allowing flags and positional arguments to
// be interspersed (stdlib flag stops at the first positional). It returns the
// positional arguments in order.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// discover prints the author's known crofty projects with absolute paths, or
// guides a first-timer to `crofty init`. It reads crofty's own global registry,
// not the current directory, so it works from anywhere across sessions.
func discover() {
	fmt.Println("crofty — write Markdown; build and deploy a site you own.")
	fmt.Println()
	projects := project.KnownProjects()
	if len(projects) == 0 {
		fmt.Println("No crofty projects yet.")
		fmt.Println()
		fmt.Println("To start one, type this and press Enter:")
		fmt.Println()
		fmt.Println("    crofty init")
		fmt.Println()
		fmt.Println("It creates your site under ~/Documents/Crofty/ and prints the exact path.")
		fmt.Println()
		fmt.Println("Curious what crofty can do first? Run 'crofty features'.")
		return
	}
	fmt.Println("Your crofty projects:")
	for _, p := range projects {
		fmt.Printf("    %-16s %s\n", filepath.Base(p), p)
	}
	fmt.Println()
	fmt.Println("→ To work on one, cd into it, e.g.:")
	fmt.Printf("    cd %s\n", projects[0])
	fmt.Println()
	fmt.Println("Then run 'crofty help' to see what you can do there.")
}

func usage() {
	fmt.Println("crofty — write Markdown; build and deploy a site you own.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, c := range commands() {
		fmt.Printf("  %-9s %s\n", c.name, c.summary)
	}
	fmt.Println()
	fmt.Println("Run 'crofty <command> -h' for command flags.")
}
