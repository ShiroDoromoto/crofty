package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// tokenDecl matches a CSS custom-property declaration (`--name:`), not a usage
// (`var(--name)`) or a mention in prose, so we compare the variables a file
// actually defines.
var tokenDecl = regexp.MustCompile(`--([a-z-]+)\s*:`)

func declaredTokens(css string) []string {
	seen := map[string]bool{}
	for _, m := range tokenDecl.FindAllStringSubmatch(css, -1) {
		seen[m[1]] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestCustomCSSMatchesThemeTokens is the drift guard: the starter override that
// `crofty theme eject` writes must declare exactly the tokens the bundled theme
// defines, or an eject would silently miss (or invent) variables.
func TestCustomCSSMatchesThemeTokens(t *testing.T) {
	b, err := fs.ReadFile(FS(), "static/css/crofty.css")
	if err != nil {
		t.Fatalf("reading bundled crofty.css: %v", err)
	}
	themeTokens := strings.Join(declaredTokens(string(b)), " ")
	customTokens := strings.Join(declaredTokens(CustomCSS), " ")
	if themeTokens != customTokens {
		t.Errorf("token drift between crofty.css and CustomCSS:\n  theme:  %s\n  custom: %s", themeTokens, customTokens)
	}
}

func TestEjectFullWritesAndSkips(t *testing.T) {
	dst := t.TempDir()

	written, skipped, err := EjectFull(dst, false)
	if err != nil {
		t.Fatalf("EjectFull: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("EjectFull wrote nothing")
	}
	if len(skipped) != 0 {
		t.Fatalf("first eject skipped files: %v", skipped)
	}

	// A known layout lands at the override path; theme.toml is not ejected.
	if _, err := os.Stat(filepath.Join(dst, "layouts", "_default", "baseof.html")); err != nil {
		t.Errorf("expected baseof.html ejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "theme.toml")); !os.IsNotExist(err) {
		t.Errorf("theme.toml should not be ejected, stat err = %v", err)
	}

	// Re-running without force clobbers nothing.
	written2, skipped2, err := EjectFull(dst, false)
	if err != nil {
		t.Fatalf("second EjectFull: %v", err)
	}
	if len(written2) != 0 {
		t.Errorf("second eject rewrote %d file(s), want 0", len(written2))
	}
	if len(skipped2) != len(written) {
		t.Errorf("second eject skipped %d, want %d", len(skipped2), len(written))
	}
}
