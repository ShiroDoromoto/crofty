package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// runLang groups the multilingual commands. Going from one language to two was
// the slowest part of real use: the languages: block, defaultContentLanguage and
// the per-language label/locale keys all had to be hand-written from the demo.
// `crofty lang add` writes the content stub it owns and shows the exact hugo.yaml
// to paste — config stays the author's to keep (same stance as `crofty init`).
func runLang(args []string) error {
	if len(args) == 0 {
		langUsage()
		return nil
	}
	switch args[0] {
	case "add":
		return runLangAdd(args[1:])
	case "list":
		return runLangList(args[1:])
	case "-h", "--help", "help":
		langUsage()
		return nil
	default:
		return fmt.Errorf("unknown lang subcommand %q (try: crofty lang add <code> | crofty lang list)", args[0])
	}
}

func langUsage() {
	fmt.Println("crofty lang — add or list the languages your site is written in")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty lang add <code>   # e.g. 'crofty lang add ja' — show the config + make a stub")
	fmt.Println("  crofty lang list         # the languages configured now")
}

// langCodeRe is a conservative language-subtag check (ISO 639-style): 2–3 lower
// letters, optionally a -REGION suffix we keep as-is (e.g. zh-Hant, pt-BR).
var langCodeRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z]{2,4})?$`)

func runLangAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("which language? e.g. 'crofty lang add ja'")
	}
	code := strings.TrimSpace(args[0])
	if !langCodeRe.MatchString(code) {
		return fmt.Errorf("%q doesn't look like a language code — try a code like 'ja', 'en', 'fr', 'zh-Hant'", code)
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	cfg, err := loadHugoConfig(proj.Root)
	if err != nil {
		return err
	}

	existing := configuredLanguages(cfg)
	defaultLang := defaultLanguage(cfg)
	if _, ok := existing[code]; ok {
		fmt.Printf("%s is already configured. See 'crofty lang list'.\n", code)
		return nil
	}

	// The content stub is crofty's to write: a translated homepage so the new
	// language isn't empty. Posts stay the author's to translate.
	stub := filepath.Join(proj.Root, "content", "_index."+code+".md")
	wroteStub := false
	if !fileExists(stub) {
		title := currentTitle(cfg)
		body := fmt.Sprintf("---\ntitle: %q\ndescription: \"\"\n---\n\n", title) +
			"<!-- This is the " + code + " homepage. Translate the title above and write here. -->\n"
		if err := os.WriteFile(stub, []byte(body), 0o644); err != nil {
			return err
		}
		wroteStub = true
	}

	fmt.Printf("Adding %s (%s).\n\n", langDisplay(code), code)
	if len(existing) == 0 {
		printFirstLanguageBlock(defaultLang, currentTitle(cfg), code)
	} else {
		printAdditionalLanguageBlock(existing, code)
	}

	fmt.Println()
	if wroteStub {
		fmt.Printf("✓ wrote content/_index.%s.md (the %s homepage — translate it)\n", code, code)
	} else {
		fmt.Printf("· content/_index.%s.md already exists — left as is\n", code)
	}
	fmt.Println()
	fmt.Println("Then translate posts as index." + code + ".md beside each index.md, and")
	fmt.Println("'crofty preview' to see the language switch. (For a post's images, use an")
	fmt.Println("absolute /section/slug/ path so the translation finds them — see 'crofty validate'.)")
	return nil
}

func runLangList(args []string) error {
	proj, err := findProject()
	if err != nil {
		return err
	}
	cfg, err := loadHugoConfig(proj.Root)
	if err != nil {
		return err
	}
	langs := configuredLanguages(cfg)
	if len(langs) == 0 {
		fmt.Printf("This site is single-language (%s). Add one with 'crofty lang add <code>'.\n", defaultLanguage(cfg))
		return nil
	}
	def := defaultLanguage(cfg)
	codes := make([]string, 0, len(langs))
	for c := range langs {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	fmt.Println("Languages configured (hugo.yaml):")
	for _, c := range codes {
		mark := "  "
		if c == def {
			mark = "★ " // default content language
		}
		fmt.Printf("    %s%-8s %s\n", mark, c, langDisplay(c))
	}
	fmt.Printf("\n★ = default content language (%s)\n", def)
	return nil
}

// --- config helpers -------------------------------------------------------

func loadHugoConfig(root string) (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(root, "hugo.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading hugo.yaml: %w", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parsing hugo.yaml: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// configuredLanguages returns the language codes already in the languages: block
// (empty when the site is still single-language).
func configuredLanguages(cfg map[string]any) map[string]any {
	langs, _ := cfg["languages"].(map[string]any)
	if langs == nil {
		return map[string]any{}
	}
	return langs
}

// defaultLanguage is defaultContentLanguage if set, else the top-level locale,
// else Hugo's own default ("en").
func defaultLanguage(cfg map[string]any) string {
	if s, ok := cfg["defaultContentLanguage"].(string); ok && s != "" {
		return s
	}
	if s, ok := cfg["locale"].(string); ok && s != "" {
		return s
	}
	return "en"
}

func currentTitle(cfg map[string]any) string {
	if s, ok := cfg["title"].(string); ok && s != "" {
		return s
	}
	return "My site"
}

// --- the config blocks we ask the author to paste -------------------------

func printFirstLanguageBlock(defaultLang, title, newCode string) {
	fmt.Println("Add this to hugo.yaml (it turns the site multilingual — your current")
	fmt.Println("top-level `title`/`locale` move into the default language below):")
	fmt.Println()
	fmt.Printf("    defaultContentLanguage: %q\n", defaultLang)
	fmt.Println("    defaultContentLanguageInSubdir: false")
	fmt.Println("    languages:")
	printLangEntry(defaultLang, title, 1)
	printLangEntry(newCode, "", 2)
}

func printAdditionalLanguageBlock(existing map[string]any, newCode string) {
	fmt.Println("Add this language under the existing `languages:` block in hugo.yaml:")
	fmt.Println()
	printLangEntry(newCode, "", len(existing)+1)
}

// printLangEntry prints one languages.<code> entry with the keys the theme reads
// (label, locale) plus weight and a title placeholder. Indented for a 2-space
// YAML map nested under `languages:`.
func printLangEntry(code, title string, weight int) {
	fmt.Printf("      %s:\n", code)
	fmt.Printf("        locale: %q\n", code)
	fmt.Printf("        label: %q\n", langLabel(code))
	fmt.Printf("        weight: %d\n", weight)
	if title != "" {
		fmt.Printf("        title: %q\n", title)
	} else {
		fmt.Printf("        title: \"%s — translate me\"\n", langDisplay(code))
	}
}

// langLabel is the short switcher label the theme shows (.Language.Label).
func langLabel(code string) string {
	if l, ok := commonLangLabels[code]; ok {
		return l
	}
	return strings.ToUpper(code)
}

// langDisplay is a human name for messages (falls back to the code).
func langDisplay(code string) string {
	if n, ok := commonLangNames[code]; ok {
		return n
	}
	return code
}

var commonLangLabels = map[string]string{
	"en": "EN", "ja": "日本語", "fr": "FR", "de": "DE", "es": "ES",
	"it": "IT", "pt": "PT", "ko": "한국어", "zh": "中文", "nl": "NL",
}

var commonLangNames = map[string]string{
	"en": "English", "ja": "Japanese", "fr": "French", "de": "German",
	"es": "Spanish", "it": "Italian", "pt": "Portuguese", "ko": "Korean",
	"zh": "Chinese", "nl": "Dutch",
}
