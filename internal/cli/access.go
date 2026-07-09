package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// deniedWalls pulls every permission wall out of err: the several a preflight
// found, or the one any other call site hit — including one nobody wrapped,
// which access.From promotes. The router asks this once, for every command.
func deniedWalls(err error) (access.Denials, bool) {
	var ds access.Denials
	if errors.As(err, &ds) {
		return ds, true
	}
	if d, ok := access.From(err); ok {
		return access.Denials{d}, true
	}
	return nil, false
}

// printDenied renders a permission wall as a fork the author chooses. It never
// suggests a way around the wall that crofty could take on its own, and it says
// so out loud — because the reader is usually an AI, and the failure this guards
// against is that AI helpfully rewriting the author's environment (D-1).
func printDenied(w io.Writer, d *access.Denied) {
	fmt.Fprintln(w, "\ncrofty needs your permission.")
	printWall(w, d.Payload())
	fmt.Fprintf(w, "\nIf you are an AI running crofty for someone: %s\n", access.AgentRule)
}

// printDenials renders every wall a command found before starting, under one
// header and one rule. Asking for two permissions at once is the whole reason
// init looks ahead: the author grants what is needed and runs the command once.
func printDenials(w io.Writer, ds access.Denials) {
	if len(ds) == 1 {
		printDenied(w, ds[0])
		return
	}
	fmt.Fprintf(w, "\ncrofty needs your permission — %d walls, before it starts.\n", len(ds))
	for _, d := range ds {
		printWall(w, d.Payload())
	}
	fmt.Fprintf(w, "\nIf you are an AI running crofty for someone: %s\n", access.AgentRule)
}

// printWall is one wall: what crofty tried, where, what it was told, and the
// ways on. It renders the same value --json emits, so the two cannot drift —
// which is why `crofty config` can show a wall it did not fail on.
func printWall(w io.Writer, p access.Payload) {
	fmt.Fprintf(w, "\n  it tried to:  %s\n", p.Op)
	if p.Path != "" {
		fmt.Fprintf(w, "  the path:     %s\n", p.Path)
	}
	fmt.Fprintf(w, "  it was told:  %s\n", p.Reason)

	if len(p.Choices) == 0 {
		fmt.Fprintln(w, "\ncrofty stopped here rather than work around it.")
	} else {
		fmt.Fprintln(w, "\nHow to go on — crofty won't choose this for you:")
		for i, c := range p.Choices {
			fmt.Fprintf(w, "\n  %d. %s\n", i+1, c.Do)
			if c.Permission != "" {
				fmt.Fprintf(w, "     needs your permission: %s\n", c.Permission)
			}
			if c.Command != "" {
				fmt.Fprintf(w, "     $ %s\n", c.Command)
			}
		}
	}
}

// printDeniedJSON emits the same value as printDenied, for the commands an agent
// drives with --json.
func printDeniedJSON(w io.Writer, d *access.Denied) {
	printDenialsJSON(w, access.Denials{d})
}

// printDenialsJSON is the wire form of one wall or several: always a list, so an
// agent parsing it never has to handle two shapes.
func printDenialsJSON(w io.Writer, ds access.Denials) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(ds.Report())
}

// wantsJSON reports whether the command line asked for machine-readable output.
// The router has to answer this without knowing which command ran, so it reads
// the raw args — every crofty command spells the flag the same way.
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" {
			return true
		}
	}
	return false
}
