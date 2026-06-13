package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

	if !runner.Look("hugo") {
		return fmt.Errorf("hugo not found on PATH.\n" +
			"crofty wraps Hugo to build your site. Install it (e.g. 'brew install hugo'), then run 'crofty build' again.")
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
	err = runner.Run(proj.Root, "hugo",
		"--source", proj.Root,
		"--themesDir", proj.ThemesDir(),
		"--theme", "crofty",
		"--destination", proj.DistDir(),
		"--cleanDestinationDir",
	)
	if err != nil {
		return fmt.Errorf("hugo build failed (your Markdown is untouched): %w", err)
	}

	fmt.Println()
	fmt.Println("✓ built →", proj.DistDir())
	warnFutureDated(filepath.Join(proj.Root, "content"), time.Now())
	fmt.Println("next:")
	fmt.Println("  crofty preview     # look at it locally first (no account)")
	fmt.Println("  crofty deploy      # put it online (connects a free Cloudflare account)")
	return nil
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
