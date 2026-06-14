package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/secret"
)

func runDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	var account string
	var reauth bool
	fs.StringVar(&account, "account", "", "Cloudflare account id to deploy to (when a token reaches several)")
	fs.BoolVar(&reauth, "reauth", false, "enter a new Cloudflare API token (replace the saved one)")
	fs.Usage = func() {
		fmt.Println("crofty deploy — publish your site to Cloudflare Pages")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty deploy                 # first run asks for a Cloudflare API token (kept in your keychain)")
		fmt.Println("  crofty deploy --reauth        # replace the saved token")
		fmt.Println("  crofty deploy --account <id>  # pick the account when a token reaches several")
	}
	if _, err := parseArgs(fs, args); err != nil {
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

	// Authenticate with crofty's OWN Cloudflare token (kept in the keychain). On
	// the first deploy this asks the user for a token; crofty then talks to the
	// Cloudflare API directly (no wrangler, no Node) using that single credential.
	token, acct, proceed, err := connectCloudflare(proj, cfg, account, reauth)
	if err != nil {
		return err
	}
	if !proceed {
		return nil // choices were shown; nothing is deployed until the user picks
	}
	fmt.Println()
	if acct.name != "" {
		fmt.Printf("Deploying to Cloudflare account: %s (%s)\n", acct.name, acct.id)
	} else {
		fmt.Printf("Deploying to Cloudflare account: %s\n", acct.id)
	}
	fmt.Printf("  project: %s\n", cfg.Deploy.Project)

	// The production branch to deploy to. Pinning this makes deploy target the
	// live site: the deployment's branch matches the project's production branch,
	// so Cloudflare treats it as production rather than a preview.
	branch := cfg.Deploy.Branch
	if branch == "" {
		branch = "main"
	}

	// Upload dist/ straight to the Pages API (no wrangler, no Node). Nothing else
	// — keys, .crofty/, config — is ever uploaded.
	url, err := cfDeployDir(token, acct.id, cfg.Deploy.Project, branch, proj.DistDir(), func(line string) {
		fmt.Println("  " + line)
	})
	if err != nil {
		return fmt.Errorf("deploy failed — your site and Markdown are untouched; fix the issue and retry: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ deployed", cfg.Deploy.Project, "to Cloudflare Pages")
	if url != "" {
		fmt.Println("  live at →", url)
	}
	return nil
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
// stored) on the first deploy or when --reauth is set. proceed is false when it
// printed account choices and is waiting for the user to pick one with --account.
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

// pickAccount resolves the account a token deploys to: an explicit --account or
// the pinned one (verified reachable), the sole account the token lists, or — when
// several or none are listed — a prompt for the account id. ok is false when it
// printed a choice and is waiting for --account.
func pickAccount(token string, cfg *project.Config, accountFlag string) (cfAccount, bool, error) {
	if want := accountFlag; want != "" {
		if err := cfVerifyPagesAccess(token, want); err != nil {
			return cfAccount{}, false, err
		}
		return cfAccount{id: want}, true, nil
	}
	if cfg.Deploy.AccountID != "" {
		if err := cfVerifyPagesAccess(token, cfg.Deploy.AccountID); err != nil {
			return cfAccount{}, false, err
		}
		return cfAccount{id: cfg.Deploy.AccountID}, true, nil
	}

	accts, err := cfListAccounts(token)
	switch {
	case err == nil && len(accts) == 1:
		return accts[0], true, nil
	case err == nil && len(accts) > 1:
		fmt.Println()
		fmt.Println("That token reaches several Cloudflare accounts:")
		for _, a := range accts {
			fmt.Printf("    %s  (%s)\n", a.id, a.name)
		}
		fmt.Println()
		fmt.Println("Pick one:  crofty deploy --account <id>")
		return cfAccount{}, false, nil
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
