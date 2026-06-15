package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shirodoromoto/crofty/internal/project"
)

// hint is one "you wrote it but it won't show (yet)" note: a capability the
// content reaches for that isn't turned on. It never blocks — it teaches, which
// is the whole point (real-use feedback F: catch it the moment you write it).
type hint struct {
	File    string `json:"file"`    // the markdown file, relative to cwd
	Feature string `json:"feature"` // a crofty features name, so `crofty features` explains more
	Message string `json:"message"` // what's wrong and how to turn it on
}

// projectContext is the config a hint needs: what's already turned on, so we
// only nudge about things that are genuinely off.
type projectContext struct {
	unsafe       bool   // markup.goldmark.renderer.unsafe — raw HTML passes through
	defaultLang  string // defaultContentLanguage (Hugo's default is "en")
	multilingual bool   // a languages: block with >1 language
	hasMermaidHk bool   // project ships a mermaid render hook
	hasABCHk     bool   // project ships an abc render hook
}

// gatherContext reads the project's hugo.yaml and layouts so hints can tell
// "off" from "already set up". Best-effort: missing/odd config just means fewer
// hints, never an error (this is an advisory pass).
func gatherContext(proj *project.Project) projectContext {
	ctx := projectContext{defaultLang: "en"}
	if proj == nil {
		return ctx
	}
	ctx.hasMermaidHk = fileExists(filepath.Join(proj.Root, "layouts", "_default", "_markup", "render-codeblock-mermaid.html"))
	ctx.hasABCHk = fileExists(filepath.Join(proj.Root, "layouts", "_default", "_markup", "render-codeblock-abc.html"))

	b, err := os.ReadFile(filepath.Join(proj.Root, "hugo.yaml"))
	if err != nil {
		return ctx
	}
	var cfg map[string]any
	if yaml.Unmarshal(b, &cfg) != nil {
		return ctx
	}
	if s, ok := cfg["defaultContentLanguage"].(string); ok && s != "" {
		ctx.defaultLang = s
	}
	if langs, ok := cfg["languages"].(map[string]any); ok && len(langs) > 1 {
		ctx.multilingual = true
	}
	ctx.unsafe = digBool(cfg, "markup", "goldmark", "renderer", "unsafe")
	return ctx
}

// hintsFor scans one markdown file's body against the project context and
// returns any capability notes. Conservative by design: it only fires on clear
// signals so the advice stays trustworthy.
func hintsFor(path string, ctx projectContext) []hint {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	body := stripFrontMatter(string(data))
	var hints []hint
	add := func(feature, msg string) {
		hints = append(hints, hint{File: relCwd(path), Feature: feature, Message: msg})
	}

	if !ctx.hasMermaidHk && fencedLang(body, "mermaid") {
		add("mermaid", "a ```mermaid block stays plain text — diagrams need a render hook (see 'crofty features').")
	}
	if !ctx.hasABCHk && fencedLang(body, "abc") {
		add("abc", "an ```abc block stays plain text — music notation needs a render hook (see 'crofty features').")
	}
	if !ctx.unsafe && hasBlockHTML(body) {
		add("raw-html", "raw HTML (e.g. <figure>) is dropped unless markup.goldmark.renderer.unsafe: true in hugo.yaml.")
	}
	if ctx.multilingual {
		if lang := translationLang(path); lang != "" && lang != ctx.defaultLang && hasRelativeImage(body) {
			add("multilingual", "a translated post's relative image resolves under /"+ctx.defaultLang+"/… only and 404s here — use an absolute /section/slug/ path.")
		}
	}
	return hints
}

// --- detectors (kept narrow to avoid false positives) --------------------

var fencedRe = regexp.MustCompile("(?m)^[ \t]*```+[ \t]*([A-Za-z0-9_-]+)")

func fencedLang(body, lang string) bool {
	for _, m := range fencedRe.FindAllStringSubmatch(body, -1) {
		if strings.EqualFold(m[1], lang) {
			return true
		}
	}
	return false
}

// hasBlockHTML looks for a raw block-level HTML tag at the start of a line —
// the case goldmark drops without `unsafe`. Inline HTML inside a paragraph is
// left alone (it's rarer and noisier to flag).
var blockHTMLRe = regexp.MustCompile(`(?mi)^[ \t]*<(figure|video|audio|iframe|details|table|div|section|picture|source)[\s/>]`)

func hasBlockHTML(body string) bool {
	return blockHTMLRe.MatchString(body)
}

// hasRelativeImage finds a Markdown image whose target is a bundle-relative path
// (not absolute, not a URL) — the multilingual page-bundle footgun.
var imageRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*([^)\s]+)`)

func hasRelativeImage(body string) bool {
	for _, m := range imageRe.FindAllStringSubmatch(body, -1) {
		target := m[1]
		if target == "" || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "data:") {
			continue
		}
		if strings.Contains(target, "://") {
			continue
		}
		return true
	}
	return false
}

// translationLang returns the language code of a translated leaf-bundle file
// like "index.ja.md" → "ja", or "" for "index.md" / non-translation names.
func translationLang(path string) string {
	base := filepath.Base(path)
	parts := strings.Split(base, ".")
	if len(parts) < 3 {
		return ""
	}
	code := parts[len(parts)-2]
	if len(code) < 2 || len(code) > 3 {
		return ""
	}
	for _, r := range code {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	return code
}

// --- small helpers --------------------------------------------------------

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func stripFrontMatter(s string) string {
	if strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n") {
		if i := strings.Index(s[3:], "\n---"); i >= 0 {
			rest := s[3+i+4:]
			if j := strings.IndexByte(rest, '\n'); j >= 0 {
				return rest[j+1:]
			}
			return ""
		}
	}
	return s
}

// digBool walks a nested map by keys and returns the bool at the end (false if
// any step is missing or not the expected type).
func digBool(m map[string]any, keys ...string) bool {
	cur := any(m)
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		cur = mm[k]
	}
	b, _ := cur.(bool)
	return b
}
