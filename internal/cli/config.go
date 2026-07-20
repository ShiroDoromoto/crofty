package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ShiroDoromoto/crofty/internal/access"
	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/theme"
)

// runConfig reports the project's current configuration: language(s), which
// opt-in features are on, the theme state, the deploy target. Real-use feedback
// A-2: there was no way to ask "is this on right now?" — you had to build and
// grep the output. --json gives an agent the current state to diff against.
func runConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit the current configuration as JSON (for tools/agents)")
	fs.Usage = func() {
		fmt.Println("crofty config — show this project's current configuration")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty config          # languages, features turned on, theme, deploy target")
		fmt.Println("  crofty config --json   # the same, machine-readable")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	cfg, err := loadHugoConfig(proj.Root)
	if err != nil {
		return err
	}
	deploy, _ := proj.LoadConfig()

	state := siteConfig{
		Title:           siteTitle(cfg),
		DefaultLanguage: defaultLanguage(cfg),
		Languages:       sortedKeys(configuredLanguages(cfg)),
		Analytics:       analyticsProviders(cfg),
		Features:        featureState(proj.Root, cfg),
		Theme:           themeState(proj.Root),
		Support:         supportProviders(proj.Root),
		State:           stateDirState(),
		PagesFunctions:  projectFunctions(proj.Root),
	}
	if deploy != nil {
		state.Provider = deploy.Deploy.Provider
		if state.Provider == "" {
			state.Provider = "cloudflare"
		}
		state.Project = deploy.Deploy.Project
		state.Host = deploy.Deploy.Host
		state.RemotePath = deploy.Deploy.Path
		state.FooterCredit = deploy.FooterCredit
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(state)
	}
	printConfig(state)
	return nil
}

// siteConfig is the current configuration, the source of truth for both outputs.
type siteConfig struct {
	Title           string          `json:"title"`
	DefaultLanguage string          `json:"defaultLanguage"`
	Languages       []string        `json:"languages"`            // empty when single-language
	Provider        string          `json:"provider"`             // cloudflare | sftp | ftps
	Project         string          `json:"project,omitempty"`    // Cloudflare: becomes <project>.pages.dev
	Host            string          `json:"host,omitempty"`       // sftp/ftps server
	RemotePath      string          `json:"remotePath,omitempty"` // sftp/ftps web root
	Analytics       []string        `json:"analytics"`            // providers configured
	Features        map[string]bool `json:"features"`             // raw-html, highlight, mermaid, abc
	Theme           string          `json:"theme"`                // default | preset | tokens | full-eject
	Support         []string        `json:"support"`              // patronage providers configured
	FooterCredit    string          `json:"footerCredit"`         // on | off | "" (undecided)
	State           stateDir        `json:"state"`                // where crofty keeps its own state, and whether it may write there
	// PagesFunctions lists the Pages Functions entry points at the project root
	// (functions/, _worker.js). Non-empty means `crofty deploy` stops by
	// default — crofty publishes static files only, so deploying would take
	// whatever serves those routes offline. Reading it here is how an agent
	// learns that before it runs deploy and hits the gate.
	PagesFunctions []string `json:"pagesFunctions"`
}

// stateDir reports crofty's own state directory: where it is, who chose it, and
// whether crofty may write there. It rides on `config` rather than `doctor`
// because doctor grades a built ./dist — an agent that wants to know whether it
// is behind a permission wall should not have to run a build to find out. Asking
// used to mean running init and reading the warning afterwards (#13, #25).
//
// An unwritable state directory never fails the command that reports it: the
// registry only powers discovery. Wall carries the ways on, so an agent reads
// the same choices here that init would have shown it (D-1).
type stateDir struct {
	Dir       string          `json:"dir"`
	Env       string          `json:"env"`                 // the variable that relocates it
	FromEnv   bool            `json:"fromEnv"`             // Env chose Dir, rather than the OS config dir
	Writable  bool            `json:"writable"`            // crofty probed it, rather than reading the mode bits
	Reason    string          `json:"reason,omitempty"`    // why not, when unwritable
	Wall      *access.Payload `json:"wall,omitempty"`      // the choices, when the reason is a permission wall
	AgentRule string          `json:"agentRule,omitempty"` // stated where an agent meets the wall
}

// stateDirState probes the state directory. A directory crofty cannot even name
// (no OS config dir, no CROFTY_HOME) is reported as unwritable with the reason,
// because a config report that omits the answer is worse than an ugly one.
func stateDirState() stateDir {
	s := stateDir{Env: project.HomeEnv}
	status, err := project.State()
	if err != nil {
		s.Reason = err.Error()
		return s
	}
	s.Dir, s.FromEnv, s.Writable = status.Dir, status.FromEnv, status.Writable()
	if status.Err == nil {
		return s
	}
	s.Reason = access.Reason(status.Err)
	if d, ok := access.From(status.Err); ok {
		p := d.Payload()
		s.Wall, s.AgentRule = &p, access.AgentRule
	}
	return s
}

// siteTitle is the display title: the top-level title, or — on a multilingual
// site where the title lives per-language — the default language's title.
func siteTitle(cfg map[string]any) string {
	if s, ok := cfg["title"].(string); ok && s != "" {
		return s
	}
	if langs, ok := cfg["languages"].(map[string]any); ok {
		if l, ok := langs[defaultLanguage(cfg)].(map[string]any); ok {
			if s, ok := l["title"].(string); ok && s != "" {
				return s
			}
		}
	}
	return "My site"
}

// supportProviders lists the patronage providers configured in data/profile.yml,
// excluding the free-text `message` (which is copy, not a provider).
func supportProviders(root string) []string {
	sup := currentSupport(root)
	out := []string{}
	for _, k := range []string{"stripe", "github_sponsors", "kofi", "patreon", "buymeacoffee"} {
		if s, ok := sup[k].(string); ok && s != "" {
			out = append(out, k)
		}
	}
	return out
}

// featureState reports which opt-in capabilities are turned on right now.
func featureState(root string, cfg map[string]any) map[string]bool {
	return map[string]bool{
		"raw-html":  digBool(cfg, "markup", "goldmark", "renderer", "unsafe"),
		"highlight": highlightOn(cfg),
		"mermaid":   fileExists(filepath.Join(root, "layouts", "_default", "_markup", "render-codeblock-mermaid.html")),
		"abc":       fileExists(filepath.Join(root, "layouts", "_default", "_markup", "render-codeblock-abc.html")),
	}
}

// highlightOn is true when class-based code colour is on (noClasses explicitly
// false). Hugo defaults noClasses to true (inline styles), so absence = off.
func highlightOn(cfg map[string]any) bool {
	cur := any(cfg)
	for _, k := range []string{"markup", "highlight", "noClasses"} {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		cur = m[k]
	}
	b, ok := cur.(bool)
	return ok && !b
}

// analyticsProviders lists the analytics keys the owner has set under
// params.crofty.analytics (plus adsense).
func analyticsProviders(cfg map[string]any) []string {
	out := []string{}
	params, _ := cfg["params"].(map[string]any)
	crofty, _ := params["crofty"].(map[string]any)
	an, _ := crofty["analytics"].(map[string]any)
	for _, k := range []string{"cloudflare", "google_tag", "gtm"} {
		if s, ok := an[k].(string); ok && s != "" {
			out = append(out, k)
		}
	}
	if ad, ok := an["adsense"].(map[string]any); ok {
		if s, ok := ad["client"].(string); ok && s != "" {
			out = append(out, "adsense")
		}
	}
	sort.Strings(out)
	return out
}

// themeState describes how the theme has been customised, from least to most
// owned: default → a token override (preset or hand-edited) → a full eject.
func themeState(root string) string {
	if fileExists(filepath.Join(root, "layouts", "_default", "baseof.html")) {
		return "full-eject"
	}
	custom := filepath.Join(root, filepath.FromSlash(theme.CustomCSSPath))
	if b, err := os.ReadFile(custom); err == nil {
		if theme.IsShippedCSS(string(b)) {
			return "tokens"
		}
		return "custom"
	}
	return "default"
}

func printConfig(s siteConfig) {
	fmt.Println("crofty config — this project right now:")
	fmt.Println()
	fmt.Printf("  title       %s\n", s.Title)
	if len(s.Languages) == 0 {
		fmt.Printf("  language    %s (single)\n", s.DefaultLanguage)
	} else {
		fmt.Printf("  languages   %v (default: %s)\n", s.Languages, s.DefaultLanguage)
	}
	switch s.Provider {
	case "sftp", "ftps":
		fmt.Printf("  deploy      %s → %s:%s\n", s.Provider, s.Host, s.RemotePath)
	default:
		if s.Project != "" {
			fmt.Printf("  deploy      %s.pages.dev\n", s.Project)
		}
	}
	fmt.Printf("  theme       %s\n", s.Theme)

	fmt.Println()
	fmt.Println("  features on:")
	for _, k := range sortedKeys(toAnyMap(s.Features)) {
		mark := "off"
		if s.Features[k] {
			mark = "on"
		}
		fmt.Printf("    %-11s %s\n", k, mark)
	}

	fmt.Println()
	fmt.Printf("  analytics   %s\n", noneIfEmpty(s.Analytics))
	fmt.Printf("  support     %s\n", noneIfEmpty(s.Support))
	fmt.Printf("  credit      %s\n", creditLabel(s.FooterCredit))
	fmt.Printf("  state       %s\n", stateLabel(s.State))
	printStateWall(s.State)
	if len(s.PagesFunctions) > 0 {
		fmt.Println()
		fmt.Printf("  ⚠ this project has Pages Functions (%s). 'crofty deploy' stops rather than\n", strings.Join(s.PagesFunctions, ", "))
		fmt.Println("    take them offline — it publishes static files only. Deploy the way they are")
		fmt.Println("    deployed, or 'crofty deploy --static-only' to drop them on purpose.")
	}
	fmt.Println()
	fmt.Println("Turn things on with 'crofty add <feature>' / 'crofty lang add <code>'.")
}

// stateLabel says where crofty's state is and, in the same breath, whether it
// may write there — the two facts are never useful apart.
func stateLabel(s stateDir) string {
	where := s.Dir
	if where == "" {
		where = "(crofty cannot tell)"
	}
	if s.FromEnv {
		where += " (" + s.Env + ")"
	}
	if s.Writable {
		return where
	}
	return where + " — crofty may not write here"
}

// printStateWall shows the ways past a wall on the state directory, without
// dressing it up as a failure: everything but discovery works without it.
func printStateWall(s stateDir) {
	if s.Writable || s.Reason == "" {
		return
	}
	fmt.Println()
	fmt.Println("  crofty cannot record your projects, so it won't find them from other folders.")
	fmt.Println("  Everything else works — cd into a project and carry on.")
	if s.Wall == nil {
		fmt.Printf("\n  it was told:  %s\n", s.Reason)
		return
	}
	printWall(os.Stdout, *s.Wall)
	fmt.Printf("\nIf you are an AI running crofty for someone: %s\n", access.AgentRule)
}

// --- small helpers --------------------------------------------------------

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toAnyMap(m map[string]bool) map[string]any {
	out := make(map[string]any, len(m))
	for k := range m {
		out[k] = m[k]
	}
	return out
}

func noneIfEmpty(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}
	return fmt.Sprint(s)
}

// creditLabel describes the footer-credit choice for the human config view.
func creditLabel(v string) string {
	switch v {
	case "on":
		return "on (\"via crofty\" in the footer)"
	case "off":
		return "off"
	default:
		return "not decided (crofty asks once on first deploy)"
	}
}
