package cli

import (
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/theme"
)

// The `theme tokens` catalogue must stay in sync with the variables the bundled
// starter (theme.CustomCSS) actually defines — otherwise the command would
// advertise tokens that editing custom.css wouldn't change.
func TestThemeTokens_MatchStarter(t *testing.T) {
	for _, tok := range themeTokens() {
		if !strings.Contains(theme.CustomCSS, tok.Name+":") {
			t.Errorf("token %q is listed by `theme tokens` but not defined in theme.CustomCSS", tok.Name)
		}
	}
}

// `theme eject --print` writes the starter tokens to stdout and touches nothing.
func TestThemeEject_Print(t *testing.T) {
	out, err := captureStdout(t, func() error { return runThemeEject([]string{"--print"}) })
	if err != nil {
		t.Fatal(err)
	}
	if out != theme.CustomCSS {
		t.Errorf("--print should emit theme.CustomCSS verbatim")
	}
}
