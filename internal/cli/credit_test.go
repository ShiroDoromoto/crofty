package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

func TestAskFooterCreditChoice(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		decided bool
	}{
		{"keep short", "k\n", project.FooterCreditOn, true},
		{"keep word", "keep\n", project.FooterCreditOn, true},
		{"yes alias", "y\n", project.FooterCreditOn, true},
		{"remove short", "r\n", project.FooterCreditOff, true},
		{"remove word", "remove\n", project.FooterCreditOff, true},
		{"no alias", "no\n", project.FooterCreditOff, true},
		{"case + spaces", "  Keep  \n", project.FooterCreditOn, true},
		{"reprompt then valid", "\nhuh\nr\n", project.FooterCreditOff, true},
		{"eof no answer", "", "", false},
		{"only invalid then eof", "maybe\n", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, decided := askFooterCreditChoice(strings.NewReader(c.input), io.Discard)
			if got != c.want || decided != c.decided {
				t.Fatalf("askFooterCreditChoice(%q) = (%q, %v), want (%q, %v)", c.input, got, decided, c.want, c.decided)
			}
		})
	}
}

// maybeAskFooterCredit must never re-ask once the choice exists — it's a one-time
// neutral prompt, and on/off must both be treated as "already decided".
func TestMaybeAskFooterCreditNoOpWhenDecided(t *testing.T) {
	for _, v := range []string{project.FooterCreditOn, project.FooterCreditOff} {
		proj := &project.Project{Root: t.TempDir()}
		cfg := &project.Config{FooterCredit: v}
		// No config file is written; if this tried to save (i.e. didn't treat the
		// value as decided) it would still be a no-op here, but the contract we
		// assert is that the value is left untouched.
		if err := maybeAskFooterCredit(proj, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.FooterCredit != v {
			t.Fatalf("FooterCredit changed from %q to %q", v, cfg.FooterCredit)
		}
	}
}
