package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

func deniedFixture() *access.Denied {
	return &access.Denied{
		Op:   "write the design tokens to assets/css/custom.css",
		Path: "/site/assets/css/custom.css",
		Err:  &fs.PathError{Op: "open", Path: "/site/assets/css/custom.css", Err: fs.ErrPermission},
		Choices: []access.Choice{
			{Do: "let crofty write into the project folder", Command: "crofty theme eject", Permission: "write access to /site"},
			{Do: "print the tokens instead", Command: "crofty theme eject --print"},
		},
	}
}

// TestPrintDenied_ShowsBranchNotFailure: the text must carry what crofty tried,
// where, why, each choice — and the norm that stops an agent from inventing a
// way around it.
func TestPrintDenied_ShowsBranchNotFailure(t *testing.T) {
	var buf bytes.Buffer
	printDenied(&buf, deniedFixture())
	out := buf.String()

	for _, want := range []string{
		"crofty needs your permission",
		"write the design tokens",
		"/site/assets/css/custom.css",
		"permission denied",
		"needs your permission: write access to /site",
		"$ crofty theme eject --print",
		access.AgentRule,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

// TestPrintDenied_NoChoices: a wall nobody described yet still says crofty
// stopped on purpose, rather than reading as a crash.
func TestPrintDenied_NoChoices(t *testing.T) {
	var buf bytes.Buffer
	printDenied(&buf, &access.Denied{Op: "mkdir", Path: "/etc/nope", Err: fs.ErrPermission})
	out := buf.String()

	if !strings.Contains(out, "stopped here rather than work around it") {
		t.Errorf("a choiceless wall must still refuse to improvise:\n%s", out)
	}
	if !strings.Contains(out, access.AgentRule) {
		t.Error("the norm must hold even with no choices")
	}
}

// TestPrintDeniedJSON_MatchesTheHumanText: the two renderings come from one
// value, so --json carries the same fields an author would read.
func TestPrintDeniedJSON_MatchesTheHumanText(t *testing.T) {
	var buf bytes.Buffer
	printDeniedJSON(&buf, deniedFixture())

	var p access.Payload
	if err := json.Unmarshal(buf.Bytes(), &p); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if p.Error != "permission_denied" || p.Path != "/site/assets/css/custom.css" {
		t.Errorf("payload = %+v", p)
	}
	if len(p.Choices) != 2 || !p.Choices[0].NeedsPermission || p.Choices[1].NeedsPermission {
		t.Errorf("choices did not survive the round trip: %+v", p.Choices)
	}
	if p.AgentRule != access.AgentRule {
		t.Error("--json must carry the norm too — an agent only reads this one")
	}
}

func TestWantsJSON(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"eject", "--json"}, true},
		{[]string{"-json"}, true},
		{[]string{"eject", "--print"}, false},
		{nil, false},
	} {
		if got := wantsJSON(tc.args); got != tc.want {
			t.Errorf("wantsJSON(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
