package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

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
	titleFlag := fs.String("title", "", "site display title (free text); default: the folder name, or asked")
	projectFlag := fs.String("project", "", "deploy/project name — becomes <name>.pages.dev; default: the folder name, or asked")
	fs.Usage = func() {
		fmt.Println("crofty init — create a new project (a website you own)")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty init                 # asks for a name (default my-site)")
		fmt.Println("  crofty init [name]          # a bare name lands in ~/Documents/Crofty/<name>")
		fmt.Println("  crofty init <path>          # an explicit path (or '.') is used as-is")
		fmt.Println("  crofty init --lang ja       # set the site language (default: from your OS)")
		fmt.Println("  crofty init --title \"…\"      # display title (free text, e.g. a Japanese name)")
		fmt.Println("  crofty init --project blog  # the published name → blog.pages.dev")
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

	// Re-running init on a project that already exists isn't an error — it's how
	// you reach the optional settings (support link, analytics) you'd otherwise
	// never discover (08 §4.3). With no name, "here" means the current directory;
	// an explicit name/path that already exists configures that one.
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		if proj, ferr := project.Find(cwd); ferr == nil {
			return runConfigure(proj)
		}
	} else if target, rerr := resolveInitTarget(rest[0]); rerr == nil && isExistingProject(target) {
		return runConfigure(&project.Project{Root: target})
	}

	// One reader for every prompt below, so buffered input isn't lost between
	// the location question and the name questions.
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	stdin := bufio.NewReader(os.Stdin)

	// Resolve where a NEW project goes. With an explicit name/path, or a
	// non-interactive (agent) run, resolve directly. In a terminal with no name,
	// ask for one (default my-site), re-asking if it already exists so a second
	// `crofty init` doesn't dead-end on the default. A bare name lands in the
	// OS-standard base (~/Documents/Crofty/<name>), ignoring cwd; a path (slash,
	// '.', absolute) is honored as-is, e.g. when an agent passes a real path.
	var abs string
	if len(rest) > 0 || !interactive {
		name := "my-site"
		if len(rest) > 0 {
			name = rest[0]
		}
		abs, err = resolveInitTarget(name)
		if err != nil {
			return err
		}
		if isExistingProject(abs) {
			return fmt.Errorf("%s is already a crofty project.\n"+
				"  To build it:     cd %s && crofty build\n"+
				"  Or make another: crofty init <name>", abs, abs)
		}
	} else {
		for {
			fmt.Print("Site name [my-site]: ")
			line, _ := stdin.ReadString('\n')
			name := strings.TrimSpace(line)
			if name == "" {
				name = "my-site"
			}
			abs, err = resolveInitTarget(name)
			if err != nil {
				return err
			}
			if !isExistingProject(abs) {
				break
			}
			fmt.Printf("  '%s' already exists — pick another name.\n", name)
		}
	}

	// Display title and project (deploy) name are two different things: the title
	// is free text shown on the site (a Japanese name is fine), while the project
	// name becomes the public address <name>.pages.dev and must be URL-safe. We
	// derive both from the folder, but ask in a terminal — and the project prompt
	// states the URL, the part people miss when 'init .' silently uses the folder.
	siteTitle, projectSlug := chooseNames(stdin, abs, *titleFlag, *projectFlag, interactive)

	if err := os.MkdirAll(filepath.Join(abs, "content", "posts", "welcome"), 0o755); err != nil {
		return err
	}

	now := time.Now().Add(-time.Hour) // safely in the past so Hugo never excludes it

	files := map[string]string{
		"hugo.yaml":                           hugoConfig(siteTitle, siteLang),
		filepath.Join("content", "_index.md"): indexContent(siteTitle),
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
		Deploy:    project.DeployConfig{Provider: "cloudflare", Project: projectSlug},
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
	fmt.Println("    content/posts/     your posts (a sample 'welcome' is here to edit or delete)")
	fmt.Println("    .crofty/           crofty's own settings (never your content, no secrets)")
	fmt.Println()
	fmt.Printf("When you deploy, it will be published at %s.pages.dev (you can add your\n", projectSlug)
	fmt.Println("own domain later). To change the title or that name, edit hugo.yaml's")
	fmt.Println("`title` and .crofty/config.json's deploy.project.")
	fmt.Println()

	// The core next step first — what to type now — so it's never buried under
	// optional settings.
	fmt.Println("next — copy these one line at a time:")
	fmt.Printf("  cd %s\n", abs)
	fmt.Println("  crofty preview     # see your site in a browser (no account needed)")
	fmt.Println()

	// Optional settings — guidance only, never a prompt. Both analytics and a
	// support link are added by the author (or their AI) editing the files, so
	// init stays fully non-interactive: the interface is neutral "state + next
	// steps" output, not an interactive question only a human could answer.
	// Analytics leads as the more familiar blog-setup idea; the support link
	// follows. Re-running 'crofty init' here shows these again, plus any links
	// already set.
	fmt.Println("Optional, anytime — you or your AI can add these by editing the files:")
	fmt.Println()
	printAnalyticsGuidance()
	fmt.Println()
	printSupportGuidance()
	return nil
}

// resolveInitTarget turns an init argument into an absolute project directory: a
// bare name lands under the OS-standard base; a path (slash, '.', absolute) is
// used as-is.
func resolveInitTarget(arg string) (string, error) {
	if looksLikePath(arg) {
		return filepath.Abs(arg)
	}
	base, err := project.DefaultBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, arg), nil
}

// isExistingProject reports whether abs already holds a crofty project.
func isExistingProject(abs string) bool {
	fi, err := os.Stat(filepath.Join(abs, project.MarkerDir))
	return err == nil && fi.IsDir()
}

// looksLikePath reports whether arg should be treated as a filesystem path
// (used as-is) rather than a bare project name (placed under DefaultBase).
func looksLikePath(arg string) bool {
	return arg == "." || arg == ".." ||
		filepath.IsAbs(arg) ||
		strings.ContainsRune(arg, '/') ||
		strings.ContainsRune(arg, os.PathSeparator)
}

// detectLang picks a default site language without a prompt. This is where the
// author's language gets established; --lang overrides it. Order matters: in the
// agent-orchestrated model crofty is usually run by an assistant whose shell has
// a neutral locale (C.UTF-8), so the user's actual OS UI language is the most
// reliable signal — fall back to the shell locale, then English.
func detectLang() string {
	if l := osPreferredLang(); l != "" {
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

// osPreferredLang reads the user's preferred UI language from the operating
// system, which reflects the person's actual language even when an agent runs
// crofty with a neutral shell locale. macOS uses AppleLanguages; Windows uses
// the UI culture. Returns "" on other platforms or on any error.
func osPreferredLang() string {
	switch runtime.GOOS {
	case "darwin":
		// AppleLanguages is a plist array, e.g. (\n  "ja-JP",\n  "en-US"\n).
		out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.Trim(strings.TrimSpace(line), "(),\"")
			if lang := langSubtag(line); lang != "" {
				return lang
			}
		}
	case "windows":
		// Get-UICulture.Name is the display language, e.g. "ja-JP" or "en-US".
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"(Get-UICulture).Name").Output()
		if err != nil {
			return ""
		}
		return langSubtag(strings.TrimSpace(string(out)))
	}
	return ""
}

// langSubtag extracts a lowercase language subtag from a locale tag:
// "ja-JP" → "ja", "zh-Hans" → "zh", "en_US" → "en". Returns "" if empty.
func langSubtag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	return strings.ToLower(strings.SplitN(strings.SplitN(tag, "-", 2)[0], "_", 2)[0])
}

// projectName derives a Hugo/Cloudflare-safe name from the target directory.
func projectName(abs string) string {
	return sanitizeName(filepath.Base(abs))
}

// sanitizeName lowercases and strips a string to a Cloudflare Pages-safe project
// slug (lowercase letters, digits, hyphens), since that name becomes the public
// <name>.pages.dev address. Falls back to "my-site" if nothing usable remains.
func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "my-site"
	}
	return s
}

// chooseNames resolves the two distinct names a site needs and keeps them apart:
//   - title: the free-text display name (a Japanese name is fine), shown on the site
//   - slug:  the deploy/project name, which becomes the public <slug>.pages.dev
//
// Flags win. Otherwise a terminal is asked — and the project prompt always spells
// out the .pages.dev URL, the consequence people miss when 'init .' silently
// adopts the folder name. A non-interactive run derives both from the folder
// (the caller announces the resulting URL so it's never a surprise).
func chooseNames(stdin *bufio.Reader, abs, titleFlag, projectFlag string, interactive bool) (title, slug string) {
	folder := filepath.Base(abs)
	title = strings.TrimSpace(titleFlag)
	slug = sanitizeName(projectFlag)
	if projectFlag == "" {
		slug = "" // distinguish "not given" from a value that sanitized to my-site
	}

	if interactive {
		if title == "" {
			fmt.Printf("Site title (shown on your site) [%s]: ", folder)
			line, _ := stdin.ReadString('\n')
			title = strings.TrimSpace(line)
		}
		if slug == "" {
			def := sanitizeName(firstNonEmpty(title, folder))
			fmt.Printf("Project name — your site will be published at <name>.pages.dev [%s]: ", def)
			line, _ := stdin.ReadString('\n')
			if entered := strings.TrimSpace(line); entered != "" {
				slug = sanitizeName(entered)
			} else {
				slug = def
			}
		}
	}

	if title == "" {
		title = folder
	}
	if slug == "" {
		slug = sanitizeName(firstNonEmpty(title, folder))
	}
	return title, slug
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
		"and `crofty deploy` puts it online (you'll connect a free account first).\n\n"+
		"## Keeping a post off your site\n\n"+
		"Two frontmatter fields control what gets published:\n\n"+
		"- Add `draft: true` to keep a post out of the built site — perfect while\n"+
		"  it's still a work in progress. Remove it (or set `false`) when it's ready.\n"+
		"- Give a post a future `date` and it stays unpublished until that day\n"+
		"  arrives — that's how you schedule ahead. `crofty build` tells you which\n"+
		"  posts it left out as drafts or future-dated, so nothing vanishes silently.\n",
		now.Format(time.RFC3339))
}
