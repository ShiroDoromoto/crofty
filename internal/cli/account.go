package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/shirodoromoto/crofty/internal/project"
)

// runConnect saves (or replaces) the Cloudflare API token crofty uses to publish
// this project — the same token flow as the first deploy, but without deploying.
// Use it to set up auth ahead of time or to swap in a new token.
func runConnect(args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	account := fs.String("account", "", "Cloudflare account id (when a token reaches several)")
	fs.Usage = func() {
		fmt.Println("crofty connect — save the Cloudflare API token used to publish this site")
		fmt.Println("\nUsage: crofty connect [--account <id>]")
		fmt.Println("\nReplaces any saved token. Run it from inside your project.")
	}
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}

	proj, err := currentProject()
	if err != nil {
		return err
	}
	cfg, err := proj.LoadConfig()
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	provider := cfg.Deploy.Provider
	if provider == "" {
		provider = "cloudflare"
	}
	if !isSupportedProvider(provider) {
		return fmt.Errorf("deploy provider %q is not supported (use one of: %s)", provider, strings.Join(supportedProviders(), ", "))
	}

	// reauth=true forces a fresh credential prompt even if one is already saved.
	switch provider {
	case "cloudflare":
		_, acct, proceed, err := connectCloudflare(proj, cfg, *account, true)
		if err != nil {
			return err
		}
		if !proceed {
			return nil // a choice was printed (e.g. pick --account)
		}
		fmt.Println()
		fmt.Printf("✓ Connected to Cloudflare account %s — token saved to your keychain.\n", acct.id)
	case "sftp":
		if _, err := connectSFTP(proj, cfg, true); err != nil {
			return err
		}
		fmt.Printf("\n✓ Saved SFTP credentials for %s@%s to your keychain.\n", cfg.Deploy.User, cfg.Deploy.Host)
	case "ftps":
		if _, err := connectFTPS(proj, cfg, true); err != nil {
			return err
		}
		fmt.Printf("\n✓ Saved FTPS credentials for %s@%s to your keychain.\n", cfg.Deploy.User, cfg.Deploy.Host)
	default:
		// isSupportedProvider() passed but no arm handled it — a provider was
		// added to supportedProviders() without a connect handler. Fail loudly
		// rather than print success having saved nothing.
		return fmt.Errorf("internal: deploy provider %q has no connect handler", provider)
	}
	fmt.Println("  Run 'crofty deploy' to publish.")
	return nil
}

// runReset removes the credentials and state crofty has saved (the Cloudflare
// token) from the OS keychain. With --all it does this
// for every known project and removes the global registry too, for a clean
// uninstall. It never touches your writing under ~/Documents/Crofty.
func runReset(args []string) error {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	all := fs.Bool("all", false, "every project's saved credentials + global state (for uninstall)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	fs.Usage = func() {
		fmt.Println("crofty reset — remove crofty's saved credentials (keychain) and state")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty reset          # this project's saved credentials")
		fmt.Println("  crofty reset --all    # every project's + global state (before uninstalling)")
		fmt.Println("\nYour writing under ~/Documents/Crofty is never touched.")
	}
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}

	type target struct {
		cfg  *project.Config
		root string
	}
	var targets []target
	if *all {
		for _, root := range project.KnownProjects() {
			if c, err := (&project.Project{Root: root}).LoadConfig(); err == nil {
				targets = append(targets, target{c, root})
			}
		}
	} else {
		proj, err := currentProject()
		if err != nil {
			return err
		}
		c, err := proj.LoadConfig()
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		targets = append(targets, target{c, proj.Root})
	}

	// Describe what will be removed before touching anything.
	var items []string
	for _, t := range targets {
		for _, d := range projectSecretDescriptions(t.cfg) {
			items = append(items, fmt.Sprintf("%s  (%s)", d, t.root))
		}
	}
	if *all {
		items = append(items, "global state (project registry)")
	}
	if len(items) == 0 {
		fmt.Println("Nothing saved to remove.")
		return nil
	}

	fmt.Println("This removes from your keychain / state:")
	for _, it := range items {
		fmt.Println("  -", it)
	}
	fmt.Println("\nYour writing under ~/Documents/Crofty is NOT touched.")

	if !*yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("re-run with --yes to confirm (no terminal to prompt)")
		}
		fmt.Print("\nRemove these? [y/N]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	for _, t := range targets {
		forgetProjectSecrets(t.cfg)
	}
	if *all {
		if dir, err := project.GlobalDir(); err == nil {
			_ = os.RemoveAll(dir)
		}
	}

	fmt.Println()
	fmt.Println("✓ Removed. Your writing is untouched.")
	if *all {
		fmt.Println("  You can now 'brew uninstall crofty' for a clean removal.")
	}
	return nil
}

// projectSecretDescriptions lists the saved secrets a config implies, for the
// confirmation summary.
func projectSecretDescriptions(c *project.Config) []string {
	var out []string
	switch c.Deploy.Provider {
	case "sftp":
		if c.Deploy.Host != "" && c.Deploy.User != "" {
			out = append(out, "SFTP credentials ("+c.Deploy.User+"@"+c.Deploy.Host+")")
		}
	case "ftps":
		if c.Deploy.Host != "" && c.Deploy.User != "" {
			out = append(out, "FTPS password ("+c.Deploy.User+"@"+c.Deploy.Host+")")
		}
	default: // "" or "cloudflare"
		if c.Deploy.AccountID != "" {
			out = append(out, "Cloudflare token (account "+c.Deploy.AccountID+")")
		}
	}
	return out
}

// forgetProjectSecrets deletes a project's saved secrets from the keychain.
// Absent entries are not an error.
func forgetProjectSecrets(c *project.Config) {
	switch c.Deploy.Provider {
	case "sftp":
		if c.Deploy.Host != "" && c.Deploy.User != "" {
			t := c.Deploy.Host + ":" + c.Deploy.User
			_ = sftpSecretStore().Delete(t, "password")
			_ = sftpSecretStore().Delete(t, "key_passphrase")
		}
	case "ftps":
		if c.Deploy.Host != "" && c.Deploy.User != "" {
			_ = ftpsSecretStore().Delete(c.Deploy.Host+":"+c.Deploy.User, "password")
		}
	default: // "" or "cloudflare"
		if c.Deploy.AccountID != "" {
			_ = cfTokenStore().Delete(c.Deploy.AccountID, "api_token")
		}
	}
}
