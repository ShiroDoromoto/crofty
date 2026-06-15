package cli

import (
	"path/filepath"
	"testing"
)

// A bare project (no render hooks, no unsafe) should surface the mermaid and
// raw-HTML hints; a wired one should stay quiet.
func TestHints_FireWhenOff_QuietWhenOn(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "index.md")
	write(t, md, "---\ntitle: T\n---\n\n```mermaid\ngraph TD; A-->B\n```\n\n<figure><img src=\"x.png\"></figure>\n")

	off := projectContext{defaultLang: "en"}
	hs := hintsFor(md, off)
	if !hasFeature(hs, "mermaid") {
		t.Error("expected a mermaid hint when no render hook is present")
	}
	if !hasFeature(hs, "raw-html") {
		t.Error("expected a raw-html hint when unsafe is off")
	}

	on := projectContext{defaultLang: "en", unsafe: true, hasMermaidHk: true}
	if hs := hintsFor(md, on); len(hs) != 0 {
		t.Errorf("expected no hints when wired up, got %+v", hs)
	}
}

// The multilingual footgun: a translated bundle with a relative image, on a
// multilingual site, warns — but only for the non-default language.
func TestHints_TranslatedRelativeImage(t *testing.T) {
	dir := t.TempDir()
	ja := filepath.Join(dir, "index.ja.md")
	write(t, ja, "---\ntitle: T\n---\n\n![diagram](chart.svg)\n")
	en := filepath.Join(dir, "index.md")
	write(t, en, "---\ntitle: T\n---\n\n![diagram](chart.svg)\n")

	ctx := projectContext{defaultLang: "en", multilingual: true}
	if !hasFeature(hintsFor(ja, ctx), "multilingual") {
		t.Error("expected a multilingual hint for a translated post's relative image")
	}
	if hasFeature(hintsFor(en, ctx), "multilingual") {
		t.Error("default-language post should not get the multilingual hint")
	}
	// An absolute path is the documented fix — no hint.
	abs := filepath.Join(dir, "index.fr.md")
	write(t, abs, "---\ntitle: T\n---\n\n![diagram](/posts/x/chart.svg)\n")
	if hasFeature(hintsFor(abs, ctx), "multilingual") {
		t.Error("absolute image path should not trigger the multilingual hint")
	}
}

// A fenced block whose info string merely contains the word shouldn't false-fire
// the way a plain prose mention would; and inline-only HTML stays quiet.
func TestHints_NoFalsePositives(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "index.md")
	write(t, md, "---\ntitle: T\n---\n\nI like mermaid diagrams in theory.\n\nUse the <kbd>Tab</kbd> key inline.\n")
	if hs := hintsFor(md, projectContext{defaultLang: "en"}); len(hs) != 0 {
		t.Errorf("prose mention / inline HTML should not produce hints, got %+v", hs)
	}
}

func hasFeature(hs []hint, feature string) bool {
	for _, h := range hs {
		if h.Feature == feature {
			return true
		}
	}
	return false
}
