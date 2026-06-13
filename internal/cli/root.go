package cli

import (
	"errors"
	"fmt"
	"os"
)

// Version is the crofty CLI version, bumped by hand.
const Version = "0.0.2-m2"

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
		{"build", "Render the site to ./dist with Hugo", runBuild},
		{"deploy", "Publish ./dist to your Cloudflare Pages project", runDeploy},
		{"validate", "Check content against the crofty spec (v0)", runValidate},
		{"publish", "Syndicate fragments to platforms (M3)", runPublish},
		{"eject", "Convert to a plain Hugo project (later)", runEject},
	}
}

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
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
				if !errors.Is(err, errSilent) {
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
