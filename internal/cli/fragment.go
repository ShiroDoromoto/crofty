package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shirodoromoto/crofty/internal/project"
	"github.com/shirodoromoto/crofty/internal/spec"
)

// This file holds the helpers `share` uses to compose a post's fragment — the
// title, summary and canonical link. crofty composes fragments; it does not post
// them. The body is never part of a fragment, by design.

func currentProject() (*project.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.Find(cwd)
}

// frontmatterChannels uses --to if given, else the post's crofty.targets.
func frontmatterChannels(to string, fm spec.Frontmatter) ([]string, error) {
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

// liveness is the result of probing whether a post is reachable on the live site.
type liveness int

const (
	liveUnknown liveness = iota // could not tell (network error, timeout)
	liveYes                     // reachable (2xx/3xx)
	liveNo                      // definitely absent (404/4xx)
)

// checkLive probes the canonical URL on the user's own site. It is a package var
// so tests can stub it without hitting the network.
var checkLive = func(url string) liveness {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Head(url)
	if err != nil {
		return liveUnknown
	}
	// Some hosts reject HEAD; fall back to GET before trusting a 4xx.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		resp.Body.Close()
		if resp, err = client.Get(url); err != nil {
			return liveUnknown
		}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		return liveYes
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return liveNo
	default:
		return liveUnknown
	}
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
