package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/shirodoromoto/crofty/internal/project"
)

// Re-running `crofty init` on an existing project lands here: an idempotent
// "configure" pass for the optional, discoverable-by-no-one settings —
// patronage and analytics (08 §4.3 C). It only ever writes data/profile.yml (a
// file crofty owns); analytics and the title are *shown* with where to set them,
// never auto-edited, so a hand-tuned hugo.yaml is never clobbered. Content,
// workspace id, and deploy config are never touched.

// profilePath is where patronage data lives — data/profile.yml, read by the
// theme's crofty/patronage.html partial.
func profilePath(root string) string {
	return filepath.Join(root, "data", "profile.yml")
}

// runConfigure walks the optional settings for an existing project. In a
// terminal it prompts for a support link; otherwise (an agent) it just prints
// the current state and where to set things, with no prompts.
func runConfigure(proj *project.Project) error {
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

	if term.IsTerminal(int(os.Stdin.Fd())) {
		if link, ok := promptSupportLink(); ok {
			if !isHTTPURL(link) {
				fmt.Println("  (that doesn't look like a URL — skipped)")
			} else if err := setProfileSupport(proj.Root, "stripe", link); err != nil {
				return err
			} else {
				fmt.Println("  ✓ saved to data/profile.yml — it shows in your site footer after the next build.")
			}
			fmt.Println()
		}
	}

	printAnalyticsGuidance()
	fmt.Println()
	printDirectEditTip(proj.Root)
	return nil
}

// promptSupportLink asks for a patronage link (a non-secret URL, so a plain
// prompt — unlike a token). Empty input skips.
func promptSupportLink() (string, bool) {
	fmt.Println("Add a support link so readers can chip in? (optional)")
	fmt.Println("  Paste a Stripe Payment Link — recommended: low fees, and the supporter")
	fmt.Println("  relationship stays yours. Create one: https://dashboard.stripe.com/payment-links")
	fmt.Println("  (GitHub Sponsors / Ko-fi / Patreon also work — add them in data/profile.yml.)")
	fmt.Print("Support link (or press Enter to skip): ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	link := strings.TrimSpace(line)
	return link, link != ""
}

func printAnalyticsGuidance() {
	fmt.Println("Analytics (optional — off by default, no trackers). To turn on, add under")
	fmt.Println("params.crofty.analytics in hugo.yaml (any subset):")
	fmt.Println(`    cloudflare: "<token>"    # Cloudflare Web Analytics`)
	fmt.Println(`    google_tag: "G-XXXXXXX"  # Google Analytics 4`)
	fmt.Println(`    gtm: "GTM-XXXXXX"         # Google Tag Manager`)
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
	fmt.Println("tip: 'crofty init' here adds an optional support link or analytics.")
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
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

func saveProfile(root string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(profilePath(root), b, 0o644)
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

// setProfileSupport sets one support entry, preserving any others already there.
func setProfileSupport(root, key, val string) error {
	m, err := loadProfile(root)
	if err != nil {
		return err
	}
	sup, ok := asStringMap(m["support"])
	if !ok || sup == nil {
		sup = map[string]any{}
	}
	sup[key] = val
	m["support"] = sup
	return saveProfile(root, m)
}
