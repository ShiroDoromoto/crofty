package cli

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

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

	// Resolve and surface which Cloudflare account this deploys to, so a deploy
	// is never silently sent to whatever account wrangler happens to be logged
	// into. The chosen account is pinned to config on the first deploy.
	acct, err := resolveAccount(proj, cfg, bin, base)
	if err != nil {
		return err
	}
	fmt.Println()
	if acct.name != "" {
		fmt.Printf("Deploying to Cloudflare account: %s (%s)\n", acct.email, acct.name)
	} else {
		fmt.Printf("Deploying to Cloudflare account: %s\n", acct.email)
	}
	fmt.Printf("  account id: %s\n", acct.id)
	fmt.Printf("  project:    %s\n", cfg.Deploy.Project)

	// The production branch to deploy to. Pinning this makes deploy target the
	// live site regardless of the local git branch (Wrangler otherwise infers a
	// preview deployment from the current branch).
	branch := cfg.Deploy.Branch
	if branch == "" {
		branch = "main"
	}

	// Pin the account explicitly for every wrangler call rather than relying on
	// ambient auth, so a multi-account token can't deploy to the wrong place.
	env := []string{"CLOUDFLARE_ACCOUNT_ID=" + acct.id}

	// First deploy: make sure the Pages project exists. This is idempotent — on
	// later deploys it already exists, so we run it quietly and ignore that.
	_, _ = runner.CaptureEnv(proj.Root, env, bin, append(append([]string{}, base...),
		"pages", "project", "create", cfg.Deploy.Project,
		"--production-branch", branch)...)

	// Publish dist/. Nothing else (keys, .crofty/, config) is ever uploaded.
	out, err := runner.RunTee(proj.Root, env, bin, append(append([]string{}, base...),
		"pages", "deploy", proj.DistDir(),
		"--project-name", cfg.Deploy.Project,
		"--branch", branch,
		"--commit-dirty=true")...)
	if err != nil {
		return fmt.Errorf("deploy failed — your site and Markdown are untouched; fix the issue and retry: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ deployed", cfg.Deploy.Project, "to Cloudflare Pages")
	if url := canonicalPagesURL(out); url != "" {
		fmt.Println("  live at →", url)
	}
	return nil
}

// cfAccount is a Cloudflare account wrangler is authenticated for.
type cfAccount struct {
	email, name, id string
}

// resolveAccount reads the logged-in Cloudflare account(s) from wrangler and
// picks the one to deploy to: the config-pinned account if set (erroring on a
// drift), otherwise the sole logged-in account (which it pins). It guides the
// user to log in if wrangler isn't authenticated, and refuses to guess when a
// token spans multiple accounts.
func resolveAccount(proj *project.Project, cfg *project.Config, bin string, base []string) (cfAccount, error) {
	out, err := runner.Capture(proj.Root, bin, append(append([]string{}, base...), "whoami")...)
	low := strings.ToLower(out)
	if err != nil || strings.Contains(low, "not authenticated") || strings.Contains(low, "not logged in") {
		return cfAccount{}, fmt.Errorf("not connected to Cloudflare yet.\n" +
			"  Connect your (free) account first:  wrangler login\n" +
			"  Then run 'crofty deploy' again.")
	}

	email := ""
	if m := regexp.MustCompile(`email\s+(\S+)`).FindStringSubmatch(out); len(m) == 2 {
		email = strings.TrimRight(m[1], ".") // the line ends "…@host.tld." — drop the period
	}
	// Each account row carries a 32-hex id; the cell beside it is the name.
	accounts := parseAccounts(out)
	if len(accounts) == 0 {
		return cfAccount{}, fmt.Errorf("could not read your Cloudflare account from 'wrangler whoami'.\n" +
			"  Try 'wrangler login' again, then retry.")
	}

	// Pinned: must still be reachable by the current login.
	if cfg.Deploy.AccountID != "" {
		for _, a := range accounts {
			if a.id == cfg.Deploy.AccountID {
				a.email = email
				return a, nil
			}
		}
		return cfAccount{}, fmt.Errorf("this project is pinned to Cloudflare account %s, but the current wrangler login (%s) can't reach it.\n"+
			"  Log in to the right account with 'wrangler login', or change deploy.accountId in %s to retarget on purpose.",
			cfg.Deploy.AccountID, email, proj.ConfigPath())
	}

	// Not pinned yet: only proceed when the choice is unambiguous.
	if len(accounts) > 1 {
		var ids []string
		for _, a := range accounts {
			ids = append(ids, fmt.Sprintf("%s (%s)", a.id, a.name))
		}
		return cfAccount{}, fmt.Errorf("wrangler is logged into multiple Cloudflare accounts:\n  %s\n"+
			"  Pick one by setting deploy.accountId in %s, then run 'crofty deploy'.",
			strings.Join(ids, "\n  "), proj.ConfigPath())
	}
	acct := accounts[0]
	acct.email = email

	// First deploy: pin the account so a later re-login can't silently retarget.
	cfg.Deploy.AccountID = acct.id
	if err := proj.SaveConfig(cfg); err != nil {
		return cfAccount{}, err
	}
	return acct, nil
}

// parseAccounts pulls (id, name) pairs out of a `wrangler whoami` table.
func parseAccounts(out string) []cfAccount {
	idRe := regexp.MustCompile(`[0-9a-f]{32}`)
	seen := map[string]bool{}
	var accounts []cfAccount
	for _, line := range strings.Split(out, "\n") {
		id := idRe.FindString(line)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := ""
		for _, cell := range strings.Split(line, "│") {
			cell = strings.TrimSpace(cell)
			if cell != "" && cell != id {
				name = cell
				break
			}
		}
		accounts = append(accounts, cfAccount{name: name, id: id})
	}
	return accounts
}

// canonicalPagesURL extracts the project's production *.pages.dev URL from
// wrangler's deploy output. Wrangler prints the per-deploy alias
// (https://<hash>.<sub>.pages.dev), whose wildcard cert isn't valid until
// Cloudflare provisions it; dropping the hash label yields the canonical
// https://<sub>.pages.dev, which is valid immediately.
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
