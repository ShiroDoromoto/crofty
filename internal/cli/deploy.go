package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/runner"
)

func runDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("crofty deploy — publish ./dist to your Cloudflare Pages project")
		fmt.Println("\nUsage: crofty deploy")
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
	cfg, err := proj.LoadConfig()
	if err != nil {
		return fmt.Errorf("reading deploy config: %w", err)
	}
	if cfg.Deploy.Provider != "cloudflare" {
		return fmt.Errorf("deploy provider %q is not supported in M1 (only \"cloudflare\")", cfg.Deploy.Provider)
	}
	if cfg.Deploy.Project == "" {
		return fmt.Errorf("deploy.project is empty in %s", proj.ConfigPath())
	}

	// Deploy uploads ONLY ./dist. If there is no build, stop here rather than
	// risk publishing anything else from the project.
	if _, err := os.Stat(proj.DistDir()); err != nil {
		return fmt.Errorf("no build output at %s — run 'crofty build' first", proj.DistDir())
	}

	bin, base := wranglerCmd()
	if bin == "" {
		return fmt.Errorf("wrangler not found.\n" +
			"crofty uses Wrangler to deploy to Cloudflare Pages. Install Node.js (which provides npx), then retry.")
	}

	// First deploy: make sure the Pages project exists. This is idempotent — on
	// later deploys it already exists, so we run it quietly and ignore that.
	_, _ = runner.Capture(proj.Root, bin, append(append([]string{}, base...),
		"pages", "project", "create", cfg.Deploy.Project,
		"--production-branch", "main")...)

	// Publish dist/. Nothing else (keys, .crofty/, config) is ever uploaded.
	err = runner.Run(proj.Root, bin, append(append([]string{}, base...),
		"pages", "deploy", proj.DistDir(),
		"--project-name", cfg.Deploy.Project,
		"--commit-dirty=true")...)
	if err != nil {
		return fmt.Errorf("deploy failed — your site and Markdown are untouched; fix the issue and retry: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ deployed", cfg.Deploy.Project, "to Cloudflare Pages")
	return nil
}

// wranglerCmd resolves how to invoke Wrangler: a global binary if present,
// otherwise via npx. Returns ("", nil) if neither is available.
func wranglerCmd() (bin string, base []string) {
	if runner.Look("wrangler") {
		return "wrangler", nil
	}
	if runner.Look("npx") {
		return "npx", []string{"--yes", "wrangler"}
	}
	return "", nil
}
