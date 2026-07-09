package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// Re-running `crofty init` on an existing project lands here: an idempotent
// "configure" pass for the optional, discoverable-by-no-one settings —
// patronage and analytics (08 §4.3 C). It writes nothing: both are *shown* with
// where to set them — analytics in hugo.yaml, a support link in data/profile.yml
// — so the author (or their AI) edits the files and a hand-tuned config is never
// clobbered. Content, workspace id, and deploy config are never touched.

// profilePath is where patronage data lives — data/profile.yml, read by the
// theme's crofty/patronage.html partial.
func profilePath(root string) string {
	return filepath.Join(root, "data", "profile.yml")
}

// runConfigure shows the optional settings for an existing project. It never
// prompts and never writes — it prints the current state and exactly where to
// set things, leaving the edit to the author or their AI (same as init).
func runConfigure(proj *project.Project) error {
	// init only lands here when the target is already a project. Say that out
	// loud: an `init` that quietly does something else is how "it succeeded but
	// there's no site" happens (D-2).
	fmt.Println("This is already a crofty project — configuring it instead of creating a new site.")
	fmt.Println()
	fmt.Println("Configure (optional settings — your content is untouched):")
	fmt.Println()
	fmt.Println("📁 ", proj.Root)
	fmt.Println()

	if sup := currentSupport(proj.Root); len(sup) > 0 {
		fmt.Println("Support links already set (data/profile.yml):")
		for k, v := range sup {
			fmt.Printf("    %s: %v\n", k, v)
		}
		fmt.Println()
	}

	// Lead with the more familiar blog-setup item (analytics) before the support
	// link, so the optional section eases in rather than opening on a money
	// question (least psychological friction first).
	printAnalyticsGuidance()
	fmt.Println()
	printSupportGuidance()
	fmt.Println()

	printDirectEditTip(proj.Root)
	return nil
}

func printAnalyticsGuidance() {
	fmt.Println("Analytics (optional — off by default, no trackers). To turn on, add under")
	fmt.Println("params.crofty.analytics in hugo.yaml (any subset):")
	fmt.Println(`    cloudflare: "<token>"    # Cloudflare Web Analytics`)
	fmt.Println(`    google_tag: "G-XXXXXXX"  # Google Analytics 4`)
	fmt.Println(`    gtm: "GTM-XXXXXX"         # Google Tag Manager`)
}

// printSupportGuidance mirrors printAnalyticsGuidance: a support link is shown,
// never prompted. It's a plain (non-secret) URL the author or their AI drops
// into data/profile.yml, where the theme's crofty/patronage.html partial renders
// it in the footer. Stripe is suggested first (low fees, the supporter
// relationship stays with the author and never touches crofty).
func printSupportGuidance() {
	fmt.Println("Support link (optional — let readers chip in). Add under `support` in")
	fmt.Println("data/profile.yml (any subset); it shows in your site footer:")
	fmt.Println(`    stripe: "https://buy.stripe.com/…"   # a Stripe Payment Link (suggested)`)
	fmt.Println(`    github_sponsors: "your-username"     # also: kofi, patreon, buymeacoffee`)
}

func printDirectEditTip(root string) {
	fmt.Println("These are plain config — you (or your AI) can also edit them directly:")
	fmt.Printf("    %s\n", profilePath(root))
	fmt.Printf("    %s\n", filepath.Join(root, "hugo.yaml"))
}

// optionalSetupHint nudges, once a build, toward the optional settings — but
// only while none are set up (no data/profile.yml), so it disappears for anyone
// who has a profile and never nags those who don't want one for long.
func optionalSetupHint(root string) {
	if _, err := os.Stat(profilePath(root)); err == nil {
		return // already has a profile — no nudge
	}
	fmt.Println()
	fmt.Println("tip: 'crofty init' here shows optional analytics or a support link.")
}

// --- data/profile.yml (crofty-owned) -------------------------------------

func loadProfile(root string) (map[string]any, error) {
	b, err := os.ReadFile(profilePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// currentSupport returns the existing support map (or nil).
func currentSupport(root string) map[string]any {
	m, err := loadProfile(root)
	if err != nil {
		return nil
	}
	sup, _ := asStringMap(m["support"]) // asStringMap lives in publish.go
	return sup
}
