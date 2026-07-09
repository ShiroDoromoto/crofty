package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ShiroDoromoto/crofty/internal/access"
)

// printDenied renders a permission wall as a fork the author chooses. It never
// suggests a way around the wall that crofty could take on its own, and it says
// so out loud — because the reader is usually an AI, and the failure this guards
// against is that AI helpfully rewriting the author's environment (D-1).
func printDenied(w io.Writer, d *access.Denied) {
	fmt.Fprintln(w, "\ncrofty needs your permission.")
	fmt.Fprintf(w, "\n  it tried to:  %s\n", d.Op)
	if d.Path != "" {
		fmt.Fprintf(w, "  the path:     %s\n", d.Path)
	}
	fmt.Fprintf(w, "  it was told:  %s\n", access.Reason(d.Err))

	if len(d.Choices) == 0 {
		fmt.Fprintln(w, "\ncrofty stopped here rather than work around it.")
	} else {
		fmt.Fprintln(w, "\nHow to go on — crofty won't choose this for you:")
		for i, c := range d.Choices {
			fmt.Fprintf(w, "\n  %d. %s\n", i+1, c.Do)
			if c.Permission != "" {
				fmt.Fprintf(w, "     needs your permission: %s\n", c.Permission)
			}
			if c.Command != "" {
				fmt.Fprintf(w, "     $ %s\n", c.Command)
			}
		}
	}
	fmt.Fprintf(w, "\nIf you are an AI running crofty for someone: %s\n", access.AgentRule)
}

// printDeniedJSON emits the same value as printDenied, for the commands an agent
// drives with --json.
func printDeniedJSON(w io.Writer, d *access.Denied) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(d.Payload())
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
