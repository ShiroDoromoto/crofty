package cli

import "fmt"

// The machine layer crofty will own — stubbed so the surface is visible and
// scriptable now, implemented in later milestones. They are deliberately not
// errors: invoking one prints what it will do and exits cleanly.

func runEject(args []string) error {
	return notYet("eject", "a later milestone",
		"detaches the project from crofty entirely, leaving a plain Hugo site you fully own "+
			"(to edit the theme in place today, use 'crofty theme eject --full')")
}

func notYet(name, when, what string) error {
	fmt.Printf("crofty %s is not implemented yet (planned for %s).\n", name, when)
	fmt.Printf("When it lands it %s.\n", what)
	return nil
}
