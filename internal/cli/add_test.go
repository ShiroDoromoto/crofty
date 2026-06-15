package cli

import (
	"strings"
	"testing"
)

// The embedded hooks are written verbatim into a user's project, so guard their
// shape: a render hook Hugo recognises, targeting the right language.
func TestRenderHookTemplates(t *testing.T) {
	if !strings.Contains(mermaidHook, `class="mermaid"`) {
		t.Error("mermaid hook lost its mermaid element")
	}
	if !strings.Contains(mermaidHook, "cdn.jsdelivr.net/npm/mermaid") {
		t.Error("mermaid hook lost its loader")
	}
	if !strings.Contains(abcHook, "abcjs") {
		t.Error("abc hook lost its abcjs loader")
	}
	// Render hooks must not be empty templates.
	for name, body := range map[string]string{"mermaid": mermaidHook, "abc": abcHook} {
		if len(strings.TrimSpace(body)) == 0 {
			t.Errorf("%s hook is empty", name)
		}
	}
}

func TestAdd_UnknownFeature(t *testing.T) {
	err := runAdd([]string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "unknown feature") {
		t.Errorf("expected unknown-feature error, got %v", err)
	}
}

// raw-html and highlight are guidance-only (no project needed, nothing written).
func TestAdd_GuidanceOnly(t *testing.T) {
	for _, feature := range []string{"raw-html", "highlight"} {
		out, err := captureStdout(t, func() error { return runAdd([]string{feature}) })
		if err != nil {
			t.Fatalf("add %s: %v", feature, err)
		}
		if !strings.Contains(out, "hugo.yaml") {
			t.Errorf("add %s should point at hugo.yaml, got: %s", feature, out)
		}
	}
}
