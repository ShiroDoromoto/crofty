package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/shirodoromoto/crofty/internal/id"
	"github.com/shirodoromoto/crofty/internal/project"
)

// runInit scaffolds a new crofty project: a plain Hugo site (hugo.yaml +
// content) plus the .crofty/ marker with a fresh workspace id. It creates the
// container, not the writing — a sample post shows the shape and is yours to
// edit or delete. This is the one on-ramp for someone who has never opened a
// terminal: every message ends by telling them exactly what to type next.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	lang := fs.String("lang", "", "site language code (e.g. en, ja); default: detected from your OS")
	fs.Usage = func() {
		fmt.Println("crofty init — create a new project (a website you own)")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty init [name]      # a bare name lands in ~/Documents/Crofty/<name>")
		fmt.Println("  crofty init <path>      # an explicit path (or '.') is used as-is")
		fmt.Println("  crofty init --lang ja   # set the site language (default: from your OS)")
	}
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	// Establish the site language once, here — without a prompt. An explicit
	// --lang wins; otherwise infer it from the OS locale (so a Japanese Mac gets
	// a Japanese site by default). The agent can pass --lang when it knows
	// better from the conversation (07 O / language).
	siteLang := *lang
	if siteLang == "" {
		siteLang = detectLang()
	}

	// Resolve where the project goes. A bare name (the common case) lands in the
	// OS-standard, user-visible base — ignoring cwd, which the user likely can't
	// perceive. An explicit path (slash, '.', or absolute) is honored as-is, for
	// when an agent translates "put it on my Desktop" into a real path (07 O2).
	arg := "my-site"
	if len(rest) > 0 {
		arg = rest[0]
	}
	var abs string
	if looksLikePath(arg) {
		abs, err = filepath.Abs(arg)
		if err != nil {
			return err
		}
	} else {
		base, err := project.DefaultBase()
		if err != nil {
			return err
		}
		abs = filepath.Join(base, arg)
	}

	// Refuse to scaffold over an existing project rather than clobber a config.
	if fi, err := os.Stat(filepath.Join(abs, project.MarkerDir)); err == nil && fi.IsDir() {
		return fmt.Errorf("%s is already a crofty project.\n"+
			"  To build it:    cd %s && crofty build", abs, abs)
	}

	if err := os.MkdirAll(filepath.Join(abs, "content", "posts", "welcome"), 0o755); err != nil {
		return err
	}

	siteName := projectName(abs)
	now := time.Now().Add(-time.Hour) // safely in the past so Hugo never excludes it

	files := map[string]string{
		"hugo.yaml":                           hugoConfig(siteName, siteLang),
		"AGENTS.md":                           agentsGuide(),
		filepath.Join("content", "_index.md"): indexContent(siteName),
		filepath.Join("content", "posts", "welcome", "index.md"): welcomePost(now),
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(abs, rel), []byte(body), 0o644); err != nil {
			return err
		}
	}

	// Assign a workspace id and a sensible default deploy project name (the
	// folder name); the Cloudflare project is created on first deploy.
	ws, err := id.NewULID()
	if err != nil {
		return err
	}
	proj := &project.Project{Root: abs}
	cfg := &project.Config{
		Workspace: ws,
		Deploy:    project.DeployConfig{Provider: "cloudflare", Project: siteName},
	}
	if err := proj.SaveConfig(cfg); err != nil {
		return err
	}

	// Record the location globally so later sessions (and agents started
	// elsewhere) can find this project via a bare `crofty` (07 O3).
	if err := project.RegisterProject(abs); err != nil {
		return err
	}

	// crofty chose the location, not the user — announce the absolute path
	// loudly so neither the author nor their agent is left guessing where it is.
	fmt.Println()
	fmt.Println("✓ Your site is ready.")
	fmt.Println()
	fmt.Println("📁 ", abs)
	fmt.Println()
	fmt.Println("Everything for this site lives in that one folder — your writing,")
	fmt.Println("the settings, the built pages. Back up that folder and you have it all.")
	fmt.Println("    AGENTS.md          how an AI assistant should work with this project")
	fmt.Println("    content/posts/     your posts (a sample 'welcome' is here to edit or delete)")
	fmt.Println("    .crofty/           crofty's own settings (never your content, no secrets)")
	fmt.Println()
	fmt.Println("next — copy these one line at a time:")
	fmt.Printf("  cd %s\n", abs)
	fmt.Println("  crofty preview     # see your site in a browser (no account needed)")
	fmt.Println()
	fmt.Printf("If an AI assistant is helping you, have it read %s first.\n", filepath.Join(abs, "AGENTS.md"))
	return nil
}

// looksLikePath reports whether arg should be treated as a filesystem path
// (used as-is) rather than a bare project name (placed under DefaultBase).
func looksLikePath(arg string) bool {
	return arg == "." || arg == ".." ||
		filepath.IsAbs(arg) ||
		strings.ContainsRune(arg, '/') ||
		strings.ContainsRune(arg, os.PathSeparator)
}

// agentsGuide is the neutral, agent-agnostic playbook written to AGENTS.md at
// the project root. Any assistant opened in (or pointed at) this folder reads it
// and learns how to drive crofty — no specific AI is assumed (07 O4).
func agentsGuide() string {
	return "# crofty project\n\n" +
		"This folder is a website its author owns, built from Markdown with `crofty`\n" +
		"(a CLI that wraps Hugo and deploys to the author's own hosting and social\n" +
		"accounts). You are working in it on the author's behalf.\n\n" +
		"## Commands (run from this folder)\n\n" +
		"Each command prints the current state and the next step — read its output\n" +
		"before the next move.\n\n" +
		"- `crofty validate`        check posts against the spec\n" +
		"- `crofty preview`         serve locally at http://localhost:1313 (no account)\n" +
		"- `crofty build`           render the site into ./dist\n" +
		"- `crofty deploy`          publish ./dist to the author's site\n" +
		"- `crofty publish <post>`  syndicate a post's fragment to the author's accounts\n" +
		"- `crofty share <post>`    print a ready-to-post fragment for any network\n\n" +
		"To find this or other crofty projects from another directory, run `crofty`.\n\n" +
		"## Posts\n\n" +
		"Posts live in `content/posts/<slug>/index.md`. Front matter: `title` and\n" +
		"`date` are required; `description` is recommended. Dates in the future are\n" +
		"silently excluded from the build, so keep them at now or earlier.\n\n" +
		"## House rules\n\n" +
		"- The author writes the content. Don't invent posts or rewrite their voice.\n" +
		"- Never edit `crofty.id` in front matter — the tool manages it.\n" +
		"- Deploy before sharing links, so the canonical URL is live.\n" +
		"- Reply to the author in the site's language (`locale` in hugo.yaml), and\n" +
		"  switch to match if they write to you in a different one. Don't make them\n" +
		"  choose a language — an author who can't read English shouldn't be asked\n" +
		"  in English which language they prefer.\n"
}

// detectLang picks a default site language without a prompt. This is where the
// author's language gets established; --lang overrides it. Order matters: in the
// agent-orchestrated model crofty is usually run by an assistant whose shell has
// a neutral locale (C.UTF-8), so the user's actual OS UI language is the most
// reliable signal — fall back to the shell locale, then English.
func detectLang() string {
	if l := macPreferredLang(); l != "" {
		return l
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(key)
		if v == "" {
			continue
		}
		v = strings.SplitN(v, ".", 2)[0] // drop ".UTF-8" (e.g. "C.UTF-8" → "C")
		v = strings.SplitN(v, "@", 2)[0] // drop "@modifier"
		lang := strings.ToLower(strings.TrimSpace(strings.SplitN(v, "_", 2)[0]))
		// Skip the locale-less "C"/"POSIX" placeholders only after stripping the
		// encoding, so "C.UTF-8" doesn't get mistaken for a language.
		if lang == "" || lang == "c" || lang == "posix" {
			continue
		}
		return lang
	}
	return "en"
}

// macPreferredLang reads the user's preferred UI language from macOS
// (AppleLanguages), which reflects the person's actual language even when an
// agent runs crofty with a neutral shell locale. Returns "" off macOS or on any
// error.
func macPreferredLang() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	// Output is a plist array, e.g. (\n    "ja-JP",\n    "en-US"\n). Take the
	// first entry's language subtag: "ja-JP" → "ja", "zh-Hans" → "zh".
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.Trim(strings.TrimSpace(line), "(),\"")
		if line == "" {
			continue
		}
		lang := strings.ToLower(strings.SplitN(strings.SplitN(line, "-", 2)[0], "_", 2)[0])
		if lang != "" {
			return lang
		}
	}
	return ""
}

// projectName derives a Hugo/Cloudflare-safe name from the target directory.
func projectName(abs string) string {
	base := filepath.Base(abs)
	base = strings.ToLower(base)
	base = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return "my-site"
	}
	return base
}

// These templates use double-quoted strings so the Markdown can contain literal
// backticks (inline code) — a Go raw string literal cannot.
func hugoConfig(name, lang string) string {
	return fmt.Sprintf("# Standard Hugo config. crofty reads only its own settings from .crofty/config.json,\n"+
		"# so this file stays plain Hugo — your project is always yours to keep (eject-safe).\n"+
		"baseURL: \"https://example.com/\"\n"+
		"locale: %q\n"+
		"title: %q\n"+
		"enableRobotsTXT: true\n"+
		"params:\n"+
		"  description: \"A website I own, built from Markdown.\"\n"+
		"  # Tool-specific front matter and params nest under `crofty:` (spec v0).\n"+
		"  crofty:\n"+
		"    specVersion: \"0\"\n", lang, name)
}

func indexContent(name string) string {
	return fmt.Sprintf("---\n"+
		"title: %q\n"+
		"description: \"A website I own, built from Markdown.\"\n"+
		"---\n\n"+
		"Welcome. This is the homepage of a site you own.\n\n"+
		"Write posts as Markdown files under `content/posts/`, then run\n"+
		"`crofty build` to render the site and `crofty preview` to see it.\n", name)
}

func welcomePost(now time.Time) string {
	return fmt.Sprintf("---\n"+
		"title: \"Welcome to your new site\"\n"+
		"date: %s\n"+
		"description: \"A sample post. Edit this file, or delete the welcome folder and write your own.\"\n"+
		"crofty:\n"+
		"    tier: full\n"+
		"---\n\n"+
		"This is a sample post so you can see the shape of things.\n\n"+
		"It lives at `content/posts/welcome/index.md`. Change the title and the\n"+
		"words above, save the file, and run `crofty preview` to watch it update.\n\n"+
		"When you're happy, `crofty build` renders the whole site into `dist/`,\n"+
		"and `crofty deploy` puts it online (you'll connect a free account first).\n",
		now.Format(time.RFC3339))
}
