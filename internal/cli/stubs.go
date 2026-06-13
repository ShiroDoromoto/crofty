package cli

import "fmt"

// The machine layer crofty will own — stubbed so the surface is visible and
// scriptable now, implemented in later milestones. They are deliberately not
// errors: invoking one prints what it will do and exits cleanly.

func runValidate(args []string) error {
	return notYet("validate", "M2",
		"checks your Markdown against the crofty spec and prints neutral, source-agnostic fix guidance")
}

func runPublish(args []string) error {
	return notYet("publish", "M3",
		"syndicates title+description fragments to platforms with a canonical link back to your own site (it never sends the body)")
}

func runEject(args []string) error {
	return notYet("eject", "a later milestone",
		"writes the bundled theme into ./themes so what remains is a plain Hugo project you fully own")
}

func notYet(name, when, what string) error {
	fmt.Printf("crofty %s is not implemented yet (planned for %s).\n", name, when)
	fmt.Printf("When it lands it %s.\n", what)
	return nil
}
