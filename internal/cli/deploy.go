package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/secret"
)

func runDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	var account string
	var reauth bool
	var skipBuild bool
	var yes bool
	fs.StringVar(&account, "account", "", "Cloudflare account id to deploy to (when a token reaches several)")
	fs.BoolVar(&reauth, "reauth", false, "enter new credentials (replace the saved token / password)")
	fs.BoolVar(&skipBuild, "skip-build", false, "publish the existing ./dist as-is, without rebuilding (e.g. CI built it)")
	fs.BoolVar(&yes, "yes", false, "trust an unknown SFTP host key on first use without the y/N prompt")
	fs.BoolVar(&yes, "y", false, "trust an unknown SFTP host key on first use without the y/N prompt")
	fs.Usage = func() {
		fmt.Println("crofty deploy — build the current site and publish it to your deploy provider")
		fmt.Println("\nProviders (set at 'crofty init'): cloudflare (default), sftp, ftps")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty deploy                 # build, then publish (first run asks for credentials)")
		fmt.Println("  crofty deploy --skip-build    # publish the existing ./dist without rebuilding")
		fmt.Println("  crofty deploy --reauth        # replace saved credentials")
		fmt.Println("  crofty deploy --yes           # SFTP: trust an unknown host key on first use (no y/N prompt)")
		fmt.Println("  crofty deploy --account <id>  # Cloudflare: pick the account when a token reaches several")
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
		return fmt.Errorf("reading deploy config: %w", err)
	}
	provider := cfg.Deploy.Provider
	if provider == "" {
		provider = "cloudflare" // backwards-compatible default for early configs
	}
	if !isSupportedProvider(provider) {
		return fmt.Errorf("deploy provider %q is not supported (use one of: %s)", provider, strings.Join(supportedProviders(), ", "))
	}

	// Build the current source before publishing, so deploy can never ship a
	// stale ./dist left behind after an edit (the source moved on but ./dist
	// didn't). --skip-build opts out for callers that built separately (e.g. CI).
	if skipBuild {
		if _, err := os.Stat(proj.DistDir()); err != nil {
			return fmt.Errorf("no build output at %s — run 'crofty build' first, or drop --skip-build", proj.DistDir())
		}
	} else {
		// Ask once — before the build renders the footer — whether to keep the
		// "Made with crofty" line, so the very first published site already
		// reflects the choice. Interactive + undecided only; CI / agent deploys
		// are never asked and stay off (see maybeAskFooterCredit).
		if err := maybeAskFooterCredit(proj, cfg); err != nil {
			return err
		}
		fmt.Println("Building the current site before publishing…")
		if err := buildSite(proj); err != nil {
			return err
		}
		contentDir := filepath.Join(proj.Root, "content")
		warnDrafts(contentDir)
		warnFutureDated(contentDir, time.Now())
	}

	// Gate on the output contract before touching credentials or the network: a
	// build that dropped canonical links / the feed / etc. is blocked here, so a
	// broken site never goes live (08 §6). Warnings are shown but don't block.
	if err := contractGate(proj.DistDir()); err != nil {
		return err
	}

	// Resolve credentials (keychain / TTY prompt) and build the Deployer for the
	// configured provider. Nothing but dist/ is ever uploaded — keys, .crofty/,
	// and config stay local.
	deployer, onDone, err := resolveDeployer(provider, proj, cfg, account, reauth, yes)
	if err != nil {
		return err
	}
	if deployer == nil {
		return nil // a choice was printed (e.g. Cloudflare's "pick --account")
	}

	url, err := deployer.Deploy(proj.DistDir(), func(line string) {
		fmt.Println("  " + line)
	})
	if err != nil {
		return fmt.Errorf("deploy failed — your site and Markdown are untouched; fix the issue and retry: %w", err)
	}
	onDone(url)
	return nil
}

// resolveDeployer authenticates for the given provider and returns a ready
// Deployer plus an onDone callback that prints the provider's success message. A
// nil Deployer (with nil error) means a choice was printed and nothing should be
// deployed yet (Cloudflare's multi-account case).
func resolveDeployer(provider string, proj *project.Project, cfg *project.Config, account string, reauth, yes bool) (Deployer, func(url string), error) {
	switch provider {
	case "cloudflare":
		if cfg.Deploy.Project == "" {
			return nil, nil, fmt.Errorf("deploy.project is empty in %s", proj.ConfigPath())
		}
		// crofty's own Cloudflare token (kept in the keychain) talks to the Pages
		// API directly — no wrangler, no Node.
		token, acct, proceed, err := connectCloudflare(proj, cfg, account, reauth)
		if err != nil {
			return nil, nil, err
		}
		if !proceed {
			return nil, nil, nil // choices were shown; wait for the user to pick
		}
		fmt.Println()
		if acct.name != "" {
			fmt.Printf("Deploying to Cloudflare account: %s (%s)\n", acct.name, acct.id)
		} else {
			fmt.Printf("Deploying to Cloudflare account: %s\n", acct.id)
		}
		fmt.Printf("  project: %s\n", cfg.Deploy.Project)
		// Pinning the production branch makes Cloudflare treat this as production
		// rather than a preview, regardless of the local git branch.
		branch := cfg.Deploy.Branch
		if branch == "" {
			branch = "main"
		}
		d := &cloudflareDeployer{token: token, accountID: acct.id, project: cfg.Deploy.Project, branch: branch}
		return d, func(url string) {
			fmt.Println()
			fmt.Println("✓ deployed", cfg.Deploy.Project, "to Cloudflare Pages")
			if url != "" {
				fmt.Println("  live at →", url)
			}
			printCustomDomainHelp(cfg.Deploy.Project)
		}, nil

	case "sftp":
		d, err := connectSFTP(proj, cfg, reauth, yes)
		if err != nil {
			return nil, nil, err
		}
		fmt.Printf("\nDeploying over SFTP to %s@%s:%s\n", cfg.Deploy.User, cfg.Deploy.Host, cfg.Deploy.Path)
		return d, func(string) {
			fmt.Printf("\n✓ deployed to %s:%s over SFTP\n", cfg.Deploy.Host, cfg.Deploy.Path)
		}, nil

	case "ftps":
		d, err := connectFTPS(proj, cfg, reauth)
		if err != nil {
			return nil, nil, err
		}
		fmt.Printf("\nDeploying over FTPS to %s@%s:%s\n", cfg.Deploy.User, cfg.Deploy.Host, cfg.Deploy.Path)
		return d, func(string) {
			fmt.Printf("\n✓ deployed to %s:%s over FTPS\n", cfg.Deploy.Host, cfg.Deploy.Path)
		}, nil
	}
	return nil, nil, fmt.Errorf("deploy provider %q is not supported", provider)
}

// printCustomDomainHelp shows how to point a custom domain at the site once it's
// live — the step that has no CLI home and otherwise assumes Cloudflare-dashboard
// knowledge. There are two paths depending on where the owner runs DNS, and which
// one applies isn't something crofty can tell from here, so it names both.
func printCustomDomainHelp(projectName string) {
	fmt.Println()
	fmt.Println("Want your own domain (e.g. blog.example.com)? Two ways, by where your DNS lives:")
	fmt.Println("  · DNS on Cloudflare — add the domain in")
	fmt.Println("      Pages → your project → Custom domains; the DNS record is made for you.")
	fmt.Printf("  · DNS elsewhere — keep your provider and add one record:\n")
	fmt.Printf("      CNAME  blog  →  %s.pages.dev\n", projectName)
	fmt.Println("  More: https://developers.cloudflare.com/pages/configuration/custom-domains/")
}

// cfAccount is a Cloudflare account crofty can deploy to.
type cfAccount struct {
	name, id string
}

// cfSignupURL is where a user with no Cloudflare account can make a free one.
const cfSignupURL = "https://dash.cloudflare.com/sign-up"

// cfTokenStore keeps Cloudflare API tokens in the OS keychain, keyed by account
// id so projects sharing an account share one token. These are crofty's own
// tokens — wrangler's login is never used.
func cfTokenStore() *secret.Store { return secret.New("cloudflare") }

func savedCFToken(accountID string) (string, error) {
	return cfTokenStore().Get(accountID, "api_token")
}

func saveCFToken(accountID, token string) error {
	return cfTokenStore().Set(accountID, "api_token", token)
}

// connectCloudflare returns the token + account a deploy should use. It reuses
// the saved token for a pinned account, or runs the token flow (TTY, verified,
// stored) on the first deploy or when --reauth is set. proceed is false only when
// the user chose no account (e.g. cancelled the picker).
func connectCloudflare(proj *project.Project, cfg *project.Config, accountFlag string, reauth bool) (token string, acct cfAccount, proceed bool, err error) {
	// Fast path: a pinned account with a saved, still-valid token.
	if cfg.Deploy.AccountID != "" && accountFlag == "" && !reauth {
		if tok, e := savedCFToken(cfg.Deploy.AccountID); e == nil {
			if cfVerifyPagesAccess(tok, cfg.Deploy.AccountID) == nil {
				return tok, cfAccount{id: cfg.Deploy.AccountID}, true, nil
			}
			fmt.Println("Your saved Cloudflare token no longer works — let's set it again.")
			fmt.Println()
		}
	}

	// Get a token (interactive, TTY-only — a secret never comes through an agent).
	tok, e := promptCFToken()
	if e != nil {
		return "", cfAccount{}, false, e
	}

	chosen, ok, e := pickAccount(tok, cfg, accountFlag)
	if e != nil || !ok {
		return "", cfAccount{}, false, e
	}

	if err := saveCFToken(chosen.id, tok); err != nil {
		return "", cfAccount{}, false, err
	}
	cfg.Deploy.AccountID = chosen.id
	if err := proj.SaveConfig(cfg); err != nil {
		return "", cfAccount{}, false, err
	}
	return tok, chosen, true, nil
}

// pickAccount resolves the account a token deploys to: an explicit --account, the
// pinned account when the token can still reach it, the sole account the token
// lists, an interactive pick among several, or a prompt for the account id when
// the token can't list any.
//
// A pinned account the token can NO LONGER reach is deliberately not a hard error:
// the common case is a fresh token for a *different* Cloudflare account because
// the user is moving the site. Forcing them to rerun with --account (a flag most
// people never discover) is the wrong answer — instead we fall through to account
// discovery and re-pin to whatever the token can actually use. pickAccount is only
// ever reached after the interactive token prompt, so stdin is a terminal and we
// can ask the user to choose. ok is false only when nothing was chosen.
func pickAccount(token string, cfg *project.Config, accountFlag string) (cfAccount, bool, error) {
	// An explicit --account always wins.
	if want := accountFlag; want != "" {
		if err := cfVerifyPagesAccess(token, want); err != nil {
			return cfAccount{}, false, err
		}
		return cfAccount{id: want}, true, nil
	}

	// Reuse the pinned account when this token can still manage Pages there.
	pinned := cfg.Deploy.AccountID
	if pinned != "" {
		if err := cfVerifyPagesAccess(token, pinned); err == nil {
			return cfAccount{id: pinned}, true, nil
		}
		// The token can't reach the pinned account — almost always a new token for
		// a new account (moving the site). Don't dead-end; find one it can use.
		fmt.Println()
		fmt.Printf("This token can't manage Pages on this site's saved account (%s).\n", pinned)
		fmt.Println("That's expected if you're moving the site to a different Cloudflare")
		fmt.Println("account. Finding an account this token can use…")
	}

	accts, err := cfListAccounts(token)
	switch {
	case err == nil && len(accts) == 1:
		a := accts[0]
		if pinned != "" && a.id != pinned {
			fmt.Printf("→ Moving this site to account %s (%s).\n", a.id, a.name)
		}
		return a, true, nil
	case err == nil && len(accts) > 1:
		return chooseAccount(accts, pinned)
	}

	// The token can't list accounts (a Pages-only token often can't) — ask for
	// the account id and verify the token can manage Pages there.
	id, perr := promptAccountID()
	if perr != nil {
		return cfAccount{}, false, perr
	}
	if verr := cfVerifyPagesAccess(token, id); verr != nil {
		return cfAccount{}, false, verr
	}
	return cfAccount{id: id}, true, nil
}

// chooseAccount asks the user to pick from the accounts a token can reach, by
// number — so it works without the user knowing the --account flag. It is only
// reached after the interactive token prompt, so stdin is a terminal.
func chooseAccount(accts []cfAccount, pinned string) (cfAccount, bool, error) {
	fmt.Println()
	fmt.Println("This token reaches several Cloudflare accounts:")
	for i, a := range accts {
		marker := ""
		if a.id == pinned {
			marker = "  ← current"
		}
		fmt.Printf("  %d) %s  (%s)%s\n", i+1, a.id, a.name, marker)
	}
	r := bufio.NewReader(os.Stdin)
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Printf("Which account? [1-%d]: ", len(accts))
		line, err := r.ReadString('\n')
		if n, ok := parseMenuChoice(line, len(accts)); ok {
			return accts[n-1], true, nil
		}
		if err != nil {
			break
		}
		fmt.Println("  (enter a number from the list)")
	}
	return cfAccount{}, false, fmt.Errorf("no account chosen — run the command again to pick one")
}

// parseMenuChoice reads a 1-based menu selection in [1,max] from a line of input.
func parseMenuChoice(line string, max int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > max {
		return 0, false
	}
	return n, true
}

// promptCFToken guides the user to a Pages: Edit token and reads it from a hidden
// TTY prompt — never via a flag or stdin pipe, so the secret never passes through
// an assistant's context (same rule as `targets add`).
func promptCFToken() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("crofty needs a Cloudflare API token to publish, and a token must be typed in a terminal — never through an assistant.\n"+
			"  Run 'crofty deploy' yourself and paste the token when asked.\n"+
			"  Create one: https://dash.cloudflare.com/profile/api-tokens\n"+
			"    → Create Token → Custom token → Permissions: Account · Cloudflare Pages · Edit\n"+
			"  No Cloudflare account yet? Free sign-up: %s", cfSignupURL)
	}
	fmt.Println("To publish, crofty needs a Cloudflare API token. It's kept in your keychain")
	fmt.Println("and used only to deploy your site — crofty has no server of its own.")
	fmt.Println()
	fmt.Println("  Create one:  https://dash.cloudflare.com/profile/api-tokens")
	fmt.Println("               → Create Token → Custom token → Get started")
	fmt.Println("               → Permissions: Account · Cloudflare Pages · Edit")
	fmt.Println("               → Continue to summary → Create Token")
	fmt.Printf("  No account?  Free sign-up: %s\n", cfSignupURL)
	fmt.Println()
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Print("Paste your Cloudflare API token: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		if tok := strings.TrimSpace(string(b)); tok != "" {
			return tok, nil
		}
		fmt.Println("  (nothing entered — try again)")
	}
	return "", fmt.Errorf("no token entered — run 'crofty deploy' again when you have one")
}

// promptAccountID reads a (non-secret) Cloudflare account id from the terminal,
// used when the token itself can't tell crofty which account it belongs to (a
// Pages-only token can't list accounts). The user may paste the bare id or the
// whole dashboard URL — crofty pulls the 32-hex id out of either.
func promptAccountID() (string, error) {
	fmt.Println()
	fmt.Println("crofty couldn't read the account from that token — normal for a Pages-only")
	fmt.Println("token. Open https://dash.cloudflare.com; the address bar shows your account id:")
	fmt.Println("    https://dash.cloudflare.com/<account-id>/...")
	fmt.Println("Paste that id — or just paste the whole URL and crofty will find it:")
	fmt.Println()
	fmt.Print("  Account id (or dashboard URL): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if m := regexp.MustCompile(`[0-9a-fA-F]{32}`).FindString(line); m != "" {
		return strings.ToLower(m), nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("that didn't contain a 32-character account id — copy it from the dashboard URL")
}

// canonicalPagesURL turns a per-deploy alias (https://<hash>.<sub>.pages.dev),
// whose wildcard cert isn't valid until Cloudflare provisions it, into the
// canonical https://<sub>.pages.dev, which is valid immediately. Used as a
// fallback when the deployment response carries no stable *.pages.dev alias.
func canonicalPagesURL(out string) string {
	m := regexp.MustCompile(`https://[a-z0-9.-]+\.pages\.dev`).FindString(out)
	if m == "" {
		return ""
	}
	labels := strings.Split(strings.TrimPrefix(m, "https://"), ".")
	if len(labels) >= 4 { // [hash, sub, pages, dev] → [sub, pages, dev]
		labels = labels[1:]
	}
	return "https://" + strings.Join(labels, ".")
}
