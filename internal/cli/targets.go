package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/shirodoromoto/crofty/internal/id"
	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/publish"
	"github.com/shirodoromoto/crofty/internal/secret"
)

// knownTargetTypes are the syndication destinations crofty can configure.
var knownTargetTypes = map[string]bool{"bluesky": true, "mastodon": true}

func runTargets(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		targetsUsage()
		return nil
	}
	switch args[0] {
	case "add":
		return targetsAdd(args[1:])
	case "list":
		return targetsList(args[1:])
	case "test":
		return targetsTest(args[1:])
	default:
		return fmt.Errorf("unknown targets subcommand %q (want: add | list | test)", args[0])
	}
}

func targetsUsage() {
	fmt.Println("crofty targets — manage syndication destinations (your own accounts)")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty targets add bluesky     add a destination (prompts for credentials)")
	fmt.Println("  crofty targets add mastodon    add a Mastodon account (instance + access token)")
	fmt.Println("  crofty targets list            list configured destinations")
	fmt.Println("  crofty targets test [name]     check stored credentials")
	fmt.Println()
	fmt.Println("Credentials are stored in your OS keychain and never sent anywhere of ours.")
}

func targetsAdd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: crofty targets add <bluesky|mastodon>")
	}
	typ := args[0]
	if !knownTargetTypes[typ] {
		return fmt.Errorf("unknown target type %q (supported: bluesky, mastodon)", typ)
	}

	proj, err := currentProject()
	if err != nil {
		return err
	}
	cfg, err := loadOrInitConfig(proj)
	if err != nil {
		return err
	}

	// The secret must be entered by the user directly, never through agent
	// context (A5). Refuse non-interactive stdin rather than risk capture.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("run 'crofty targets add' in an interactive terminal so your token is typed directly and never captured")
	}

	ws, err := ensureWorkspace(proj, cfg)
	if err != nil {
		return err
	}

	switch typ {
	case "bluesky":
		fmt.Print("Bluesky handle (e.g. you.bsky.social): ")
		handle, err := readLine()
		if err != nil {
			return err
		}
		handle = strings.TrimSpace(handle)
		if handle == "" {
			return fmt.Errorf("handle is required")
		}
		appPassword, err := readSecret("Bluesky app password (input hidden): ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(appPassword) == "" {
			return fmt.Errorf("app password is required")
		}

		creds := publish.BlueskyCreds{Server: "https://bsky.social", Handle: handle, AppPassword: appPassword}
		fmt.Println("Checking credentials…")
		if err := publish.VerifyBluesky(creds); err != nil {
			return fmt.Errorf("could not sign in to Bluesky (nothing was saved): %w", err)
		}

		if err := secret.New(ws).Set("bluesky", "app_password", appPassword); err != nil {
			return fmt.Errorf("storing app password in keychain: %w", err)
		}
		if cfg.Targets == nil {
			cfg.Targets = map[string]project.TargetConfig{}
		}
		cfg.Targets["bluesky"] = project.TargetConfig{Type: "bluesky", Handle: handle, Server: creds.Server}
		if err := proj.SaveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("\n✓ added bluesky (%s). The app password is stored in your keychain, not in the project.\n", handle)
		fmt.Println("next: crofty targets test bluesky")

	case "mastodon":
		fmt.Print("Mastodon instance URL (e.g. https://mastodon.social): ")
		instance, err := readLine()
		if err != nil {
			return err
		}
		instance = strings.TrimSpace(instance)
		if instance == "" {
			return fmt.Errorf("instance URL is required")
		}
		token, err := readSecret("Mastodon access token (input hidden): ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("access token is required")
		}

		creds := publish.MastodonCreds{Server: instance, AccessToken: token}
		fmt.Println("Checking credentials…")
		if err := publish.VerifyMastodon(creds); err != nil {
			return fmt.Errorf("could not verify the Mastodon account (nothing was saved): %w", err)
		}

		if err := secret.New(ws).Set("mastodon", "access_token", token); err != nil {
			return fmt.Errorf("storing access token in keychain: %w", err)
		}
		if cfg.Targets == nil {
			cfg.Targets = map[string]project.TargetConfig{}
		}
		cfg.Targets["mastodon"] = project.TargetConfig{Type: "mastodon", Handle: instance, Server: instance}
		if err := proj.SaveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("\n✓ added mastodon (%s). The access token is stored in your keychain, not in the project.\n", instance)
		fmt.Println("next: crofty targets test mastodon")
	}
	return nil
}

func targetsList(args []string) error {
	proj, err := currentProject()
	if err != nil {
		return err
	}
	cfg, err := proj.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Targets) == 0 {
		fmt.Println("No targets configured. Add one with: crofty targets add bluesky")
		return nil
	}
	for _, name := range sortedKeys(cfg.Targets) {
		t := cfg.Targets[name]
		fmt.Printf("%-10s %s (%s)\n", name, t.Handle, t.Type)
	}
	return nil
}

func targetsTest(args []string) error {
	proj, err := currentProject()
	if err != nil {
		return err
	}
	cfg, err := proj.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Workspace == "" || len(cfg.Targets) == 0 {
		fmt.Println("No targets configured. Add one with: crofty targets add bluesky")
		return nil
	}

	names := args
	if len(names) == 0 {
		names = sortedKeys(cfg.Targets)
	}

	store := secret.New(cfg.Workspace)
	anyFail := false
	for _, name := range names {
		t, ok := cfg.Targets[name]
		if !ok {
			fmt.Printf("✗ %s — not configured\n", name)
			anyFail = true
			continue
		}
		switch t.Type {
		case "bluesky":
			pw, err := store.Get("bluesky", "app_password")
			if err != nil {
				fmt.Printf("✗ %s — no stored credential (%v)\n", name, err)
				anyFail = true
				continue
			}
			if err := publish.VerifyBluesky(publish.BlueskyCreds{Server: t.Server, Handle: t.Handle, AppPassword: pw}); err != nil {
				fmt.Printf("✗ %s — sign-in failed: %v\n", name, err)
				anyFail = true
				continue
			}
			fmt.Printf("✓ %s — credentials OK (%s)\n", name, t.Handle)
		case "mastodon":
			tok, err := store.Get("mastodon", "access_token")
			if err != nil {
				fmt.Printf("✗ %s — no stored credential (%v)\n", name, err)
				anyFail = true
				continue
			}
			if err := publish.VerifyMastodon(publish.MastodonCreds{Server: t.Server, Handle: t.Handle, AccessToken: tok}); err != nil {
				fmt.Printf("✗ %s — verification failed: %v\n", name, err)
				anyFail = true
				continue
			}
			fmt.Printf("✓ %s — credentials OK (%s)\n", name, t.Handle)
		default:
			fmt.Printf("✗ %s — unknown type %q\n", name, t.Type)
			anyFail = true
		}
	}
	if anyFail {
		return errSilent
	}
	return nil
}

// --- helpers -------------------------------------------------------------

func currentProject() (*project.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.Find(cwd)
}

// loadOrInitConfig loads the config, returning an empty one if the file does not
// exist yet.
func loadOrInitConfig(p *project.Project) (*project.Config, error) {
	cfg, err := p.LoadConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return &project.Config{}, nil
		}
		return nil, err
	}
	return cfg, nil
}

// ensureWorkspace assigns and persists a workspace id if absent.
func ensureWorkspace(p *project.Project, cfg *project.Config) (string, error) {
	if cfg.Workspace != "" {
		return cfg.Workspace, nil
	}
	w, err := id.NewULID()
	if err != nil {
		return "", err
	}
	cfg.Workspace = w
	if err := p.SaveConfig(cfg); err != nil {
		return "", err
	}
	return w, nil
}

func sortedKeys(m map[string]project.TargetConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func readLine() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

func readSecret(prompt string) (string, error) {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}
