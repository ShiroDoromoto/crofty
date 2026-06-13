package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/shirodoromoto/crofty/internal/id"
	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/publish"
	"github.com/shirodoromoto/crofty/internal/secret"
	"github.com/shirodoromoto/crofty/internal/spec"
	"github.com/shirodoromoto/crofty/internal/state"
)

func runPublish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	to := fs.String("to", "", "comma-separated targets (default: the post's crofty.targets)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt and publish")
	fs.Usage = func() {
		fmt.Println("crofty publish — syndicate a post's fragment to your destinations")
		fmt.Println("\nUsage:\n  crofty publish <article.md> [--to bluesky] [--yes]")
		fmt.Println("\nOnly the title, summary and a link back to your site are sent — never the body.")
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return errSilent
	}
	article := pos[0]

	proj, err := currentProject()
	if err != nil {
		return err
	}
	cfg, err := proj.LoadConfig()
	if err != nil {
		return err
	}
	contentDir := filepath.Join(proj.Root, "content")

	// Gate 1: never publish a post that fails the spec.
	if rep := spec.ValidateFile(article, contentDir); !rep.OK {
		fmt.Println("This post does not pass validate, so it will not be published:")
		renderHuman([]spec.FileReport{rep})
		return errSilent
	}

	fm, body, err := spec.ParseFile(article)
	if err != nil {
		return err
	}
	title, _ := fm["title"].(string)

	names, err := resolvePublishTargets(*to, fm)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no targets — pass --to bluesky or set crofty.targets in the post")
	}

	// Resolve credentials first, so a configuration error never mutates the post.
	publishers, err := buildPublishers(names, cfg)
	if err != nil {
		return err
	}

	baseURL, err := siteBaseURL(proj.Root)
	if err != nil {
		return err
	}
	canonical := canonicalURL(baseURL, contentDir, article, fm)

	// Assign a stable id on first publish and write it back to the post.
	newID, err := id.NewULID()
	if err != nil {
		return err
	}
	postID, created, err := spec.EnsureCroftyIDFile(article, newID)
	if err != nil {
		return fmt.Errorf("assigning crofty.id: %w", err)
	}
	if created {
		fmt.Printf("assigned crofty.id %s → saved to %s\n\n", postID, relCwd(article))
	}

	st, err := state.Load(proj.StatePath())
	if err != nil {
		return err
	}

	// Plan every target (no side effects), marking already-published, unchanged
	// ones to skip.
	type item struct {
		pub  publish.Publisher
		prev publish.Preview
		hash string
		skip bool
	}
	var items []item
	for _, pub := range publishers {
		summary := perTargetSummary(fm, body, pub.Name())
		frag := publish.Fragment{Title: title, Summary: summary, CanonicalURL: canonical}
		prev, err := pub.Plan(frag)
		if err != nil {
			return fmt.Errorf("%s: %w", pub.Name(), err)
		}
		h := state.Hash(title, summary, canonical)
		skip := false
		if rec, ok := st.Get(postID, pub.Name()); ok && rec.DescriptionHash == h {
			skip = true
		}
		items = append(items, item{pub, prev, h, skip})
	}

	// Confirmation gate (B-3): show what goes where, and that the body stays put.
	fmt.Println("Publish plan for", relCwd(article))
	fmt.Printf("  body → your site: %s  (stays here, not syndicated)\n", canonical)
	pending := 0
	for _, it := range items {
		if it.skip {
			fmt.Printf("  — %s: already published, unchanged — skipping\n", it.pub.Name())
			continue
		}
		pending++
		fmt.Printf("  → %s:\n", it.pub.Name())
		fmt.Printf("      text: %s\n", it.prev.Text)
		fmt.Printf("      card: %s\n", canonical)
		for _, n := range it.prev.Notes {
			fmt.Printf("      note: %s\n", n)
		}
	}
	if pending == 0 {
		fmt.Println("\nNothing to publish — everything is up to date.")
		return nil
	}
	fmt.Println("\nOnly the text and link above are sent. The body stays on your site.")

	if !*yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("publishing is irreversible — confirm in an interactive terminal, or pass --yes")
		}
		if !confirm("Publish now? [y/N]: ") {
			fmt.Println("Aborted. Nothing was sent.")
			return nil
		}
	}

	// Execute each target independently; one failure never stops the others, and
	// state is saved after each success so a retry won't double-post (A4 / ux §8).
	anyFail := false
	for _, it := range items {
		if it.skip {
			continue
		}
		res, err := it.pub.Execute(it.prev)
		if err != nil {
			fmt.Printf("✗ %s: %v\n   (your post and site are untouched)\n", it.pub.Name(), err)
			anyFail = true
			continue
		}
		st.Record(postID, it.pub.Name(), state.PublishRecord{
			PostID:          res.PostID,
			URL:             res.URL,
			PublishedAt:     res.PublishedAt,
			DescriptionHash: it.hash,
		})
		if err := st.Save(proj.StatePath()); err != nil {
			return fmt.Errorf("recording publish state: %w", err)
		}
		fmt.Printf("✓ %s: %s\n", it.pub.Name(), res.URL)
	}
	if anyFail {
		return errSilent
	}
	return nil
}

// resolvePublishTargets uses --to if given, else the post's crofty.targets.
func resolvePublishTargets(to string, fm spec.Frontmatter) ([]string, error) {
	if strings.TrimSpace(to) != "" {
		var names []string
		for _, t := range strings.Split(to, ",") {
			if t = strings.TrimSpace(t); t != "" {
				names = append(names, t)
			}
		}
		return names, nil
	}
	cm, ok := croftyMap(fm)
	if !ok {
		return nil, nil
	}
	list, ok := cm["targets"].([]any)
	if !ok {
		return nil, nil
	}
	var names []string
	for _, e := range list {
		if s, ok := e.(string); ok {
			names = append(names, s)
		}
	}
	return names, nil
}

func buildPublishers(names []string, cfg *project.Config) ([]publish.Publisher, error) {
	if cfg.Workspace == "" {
		return nil, fmt.Errorf("no targets configured — run 'crofty targets add bluesky'")
	}
	store := secret.New(cfg.Workspace)
	var pubs []publish.Publisher
	for _, name := range names {
		tc, ok := cfg.Targets[name]
		if !ok {
			return nil, fmt.Errorf("target %q is not configured — run 'crofty targets add %s'", name, name)
		}
		switch tc.Type {
		case "bluesky":
			pw, err := store.Get("bluesky", "app_password")
			if err != nil {
				return nil, fmt.Errorf("no stored credential for %s — run 'crofty targets add bluesky'", name)
			}
			pubs = append(pubs, publish.NewBluesky(name, publish.BlueskyCreds{
				Server: tc.Server, Handle: tc.Handle, AppPassword: pw,
			}))
		default:
			return nil, fmt.Errorf("target %q has unsupported type %q", name, tc.Type)
		}
	}
	return pubs, nil
}

// perTargetSummary resolves the summary for a channel: a channel override, else
// the canonical description, else an excerpt of the body.
func perTargetSummary(fm spec.Frontmatter, body []byte, target string) string {
	if cm, ok := croftyMap(fm); ok {
		if ov, ok := croftyChild(cm, "channel_overrides"); ok {
			if s, ok := ov[target].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	if d, ok := fm["description"].(string); ok && strings.TrimSpace(d) != "" {
		return d
	}
	return bodyExcerpt(body, 200)
}

func bodyExcerpt(body []byte, max int) string {
	s := strings.TrimSpace(string(body))
	if i := strings.Index(s, "\n\n"); i > 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}

// canonicalURL derives the post's URL from the site baseURL and its path under
// content/ (slug overrides the final segment). A v0 convention; see spec §11.
func canonicalURL(baseURL, contentDir, article string, fm spec.Frontmatter) string {
	abs, err := filepath.Abs(article)
	if err != nil {
		abs = article
	}
	rel, err := filepath.Rel(contentDir, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(article)
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	last := segs[len(segs)-1]

	var section []string
	var name string
	if (last == "index.md" || last == "index.markdown") && len(segs) >= 2 {
		name = segs[len(segs)-2]
		section = segs[:len(segs)-2]
	} else {
		name = strings.TrimSuffix(last, filepath.Ext(last))
		section = segs[:len(segs)-1]
	}
	if s, ok := fm["slug"].(string); ok && strings.TrimSpace(s) != "" {
		name = s
	}

	path := "/"
	if len(section) > 0 {
		path += strings.Join(section, "/") + "/"
	}
	path += name + "/"
	return strings.TrimRight(baseURL, "/") + strings.ToLower(path)
}

// siteBaseURL reads baseURL from the project's Hugo config.
func siteBaseURL(root string) (string, error) {
	for _, name := range []string{"hugo.yaml", "hugo.yml", "config.yaml", "config.yml"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		var c struct {
			BaseURL string `yaml:"baseURL"`
		}
		if err := yaml.Unmarshal(b, &c); err != nil {
			return "", fmt.Errorf("parsing %s: %w", name, err)
		}
		if strings.TrimSpace(c.BaseURL) != "" {
			return c.BaseURL, nil
		}
	}
	return "", fmt.Errorf("no baseURL found in the Hugo config (hugo.yaml)")
}

// croftyMap returns the crofty.* block as a string map, if present.
func croftyMap(fm spec.Frontmatter) (map[string]any, bool) {
	return asStringMap(fm["crofty"])
}

func croftyChild(cm map[string]any, key string) (map[string]any, bool) {
	return asStringMap(cm[key])
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case spec.Frontmatter:
		return map[string]any(m), true
	}
	return nil, false
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
