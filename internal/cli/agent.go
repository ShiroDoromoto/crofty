package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// runAgent prints crofty's entire command surface in one shot, for an AI that is
// operating crofty on the author's behalf to read first. It is the single
// "give this command to your assistant" entry point the project page points at:
// a fresh assistant runs `crofty agent` (or `--json`) and learns every command,
// its flags, the usual workflow and where to read live state — without having to
// open `-h` on fifteen subcommands or read the source.
//
// Like `features`, it needs no project, so an agent can read it before `init`.
//
// Drift is the real risk here (a feature lands but `agent` doesn't reflect it),
// so the brief is built to make omissions hard:
//   - command names and summaries are pulled straight from commands(), the same
//     source `crofty help` uses, so a new command can never be invisible here;
//   - agentDetails() adds per-command flags/examples by hand, and agent_test.go
//     fails if any command lacks an entry — so adding a command forces a visit;
//   - capabilities are NOT duplicated. The brief points at `crofty features`,
//     the single source for those, so turning a feature on needs no edit here.
func runAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit the brief as JSON (for tools that parse it)")
	fs.Usage = func() {
		fmt.Println("crofty agent — the whole command surface, for an AI to read first")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty agent          # a briefing an assistant reads to drive crofty")
		fmt.Println("  crofty agent --json   # the same, machine-readable")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	b := agentBrief()
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(b)
	}
	printAgentBrief(b)
	return nil
}

// agentFlag is one flag on a command: the spelling to type and what it does.
type agentFlag struct {
	Name string `json:"name"`
	Help string `json:"help"`
}

// agentCmd is one command (or subcommand) as the brief presents it. Name and
// Summary for top-level commands are filled from commands(); Flags, Examples and
// Sub come from agentDetails().
type agentCmd struct {
	Name     string      `json:"name"`
	Summary  string      `json:"summary"`
	Flags    []agentFlag `json:"flags,omitempty"`
	Examples []string    `json:"examples,omitempty"`
	Sub      []agentCmd  `json:"subcommands,omitempty"`
}

// brief is the whole one-shot manifest, the single source for both the text and
// --json output (so the two can never disagree).
type brief struct {
	Crofty   string     `json:"crofty"`   // one line: what crofty is
	Version  string     `json:"version"`  // the running binary's version
	Workflow []string   `json:"workflow"` // the usual order of operations
	Commands []agentCmd `json:"commands"` // every command, from commands()
	Inspect  []string   `json:"inspect"`  // machine-readable state surfaces to read
	Notes    []string   `json:"notes"`    // the handful of rules an agent must know
}

// agentBrief assembles the manifest. Command names and summaries come from
// commands() (so they can't drift from `crofty help`); the per-command detail is
// merged in from agentDetails().
func agentBrief() brief {
	details := agentDetails()
	cmds := make([]agentCmd, 0, len(commands()))
	for _, c := range commands() {
		d := details[c.name] // zero value when missing; agent_test.go forbids that
		d.Name = c.name
		d.Summary = c.summary
		cmds = append(cmds, d)
	}

	return brief{
		Crofty:  "write Markdown; build and deploy a static site you own (a Hugo site with a frozen theme, published to Cloudflare Pages).",
		Version: Version,
		Workflow: []string{
			"crofty init — create the project (a folder the author fully owns)",
			"write Markdown at content/posts/<slug>/index.md",
			"crofty preview — see it locally in a browser (no account)",
			"crofty build — render the site to ./dist",
			"crofty deploy — publish ./dist to Cloudflare Pages",
		},
		Commands: cmds,
		Inspect: []string{
			"crofty config --json     — this project now: title, languages, features on, theme, deploy target",
			"crofty features --json   — every capability and the exact one-liner to turn it on",
			"crofty validate --json   — check content against the spec (the gate before build)",
			"crofty doctor --json     — check the built ./dist against the output contract (the gate before deploy)",
		},
		Notes: []string{
			"The author installs crofty and runs `crofty init`; from there you (the AI) drive it. The interface is neutral state + next-step output, not a GUI.",
			"A Cloudflare API token must be typed in a terminal by the human — crofty reads it from a hidden TTY prompt, never stdin, so it never passes through you. To publish, tell the author to run `crofty deploy` and paste the token when asked.",
			"crofty owns the files it writes (content stubs, render hooks, assets/css/custom.css) but never rewrites hugo.yaml — for config changes it prints the exact lines for the author to paste.",
			"`draft: true` or a future `date` keeps a post off the built site; `crofty build` lists what it left out. Run `crofty validate` before build and `crofty doctor` before deploy.",
		},
	}
}

// agentDetails carries the hand-written per-command flags, examples and
// subcommands, keyed by command name. Leave Name/Summary empty — agentBrief()
// fills those from commands(). Every command in commands() MUST have an entry
// here (use agentCmd{} for one with no flags or examples); agent_test.go fails
// otherwise, which is the guard against an `agent`-reflection omission when a
// command is added or changed.
func agentDetails() map[string]agentCmd {
	return map[string]agentCmd{
		"init": {
			Flags: []agentFlag{
				{"--lang <code>", "site language (e.g. en, ja); default: detected from the OS"},
				{"--title \"<text>\"", "display title (free text, a Japanese name is fine); default: the folder name"},
				{"--project <name>", "deploy name → <name>.pages.dev; default: the folder name"},
			},
			Examples: []string{
				"crofty init                       # a new project under ~/Documents/Crofty/",
				"crofty init my-blog               # a bare name lands in the standard base",
				"crofty init .                     # turn the current folder into a project",
				"crofty init --lang ja --title \"…\" --project blog",
			},
		},
		"features": {
			Flags:    []agentFlag{{"--json", "the capability catalogue as JSON"}},
			Examples: []string{"crofty features", "crofty features --json"},
		},
		"agent": {
			Flags:    []agentFlag{{"--json", "this brief as JSON"}},
			Examples: []string{"crofty agent", "crofty agent --json"},
		},
		"config": {
			Flags:    []agentFlag{{"--json", "the current configuration as JSON"}},
			Examples: []string{"crofty config", "crofty config --json"},
		},
		"add": {
			Flags: []agentFlag{{"--force", "overwrite an existing render hook"}},
			Sub: []agentCmd{
				{Name: "mermaid", Summary: "render ```mermaid blocks as diagrams (writes a project render hook; client JS)"},
				{Name: "abc", Summary: "render ```abc blocks as sheet music (client JS)"},
				{Name: "highlight", Summary: "show the hugo.yaml for theme-following code colour (older projects)"},
				{Name: "raw-html", Summary: "show the hugo.yaml to let raw HTML in Markdown through"},
				{Name: "analytics", Summary: "show how to turn on Cloudflare / GA4 / GTM / AdSense (opt-in, off by default)"},
			},
			Examples: []string{"crofty add mermaid", "crofty add analytics"},
		},
		"lang": {
			Sub: []agentCmd{
				{Name: "add <code>", Summary: "write a translated homepage stub + print the hugo.yaml to paste (e.g. ja)"},
				{Name: "list", Summary: "the languages configured now"},
			},
			Examples: []string{"crofty lang add ja", "crofty lang list"},
		},
		"preview": {
			Examples: []string{"crofty preview   # serves locally; blocks until Control-C"},
		},
		"build": {
			Examples: []string{"crofty build   # renders to ./dist; warns about drafts / future-dated posts left out"},
		},
		"connect": {
			Flags:    []agentFlag{{"--account <id>", "Cloudflare account id (when a token reaches several)"}},
			Examples: []string{"crofty connect   # save the deploy token without deploying"},
		},
		"deploy": {
			Flags: []agentFlag{
				{"--account <id>", "Cloudflare account id to deploy to (when a token reaches several)"},
				{"--reauth", "enter a new Cloudflare API token (replace the saved one)"},
			},
			Examples: []string{"crofty deploy", "crofty deploy --reauth"},
		},
		"reset": {
			Flags: []agentFlag{
				{"--all", "every project's saved credentials + global state (for uninstall)"},
				{"--yes", "skip the confirmation prompt"},
			},
			Examples: []string{"crofty reset", "crofty reset --all --yes"},
		},
		"validate": {
			Flags: []agentFlag{
				{"--json", "structured JSON (for tools)"},
				{"--no-hints", "skip the capability hints (\"this won't render unless…\")"},
			},
			Examples: []string{"crofty validate", "crofty validate content/posts/hello/index.md"},
		},
		"doctor": {
			Flags:    []agentFlag{{"--json", "structured JSON (for tools)"}},
			Examples: []string{"crofty doctor   # checks ./dist — run 'crofty build' first"},
		},
		"share": {
			Flags: []agentFlag{
				{"--to <list>", "comma-separated channels (default: the post's crofty.targets, else all known)"},
				{"--json", "machine-readable JSON (for your agent)"},
				{"--plain", "only the plain text + link (handy for | pbcopy)"},
				{"--skip-deploy-check", "print even if the post isn't live on the site yet"},
			},
			Examples: []string{
				"crofty share content/posts/hello/index.md",
				"crofty share content/posts/hello/index.md --to x,bluesky --json",
			},
		},
		"theme": {
			Sub: []agentCmd{
				{Name: "tokens", Summary: "list the colour / type / reading-width tokens you can change"},
				{
					Name:    "set [<name>]",
					Summary: "apply a ready-made look (a token override); with no name, lists the looks crofty ships",
					Flags:   []agentFlag{{"--force", "overwrite custom.css even if it's been hand-edited"}},
				},
				{
					Name:    "eject",
					Summary: "write the design tokens to assets/css/custom.css to edit",
					Flags: []agentFlag{
						{"--full", "write the whole theme (layouts + CSS) into the project"},
						{"--force", "overwrite files that already exist"},
						{"--print", "print the tokens to stdout; touch no file"},
					},
				},
			},
			Examples: []string{"crofty theme set quiet-paper", "crofty theme eject", "crofty theme eject --full"},
		},
		"eject": {
			Examples: []string{"crofty eject   # not implemented yet — own the theme today with 'crofty theme eject --full'"},
		},
	}
}

func printAgentBrief(b brief) {
	fmt.Println("crofty —", b.Crofty)
	fmt.Println("version:", b.Version)
	fmt.Println()
	fmt.Println("For an AI operating crofty on the author's behalf. This is the whole command")
	fmt.Println("surface; read it once and you can drive crofty without opening -h on each one.")
	fmt.Println()

	fmt.Println("Typical workflow:")
	for _, w := range b.Workflow {
		fmt.Println("  → " + w)
	}
	fmt.Println()

	fmt.Println("Commands:")
	for _, c := range b.Commands {
		printAgentCmd(c, "  ")
	}
	fmt.Println()

	fmt.Println("Read live state (machine-readable — run these against the project):")
	for _, s := range b.Inspect {
		fmt.Println("  " + s)
	}
	fmt.Println()

	fmt.Println("Good to know:")
	for _, n := range b.Notes {
		fmt.Println("  - " + n)
	}
}

func printAgentCmd(c agentCmd, indent string) {
	fmt.Printf("%s%-12s %s\n", indent, c.Name, c.Summary)
	for _, f := range c.Flags {
		fmt.Printf("%s    %-20s %s\n", indent, f.Name, f.Help)
	}
	for _, ex := range c.Examples {
		fmt.Printf("%s    $ %s\n", indent, ex)
	}
	for _, s := range c.Sub {
		printAgentCmd(s, indent+"    ")
	}
}
