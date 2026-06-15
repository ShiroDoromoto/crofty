package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// runFeatures prints crofty's capability catalogue: what the tool can do and how
// to turn each thing on. It exists because the rest of the CLI lists *commands*,
// not *capabilities* — so multilingual sites, analytics, raw HTML, diagrams, code
// colour, presets and tokens were only discoverable by reading demo/ and
// internal/ (the #1 piece of real-use feedback). One command, the whole map.
//
// The catalogue is honest about the present: each entry says how to enable the
// thing *today*, and whether it works in a fresh project or needs config. It is
// static (no project required) so an agent can call it before `init`.
func runFeatures(args []string) error {
	fs := flag.NewFlagSet("features", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit the catalogue as JSON (for tools/agents)")
	fs.Usage = func() {
		fmt.Println("crofty features — what crofty can do, and how to turn each thing on")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty features          # the capability catalogue")
		fmt.Println("  crofty features --json   # the same, machine-readable")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Features []feature `json:"features"`
		}{Features: featureCatalog()})
	}

	printFeatures()
	return nil
}

// feature is one crofty capability. Status says whether it works out of the box;
// Enable is the exact thing to do to get it (a command or a config key).
type feature struct {
	Name   string `json:"name"`
	What   string `json:"what"`
	Status string `json:"status"` // "built-in" | "config" | "command"
	Enable string `json:"enable"`
}

// featureCatalog is the single source of truth for both the text and --json
// output. Keep "enable" copy-pasteable and true of the *current* build — if a
// capability needs config the bundled theme doesn't ship (mermaid/abc render
// hooks), say so plainly rather than implying a one-liner that doesn't exist yet.
func featureCatalog() []feature {
	return []feature{
		// Works in a fresh project, no setup.
		{"writing", "Markdown in content/, one folder per post (page bundles)", "built-in", "write content/<section>/<slug>/index.md"},
		{"tags", "tag pages and per-post tag footer", "built-in", "add `tags: [a, b]` to a post's front matter"},
		{"rss", "an Atom/RSS feed and a 'Follow by RSS' link", "built-in", "automatic — nothing to set"},
		{"pagination", "page-by-page navigation on list pages", "built-in", "automatic; tune with paginate in hugo.yaml"},
		{"share", "reader share buttons, and `crofty share` for a ready-to-post fragment", "built-in", "automatic on posts; `crofty share <path>` for authors"},
		{"profile", "name / tagline / avatar / social links block", "built-in", "add data/profile.yml (name, tagline, avatar, social)"},
		{"support", "patronage links (Stripe, GitHub Sponsors, Ko-fi, Patreon) in the footer", "built-in", "add `support:` to data/profile.yml"},

		// Restyle — owned, contract-safe.
		{"looks", "ready-made colour/type presets (quiet-paper, terminal, …)", "command", "crofty theme set <name>   (list: crofty theme set)"},
		{"tokens", "edit colour, type and reading-width tokens", "command", "crofty theme eject   → assets/css/custom.css"},
		{"layouts", "override individual templates, or own the whole theme", "command", "edit layouts/<…>.html, or crofty theme eject --full"},
		{"head-extra", "inject extra <head> markup (fonts, meta, CSS)", "config", "params.crofty.head_raw in hugo.yaml"},

		// Opt-in via config — off by default on purpose.
		{"analytics", "Cloudflare / GA4 / GTM / AdSense (opt-in, no trackers by default)", "config", "params.crofty.analytics.{cloudflare,google_tag,gtm} or .adsense.client"},
		{"raw-html", "pass raw HTML in Markdown through (figure, video, …)", "config", "markup.goldmark.renderer.unsafe: true in hugo.yaml"},
		{"highlight", "theme-following code colour (class-based, light/dark)", "config", "markup.highlight.noClasses: false + a chroma stylesheet via head_raw"},
		{"multilingual", "two or more languages (/ and /<code>/, switch + redirect)", "config", "add a languages: block to hugo.yaml (see demo/hugo.yaml)"},

		// Needs a render hook the bundled theme doesn't ship yet.
		{"mermaid", "turn ```mermaid code blocks into diagrams", "config", "a render-codeblock-mermaid.html hook + unsafe (see demo/)"},
		{"abc", "turn ```abc code blocks into music notation", "config", "a render-codeblock-abc.html hook + unsafe (see demo/)"},
	}
}

func printFeatures() {
	fmt.Println("crofty features — what you can do, and how to turn each thing on.")
	fmt.Println()
	groups := []struct {
		title  string
		status string
	}{
		{"Out of the box (works in a fresh project):", "built-in"},
		{"Restyle (owned, contract-safe):", "command"},
		{"Opt-in (off by default — turn on with one config key or a render hook):", "config"},
	}
	for _, g := range groups {
		fmt.Println(g.title)
		for _, f := range featureCatalog() {
			if f.Status != g.status {
				continue
			}
			fmt.Printf("    %-13s %s\n", f.Name, f.What)
			fmt.Printf("    %-13s → %s\n", "", f.Enable)
		}
		fmt.Println()
	}
	fmt.Println("Machine-readable: crofty features --json")
}
