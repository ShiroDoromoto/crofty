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
	var yes bool
	var account string
	fs.BoolVar(&yes, "yes", false, "confirm the Cloudflare account and deploy")
	fs.StringVar(&account, "account", "", "Cloudflare account id to deploy to (pins it to this project)")
	fs.Usage = func() {
		fmt.Println("crofty deploy — publish ./dist to your Cloudflare Pages project")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty deploy                 # first run shows the account and asks to confirm")
		fmt.Println("  crofty deploy --yes           # confirm the shown account and deploy")
		fmt.Println("  crofty deploy --account <id>  # deploy to a specific account (pins it)")
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

	bin, base := wranglerCmd()
	if bin == "" {
		return fmt.Errorf("wrangler not found.\n" +
			"crofty uses Wrangler to deploy to Cloudflare Pages. Install Node.js (which provides npx), then retry.")
	}

	// Resolve which Cloudflare account this deploys to. On the first deploy this
	// surfaces the account and waits for confirmation rather than silently using
	// whatever wrangler happens to be logged into.
	acct, err := chooseAccount(proj, cfg, bin, base, yes, account)
	if err != nil {
		return err
	}
	if acct == nil {
		return nil // a plan was shown; nothing is deployed until the user confirms
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

// cfSignupURL is where a user with no Cloudflare account can make a free one.
const cfSignupURL = "https://dash.cloudflare.com/sign-up"

// chooseAccount decides which Cloudflare account a deploy targets, surfacing the
// choice instead of silently inheriting whatever wrangler is logged into.
//   - returns (&acct, nil) to proceed (a pinned account, or one just confirmed);
//   - returns (nil, nil) after printing a plan, when the first deploy needs the
//     user to confirm the account (nothing is deployed);
//   - returns (nil, err) when it can't proceed (not connected, unreachable, …).
func chooseAccount(proj *project.Project, cfg *project.Config, bin string, base []string, yes bool, accountFlag string) (*cfAccount, error) {
	out, err := runner.Capture(proj.Root, bin, append(append([]string{}, base...), "whoami")...)
	low := strings.ToLower(out)
	if err != nil || strings.Contains(low, "not authenticated") || strings.Contains(low, "not logged in") {
		return nil, fmt.Errorf("not connected to Cloudflare yet.\n"+
			"  Connect an account:     wrangler login\n"+
			"  No account yet? Free:   %s\n"+
			"  Then run 'crofty deploy' again.", cfSignupURL)
	}

	email := accountEmail(out)
	accounts := parseAccounts(out)
	if len(accounts) == 0 {
		return nil, fmt.Errorf("could not read your Cloudflare account from 'wrangler whoami'.\n" +
			"  Try 'wrangler login' again, then retry.")
	}

	// Explicit --account: deploy to that one (and pin it), if the login reaches it.
	if accountFlag != "" {
		for _, a := range accounts {
			if a.id == accountFlag {
				a.email = email
				return pinAccount(proj, cfg, a)
			}
		}
		return nil, fmt.Errorf("the current wrangler login (%s) can't reach account %q.\n"+
			"  Log in to it with 'wrangler login', or pick one it can reach.", email, accountFlag)
	}

	// Already pinned: just confirm the login can still reach it, then deploy.
	if cfg.Deploy.AccountID != "" {
		for _, a := range accounts {
			if a.id == cfg.Deploy.AccountID {
				a.email = email
				return &a, nil
			}
		}
		return nil, fmt.Errorf("this project is pinned to Cloudflare account %s, but the current wrangler login (%s) can't reach it.\n"+
			"  Log in to the right account with 'wrangler login', or run 'crofty deploy --account <id>' to retarget on purpose.",
			cfg.Deploy.AccountID, email)
	}

	// First deploy, confirmed, and the account is unambiguous → pin and proceed.
	if yes && len(accounts) == 1 {
		a := accounts[0]
		a.email = email
		return pinAccount(proj, cfg, a)
	}

	// Otherwise surface the account(s) and the choices, and stop without deploying.
	printDeployPlan(email, accounts)
	return nil, nil
}

// pinAccount records the chosen account on the project and returns it.
func pinAccount(proj *project.Project, cfg *project.Config, a cfAccount) (*cfAccount, error) {
	cfg.Deploy.AccountID = a.id
	if err := proj.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return &a, nil
}

// printDeployPlan shows which account a first deploy would use and how to
// confirm, switch, or create one — so the account is always a deliberate choice.
func printDeployPlan(email string, accounts []cfAccount) {
	fmt.Println()
	if len(accounts) == 1 {
		a := accounts[0]
		fmt.Println("crofty would deploy to this Cloudflare account:")
		line := "    " + a.id
		if a.name != "" {
			line += "  (" + a.name + ")"
		}
		if email != "" {
			line += "  [" + email + "]"
		}
		fmt.Println(line)
		fmt.Println()
		fmt.Println("Is this the right account?")
		fmt.Println("  confirm    → crofty deploy --yes")
		fmt.Println("  different  → wrangler login   (switch accounts), then retry")
		fmt.Printf("  no account → create a free one: %s\n", cfSignupURL)
		return
	}
	fmt.Println("wrangler is logged into multiple Cloudflare accounts:")
	for _, a := range accounts {
		fmt.Printf("    %s  (%s)\n", a.id, a.name)
	}
	fmt.Println()
	fmt.Println("Pick the one to deploy to:  crofty deploy --account <id>")
}

// accountEmail pulls the logged-in email out of `wrangler whoami` (blank when
// authenticated by API token, which has no associated email).
func accountEmail(out string) string {
	if m := regexp.MustCompile(`email\s+(\S+)`).FindStringSubmatch(out); len(m) == 2 {
		return strings.TrimRight(m[1], ".")
	}
	return ""
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
