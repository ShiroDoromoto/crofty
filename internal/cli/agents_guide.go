package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AGENTS.md is the file an AI coding assistant reads first when it lands in a
// directory. A crofty site looks like a plain Hugo project from the outside
// (hugo.yaml, content/, a theme), so an assistant dropped into the folder will
// happily start editing layouts and running `hugo` by hand — exactly what
// crofty owns and would overwrite. This file is the funnel: it does one thing,
// point the assistant at `crofty agent`, which is always current with the
// installed version. We deliberately keep no command list here — that drifts;
// the redirect does not. The one rule that does live here is the one an
// assistant needs before it has run anything: when crofty asks for permission,
// ask the author rather than working around it (D-1).
const (
	agentsFileName  = "AGENTS.md"
	agentsBeginMark = "<!-- crofty:begin (managed block — edit outside it) -->"
	agentsEndMark   = "<!-- crofty:end -->"
)

// guideStatus is the outcome of reconciling root/AGENTS.md.
type guideStatus int

const (
	guideCreated guideStatus = iota // we wrote a fresh AGENTS.md (none existed)
	guidePresent                    // a crofty-managed block is already there
	guideForeign                    // an author's own AGENTS.md exists, untouched
)

// agentsBlock is the crofty-managed block, fenced by markers so we can find it
// again (idempotency / detection) without re-parsing the prose.
func agentsBlock() string {
	return agentsBeginMark + "\n" +
		"# This site is managed by crofty\n\n" +
		"This is a crofty site (https://crofty.site), not a plain Hugo project.\n\n" +
		"**Before doing anything, run `crofty agent`.** It tells you everything:\n" +
		"the commands, the workflow, and how to work with this site.\n\n" +
		"If crofty says it needs the author's permission, stop and ask them.\n" +
		"It will print the choices. Don't work around it — don't change environment\n" +
		"variables, move files, or elevate privileges to get past it.\n" +
		agentsEndMark + "\n"
}

// ensureAgentsGuide funnels an assistant to `crofty agent` via root/AGENTS.md,
// but only ever by CREATING the file — it never edits an author's own AGENTS.md.
//   - no file         → write one holding just the managed block (guideCreated);
//   - file w/ block    → nothing to do (guidePresent);
//   - file w/o block   → it's the author's; leave it untouched (guideForeign).
//
// The caller decides what to say about a guideForeign result (we advise, we
// don't mutate). Called from both `init` and `build` (the latter backfills
// sites made before this existed — all of which have no AGENTS.md, so they hit
// the guideCreated path cleanly).
func ensureAgentsGuide(root string) (guideStatus, error) {
	path := filepath.Join(root, agentsFileName)
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(agentsBlock()), 0o644); err != nil {
			return guideCreated, err
		}
		return guideCreated, nil
	}
	if err != nil {
		return guideForeign, err
	}
	if strings.Contains(string(existing), agentsBeginMark) {
		return guidePresent, nil
	}
	return guideForeign, nil
}

// agentsForeignAdvice is the one-line note shown when an author already keeps
// their own AGENTS.md: crofty won't touch it, so we just say what to add.
const agentsForeignAdvice = "note: your AGENTS.md is yours — crofty left it alone. " +
	"Consider adding a line telling assistants to run `crofty agent` first."

// noteAgentsGuide reconciles AGENTS.md and prints a one-line message only when
// there is something to say. Errors are non-fatal: the guide is a convenience,
// never a reason to fail a build.
func noteAgentsGuide(root string) {
	status, err := ensureAgentsGuide(root)
	if err != nil {
		return
	}
	switch status {
	case guideCreated:
		fmt.Printf("  wrote %s — tells any AI assistant to run `crofty agent` first\n", agentsFileName)
	case guideForeign:
		fmt.Println("  " + agentsForeignAdvice)
	}
}
