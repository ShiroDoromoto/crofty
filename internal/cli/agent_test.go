package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// The point of `crofty agent` is that an AI reads it FIRST and learns every
// command — so the worst failure is a command that exists but isn't reflected
// here. commands() is the single source of truth; the brief is built from it,
// and agentDetails() must carry an entry for each command so its flags/examples
// aren't silently left blank. This test is the guard against that drift: adding
// (or renaming) a command fails the build until `agent` is updated to match.
func TestAgentBrief_CoversEveryCommand(t *testing.T) {
	details := agentDetails()

	known := map[string]bool{}
	for _, c := range commands() {
		known[c.name] = true
		if _, ok := details[c.name]; !ok {
			t.Errorf("command %q has no agentDetails() entry — add its flags/examples "+
				"(or an empty agentCmd{}) so `crofty agent` reflects it", c.name)
		}
	}
	for name := range details {
		if !known[name] {
			t.Errorf("agentDetails() has %q but commands() does not — remove the stale entry", name)
		}
	}

	// The assembled brief must list every command, with the same summary `crofty
	// help` shows (proving names/summaries are sourced from commands(), not a copy).
	b := agentBrief()
	if len(b.Commands) != len(commands()) {
		t.Fatalf("brief has %d commands, commands() has %d", len(b.Commands), len(commands()))
	}
	for i, c := range commands() {
		if b.Commands[i].Name != c.name || b.Commands[i].Summary != c.summary {
			t.Errorf("brief command %d = (%q, %q); want (%q, %q)",
				i, b.Commands[i].Name, b.Commands[i].Summary, c.name, c.summary)
		}
	}
}

// --json must be valid and carry every command, since an agent reads it first.
func TestAgent_JSON(t *testing.T) {
	out, err := captureStdout(t, func() error { return runAgent([]string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var got brief
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got.Commands) != len(commands()) {
		t.Errorf("json has %d commands, commands() has %d", len(got.Commands), len(commands()))
	}
}

// The human view names every command too, so nothing is dropped by the renderer.
func TestAgent_TextCoversCommands(t *testing.T) {
	out, err := captureStdout(t, func() error { return runAgent(nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range commands() {
		if !strings.Contains(out, c.name) {
			t.Errorf("agent text is missing command %q", c.name)
		}
	}
}

// The Functions constraint has to be readable before deploy runs, or the AI
// driving crofty only learns it by hitting the gate. Both the flag and the rule
// behind it must show up — a flag with no explanation invites --static-only as a
// reflex, which is exactly the silent breakage the gate exists to stop.
func TestAgentBrief_TeachesTheFunctionsConstraint(t *testing.T) {
	b := agentBrief()

	var deploy agentCmd
	for _, c := range b.Commands {
		if c.Name == "deploy" {
			deploy = c
		}
	}
	var hasFlag bool
	for _, f := range deploy.Flags {
		if f.Name == "--static-only" {
			hasFlag = true
		}
	}
	if !hasFlag {
		t.Error("deploy is missing the --static-only flag")
	}

	var note string
	for _, n := range b.Notes {
		if strings.Contains(n, "Pages Functions") {
			note = n
		}
	}
	if note == "" {
		t.Fatal("notes never mention Pages Functions")
	}
	for _, want := range []string{"pagesFunctions", "--static-only", "author"} {
		if !strings.Contains(note, want) {
			t.Errorf("the Functions note never mentions %q — an agent cannot act on it", want)
		}
	}
}
