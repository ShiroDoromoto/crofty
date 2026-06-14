package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirodoromoto/crofty/internal/spec"
)

// shareChannel describes a destination for the auth-free `share` command: a text
// budget and, where the platform offers one, a builder for a pre-filled compose
// link. share touches no credentials and makes no API calls — it only composes
// the fragment (title + summary + canonical link) for the user (or their agent)
// to paste or open. The body never appears here, same as publish.
type shareChannel struct {
	name   string
	max    int                               // text budget in runes; 0 = no limit / link-only composer
	intent func(summary, link string) string // pre-filled composer URL, or nil if none
}

// shareChannels is the menu share knows about. Compose-intent URLs are provided
// only where the platform has a stable, public one; the rest fall back to a
// plain text block (which works anywhere, including platforms with no API).
var shareChannels = []shareChannel{
	{"bluesky", 300, nil},  // no stable public compose intent → plain text
	{"mastodon", 500, nil}, // a /share intent needs the instance host (unknown without config)
	{"x", 280, func(s, l string) string {
		return "https://twitter.com/intent/tweet?text=" + url.QueryEscape(s) + "&url=" + url.QueryEscape(l)
	}},
	{"threads", 500, func(s, l string) string {
		return "https://www.threads.net/intent/post?text=" + url.QueryEscape(strings.TrimSpace(s+" "+l))
	}},
	{"linkedin", 0, func(s, l string) string {
		return "https://www.linkedin.com/sharing/share-offsite/?url=" + url.QueryEscape(l)
	}},
	{"facebook", 0, func(s, l string) string {
		return "https://www.facebook.com/sharer/sharer.php?u=" + url.QueryEscape(l)
	}},
}

func shareChannelByName(name string) (shareChannel, bool) {
	for _, c := range shareChannels {
		if c.name == name {
			return c, true
		}
	}
	return shareChannel{}, false
}

// shareSnippet is one channel's ready-to-use output. The JSON shape is the
// machine surface an agent consumes.
type shareSnippet struct {
	Channel string `json:"channel"`
	Limit   int    `json:"limit,omitempty"`
	Text    string `json:"text"`
	Intent  string `json:"intent,omitempty"`
	Note    string `json:"note,omitempty"`
}

func runShare(args []string) error {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	to := fs.String("to", "", "comma-separated channels (default: the post's crofty.targets, else all known)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON (for your agent)")
	plain := fs.Bool("plain", false, "emit only the plain text + link (handy for | pbcopy)")
	skipDeployCheck := fs.Bool("skip-deploy-check", false, "print even if the post isn't live on your site yet")
	fs.Usage = func() {
		fmt.Println("crofty share — print a ready-to-post fragment (text + link) for any SNS")
		fmt.Println("\nUsage:\n  crofty share <article.md> [--to x,bluesky] [--json] [--plain]")
		fmt.Println("\nNo credentials, no posting: it composes the snippet and compose links for")
		fmt.Println("you (or your agent) to paste or open. Only the title, summary and canonical")
		fmt.Println("link are produced — never the body.")
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
	contentDir := filepath.Join(proj.Root, "content")

	fm, body, err := spec.ParseFile(article)
	if err != nil {
		return err
	}
	title, _ := fm["title"].(string)
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%s has no title in its frontmatter", relCwd(article))
	}

	baseURL, err := siteBaseURL(proj.Root)
	if err != nil {
		return err
	}
	canonical := canonicalURL(baseURL, contentDir, article, fm)

	// --plain: one generic snippet, nothing else — clean for `| pbcopy`.
	if *plain {
		fmt.Println(plainShare(perTargetSummary(fm, body, ""), canonical))
		return nil
	}

	// Passive deploy note: share only prints (it posts nothing), so a not-live
	// link is a warning, not a block. We probe the canonical URL on the user's
	// own site — the same liveness mechanism publish uses, not phone-home.
	liveNote := ""
	if !*skipDeployCheck && checkLive(canonical) == liveNo {
		liveNote = "not live yet — run 'crofty deploy' first, or this link will 404 for readers"
	}

	names, err := resolveShareChannels(*to, fm)
	if err != nil {
		return err
	}

	var snippets []shareSnippet
	for _, name := range names {
		ch, known := shareChannelByName(name)
		if !known {
			ch = shareChannel{name: name} // unknown channel still gets a plain snippet
		}
		summary := perTargetSummary(fm, body, name)
		text, note := composeShareText(summary, canonical, ch.max)
		intent := ""
		if ch.intent != nil {
			trimmed, _ := trimSummary(summary, canonical, ch.max)
			intent = ch.intent(trimmed, canonical)
		}
		snippets = append(snippets, shareSnippet{Channel: name, Limit: ch.max, Text: text, Intent: intent, Note: note})
	}
	plainText := plainShare(perTargetSummary(fm, body, ""), canonical)

	if *asJSON {
		out := struct {
			Post     string         `json:"post"`
			Title    string         `json:"title"`
			Link     string         `json:"link"`
			Warning  string         `json:"warning,omitempty"`
			Channels []shareSnippet `json:"channels"`
			Plain    string         `json:"plain"`
		}{relCwd(article), title, canonical, liveNote, snippets, plainText}
		// Disable HTML-escaping so the & in intent URLs stays readable (&
		// is valid JSON but ugly when a human reads or pipes the output).
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("Share %q\n", title)
	fmt.Printf("  link: %s\n", canonical)
	if liveNote != "" {
		fmt.Printf("  ⚠ %s\n", liveNote)
	}
	fmt.Println()
	for _, s := range snippets {
		label := s.Channel
		if s.Limit > 0 {
			label = fmt.Sprintf("%s (%d)", s.Channel, s.Limit)
		}
		fmt.Printf("%s:\n  %s\n", label, s.Text)
		if s.Intent != "" {
			fmt.Printf("  → %s\n", s.Intent)
		}
		if s.Note != "" {
			fmt.Printf("  note: %s\n", s.Note)
		}
		fmt.Println()
	}
	fmt.Println("Plain (copy anywhere):")
	fmt.Printf("  %s\n", plainText)
	return nil
}

// resolveShareChannels uses --to, else the post's crofty.targets, else every
// known channel as a discovery menu.
func resolveShareChannels(to string, fm spec.Frontmatter) ([]string, error) {
	names, err := frontmatterChannels(to, fm)
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		return names, nil
	}
	out := make([]string, 0, len(shareChannels))
	for _, c := range shareChannels {
		out = append(out, c.name)
	}
	return out, nil
}

// trimSummary trims summary so that "summary <space> link" fits within max runes
// (max <= 0 means no limit). Returns the summary and whether it was trimmed.
func trimSummary(summary, link string, max int) (string, bool) {
	summary = strings.TrimSpace(summary)
	if max <= 0 {
		return summary, false
	}
	budget := max - len([]rune(link)) - 1 // one space before the link
	if budget < 1 {
		return "", summary != "" // link alone fills the budget; no room for text
	}
	if r := []rune(summary); len(r) > budget {
		return strings.TrimSpace(string(r[:budget-1])) + "…", true
	}
	return summary, false
}

// composeShareText returns the paste-ready "summary link" text and a note if the
// summary had to be trimmed to fit the channel's limit.
func composeShareText(summary, link string, max int) (text, note string) {
	trimmed, didTrim := trimSummary(summary, link, max)
	if didTrim {
		note = fmt.Sprintf("trimmed to fit %d characters", max)
	}
	if trimmed == "" {
		return link, note
	}
	return trimmed + " " + link, note
}

func plainShare(summary, link string) string {
	if summary = strings.TrimSpace(summary); summary == "" {
		return link
	}
	return summary + " " + link
}
