package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/runner"
	"github.com/shirodoromoto/crofty/internal/spec"
	"github.com/shirodoromoto/crofty/internal/theme"
)

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("crofty build — render the site to ./dist")
		fmt.Println("\nUsage: crofty build")
	}
	if err := fs.Parse(args); err != nil {
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

	if err := buildSite(proj); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✓ built →", proj.DistDir())
	contentDir := filepath.Join(proj.Root, "content")
	warnDrafts(contentDir)
	warnFutureDated(contentDir, time.Now())
	contractNotice(proj.DistDir())
	optionalSetupHint(proj.Root)
	fmt.Println("next:")
	fmt.Println("  crofty preview     # look at it locally first (no account)")
	fmt.Println("  crofty deploy      # put it online (connects a free Cloudflare account)")
	return nil
}

// buildSite renders the project to ./dist with Hugo, materializing the bundled
// theme first. It is the shared core of `crofty build` and of the build that
// `crofty deploy` runs before publishing — so a deploy always ships the current
// source, never a stale ./dist left behind after an edit.
func buildSite(proj *project.Project) error {
	if !runner.Look("hugo") {
		return fmt.Errorf("hugo not found on PATH.\n" +
			"crofty wraps Hugo to build your site. Install it (e.g. 'brew install hugo'), then run the command again.")
	}
	// Materialize the bundled theme into .crofty/themes/crofty each build so the
	// copy embedded in the binary stays the single source of truth.
	themeDst := filepath.Join(proj.ThemesDir(), "crofty")
	if err := theme.Materialize(themeDst); err != nil {
		return fmt.Errorf("writing bundled theme: %w", err)
	}
	// Run Hugo against the project root. .crofty/ holds the theme and tool state
	// and is never rendered into the output, so nothing from it can ride along
	// to deploy.
	if err := runner.Run(proj.Root, "hugo",
		"--source", proj.Root,
		"--themesDir", proj.ThemesDir(),
		"--theme", "crofty",
		"--destination", proj.DistDir(),
		"--cleanDestinationDir",
	); err != nil {
		return fmt.Errorf("hugo build failed (your Markdown is untouched): %w", err)
	}
	return nil
}

// draftPosts returns content files marked `draft: true`. Hugo silently excludes
// these from the build (buildDrafts is off by default) — the same "deploy
// succeeds while the post 404s" trap as a future date, just from a different
// field. crofty never gates on draft; this only surfaces the omission so it's
// never a surprise.
func draftPosts(contentDir string) []string {
	if fi, err := os.Stat(contentDir); err != nil || !fi.IsDir() {
		return nil
	}
	files, err := collectMarkdown([]string{contentDir})
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range files {
		fm, _, err := spec.ParseFile(f)
		if err != nil {
			continue
		}
		if isDraft(fm["draft"]) {
			out = append(out, f)
		}
	}
	return out
}

// isDraft reports whether a front-matter draft value means "draft" — a YAML
// bool true, or the string "true" (some authors quote it).
func isDraft(v any) bool {
	switch d := v.(type) {
	case bool:
		return d
	case string:
		return strings.EqualFold(strings.TrimSpace(d), "true")
	}
	return false
}

// warnDrafts prints an advisory (never an error) when posts were left out of the
// build for being drafts.
func warnDrafts(contentDir string) {
	drafts := draftPosts(contentDir)
	if len(drafts) == 0 {
		return
	}
	noun := "post is a draft"
	if len(drafts) > 1 {
		noun = "posts are drafts"
	}
	fmt.Println()
	fmt.Printf("⚠ %d %s, left out of this build:\n", len(drafts), noun)
	for _, f := range drafts {
		fmt.Printf("    %s\n", relCwd(f))
	}
	fmt.Println("  Hugo excludes drafts. To publish one, remove 'draft: true' from its")
	fmt.Println("  frontmatter (or set it to false) and rebuild.")
	fmt.Println()
}

// futurePost is a content file Hugo will omit because its date is still ahead.
type futurePost struct {
	path string
	when time.Time
}

// futureDatedPosts returns content files dated after now. Hugo silently excludes
// these from the build (buildFuture is off by default), so a deploy can "succeed"
// while the newest post 404s — the worst dogfood trap to hit blind.
func futureDatedPosts(contentDir string, now time.Time) []futurePost {
	if fi, err := os.Stat(contentDir); err != nil || !fi.IsDir() {
		return nil
	}
	files, err := collectMarkdown([]string{contentDir})
	if err != nil {
		return nil
	}
	var out []futurePost
	for _, f := range files {
		fm, _, err := spec.ParseFile(f)
		if err != nil {
			continue
		}
		if t, ok := spec.ParseDate(fm["date"]); ok && t.After(now) {
			out = append(out, futurePost{path: f, when: t})
		}
	}
	return out
}

// warnFutureDated prints an advisory (never an error) when posts were left out
// of the build for being future-dated.
func warnFutureDated(contentDir string, now time.Time) {
	futures := futureDatedPosts(contentDir, now)
	if len(futures) == 0 {
		return
	}
	noun := "post is"
	if len(futures) > 1 {
		noun = "posts are"
	}
	fmt.Println()
	fmt.Printf("⚠ %d %s dated in the future and left out of this build:\n", len(futures), noun)
	for _, f := range futures {
		fmt.Printf("    %s  (%s)\n", relCwd(f.path), f.when.Format(time.RFC3339))
	}
	fmt.Println("  Hugo excludes future-dated posts. If your newest post is missing,")
	fmt.Println("  set its date to now or earlier and rebuild.")
	fmt.Println()
}
