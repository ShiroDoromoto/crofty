package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// runConnect sets where this project publishes and saves the matching
// credentials. With no flags it re-saves the current backend's secret (the token
// flow of the first deploy, without deploying). With --provider it switches the
// deploy backend (cloudflare/sftp/ftps) first — so changing where a site
// publishes no longer means hand-editing .crofty/config.json — then saves that
// backend's credentials.
func runConnect(args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	account := fs.String("account", "", "Cloudflare account id (when a token reaches several)")
	providerFlag := fs.String("provider", "", "switch deploy backend: cloudflare, sftp, or ftps")
	host := fs.String("host", "", "sftp/ftps: server hostname")
	user := fs.String("user", "", "sftp/ftps: login user")
	pathFlag := fs.String("path", "", "sftp/ftps: remote web root to upload into (e.g. /public_html)")
	port := fs.Int("port", 0, "sftp/ftps: server port (default 22 for sftp, 21 for ftps)")
	keyFlag := fs.String("key", "", "sftp: path to an SSH private key (default: password auth)")
	fs.Usage = func() {
		fmt.Println("crofty connect — set where this site publishes and save its credentials")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty connect                        # re-save the current backend's credentials")
		fmt.Println("  crofty connect --account <id>         # Cloudflare: pick the account")
		fmt.Println("  crofty connect --provider sftp  --host h --user u --path /var/www/site")
		fmt.Println("  crofty connect --provider ftps  --host h --user u --path /public_html")
		fmt.Println("  crofty connect --provider cloudflare  # switch back to Cloudflare Pages")
		fmt.Println("\nSwitching the backend rewrites .crofty/config.json's deploy section and")
		fmt.Println("forgets the old backend's saved secret. Run it from inside your project.")
	}
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	cfg, err := proj.LoadConfig()
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	current := cfg.Deploy.Provider
	if current == "" {
		current = "cloudflare"
	}
	// Optionally switch the deploy backend (or update sftp/ftps fields) before we
	// save credentials, so `connect` is also how you change where a site publishes.
	if err := reconfigureDeploy(proj, cfg, current, *providerFlag, initDeployFlags{
		provider: *providerFlag, host: *host, user: *user, path: *pathFlag, port: *port, key: *keyFlag,
	}); err != nil {
		return err
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
		if _, err := connectSFTP(proj, cfg, true, false); err != nil {
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

// reconfigureDeploy switches the project's deploy backend (and/or updates the
// sftp/ftps destination fields) before `connect` saves credentials — so changing
// where a site publishes no longer means hand-editing .crofty/config.json. It is
// a no-op unless --provider names a different backend, or sftp/ftps field flags
// are given. Switching backends rewrites the deploy config and forgets the old
// backend's saved secret so it can't linger in the keychain.
func reconfigureDeploy(proj *project.Project, cfg *project.Config, current, providerFlag string, f initDeployFlags) error {
	target := current
	if p := strings.ToLower(strings.TrimSpace(providerFlag)); p != "" {
		target = p
	}
	if !isSupportedProvider(target) {
		return fmt.Errorf("unknown deploy provider %q (use one of: %s)", target, strings.Join(supportedProviders(), ", "))
	}

	switching := target != current
	serverFields := f.host != "" || f.user != "" || f.path != "" || f.port != 0 || f.key != ""
	updatingServer := serverFields && (target == "sftp" || target == "ftps")
	if !switching && !updatingServer {
		return nil // nothing to change; connect will re-auth the current backend
	}

	old := *cfg // snapshot so we can forget the previous backend's secret

	var nd project.DeployConfig
	switch target {
	case "cloudflare":
		// Keep the existing project name (and the pinned account, when we're only
		// updating, not switching); fall back to a URL-safe folder name.
		slug := cfg.Deploy.Project
		if slug == "" {
			slug = sanitizeName(filepath.Base(proj.Root))
		}
		nd = project.DeployConfig{Provider: "cloudflare", Project: slug}
		if !switching {
			nd.AccountID = cfg.Deploy.AccountID
		}
	default: // sftp / ftps — seed from existing values so a partial flag set only changes what's given
		port := f.port
		if port == 0 {
			port = cfg.Deploy.Port
		}
		stdin := bufio.NewReader(os.Stdin)
		interactive := term.IsTerminal(int(os.Stdin.Fd()))
		dc, err := chooseDeploy(stdin, interactive, "", initDeployFlags{
			provider: target,
			host:     firstNonEmpty(f.host, cfg.Deploy.Host),
			user:     firstNonEmpty(f.user, cfg.Deploy.User),
			path:     firstNonEmpty(f.path, cfg.Deploy.Path),
			port:     port,
			key:      firstNonEmpty(f.key, cfg.Deploy.KeyPath),
		})
		if err != nil {
			return err
		}
		nd = dc
	}

	if switching {
		forgetProjectSecrets(&old)
	}
	cfg.Deploy = nd
	if err := proj.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving deploy config: %w", err)
	}

	if switching {
		fmt.Printf("Deploy backend: %s → %s (saved to .crofty/config.json)\n", current, target)
	} else {
		fmt.Printf("Updated %s destination (saved to .crofty/config.json)\n", target)
	}
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
		proj, err := findProject()
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
		fmt.Println("  Only the crofty program is left — remove it the way you installed it.")
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
