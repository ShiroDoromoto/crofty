package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/shirodoromoto/crofty/internal/theme"
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
	}
	if deploy != nil {
		state.Provider = deploy.Deploy.Provider
		if state.Provider == "" {
			state.Provider = "cloudflare"
		}
		state.Project = deploy.Deploy.Project
		state.Host = deploy.Deploy.Host
		state.RemotePath = deploy.Deploy.Path
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
	fmt.Println()
	fmt.Println("Turn things on with 'crofty add <feature>' / 'crofty lang add <code>'.")
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
