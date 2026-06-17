package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// runCredit shows or sets the optional "Made with crofty" footer line. It is the
// one place to change the choice after the first deploy — the "removable, no
// penalty, reversible anytime" guarantee made concrete. The choice is baked into
// the next build, so it says to re-deploy to apply.
func runCredit(args []string) error {
	fs := flag.NewFlagSet("credit", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("crofty credit — the optional \"Made with crofty\" line in your site's footer")
		fmt.Println("\nA free, removable referral — never forced, never changes how your site works.")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty credit        # show whether the line is on, off, or undecided")
		fmt.Println("  crofty credit on     # keep the line (applied on the next deploy)")
		fmt.Println("  crofty credit off    # remove the line (applied on the next deploy)")
	}
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	cfg, err := proj.LoadConfig()
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	if len(rest) == 0 {
		printCreditStatus(cfg.FooterCredit)
		return nil
	}

	var want string
	switch strings.ToLower(rest[0]) {
	case "on", "keep":
		want = project.FooterCreditOn
	case "off", "remove":
		want = project.FooterCreditOff
	default:
		return fmt.Errorf("unknown argument %q — use 'crofty credit on' or 'crofty credit off'", rest[0])
	}

	if cfg.FooterCredit == want {
		fmt.Printf("The footer credit is already %s.\n", want)
		return nil
	}
	cfg.FooterCredit = want
	if err := proj.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	if want == project.FooterCreditOn {
		fmt.Println("✓ The footer credit is on. Thank you.")
	} else {
		fmt.Println("✓ The footer credit is off.")
	}
	fmt.Println("Run 'crofty deploy' to apply it to your live site.")
	return nil
}

func printCreditStatus(v string) {
	switch v {
	case project.FooterCreditOn:
		fmt.Println("Footer credit: on — \"via crofty\" shows in your footer.")
		fmt.Println("Remove it anytime with 'crofty credit off' (nothing else changes).")
	case project.FooterCreditOff:
		fmt.Println("Footer credit: off — no \"via crofty\" line.")
		fmt.Println("Add it anytime with 'crofty credit on'.")
	default:
		fmt.Println("Footer credit: not decided yet — crofty will ask once, on your first deploy.")
		fmt.Println("Or decide now with 'crofty credit on' / 'crofty credit off'.")
	}
}

// maybeAskFooterCredit asks once, on an interactive deploy, whether to keep the
// "Made with crofty" footer line, and saves the choice so the build that follows
// bakes it in. It is the neutral forced choice: presented with no preselect, the
// moment the author has decided to publish (value recognized), before the build
// renders the footer — so the very first published site already reflects the
// choice.
//
// It is a no-op when the choice is already made (here, a past deploy, or
// `crofty credit`), or when stdin is not a TTY. A non-interactive deploy
// (CI / an agent) is never asked: there's no one to ask, so it stays unset and
// renders off. crofty never silently defaults the line on.
func maybeAskFooterCredit(proj *project.Project, cfg *project.Config) error {
	if cfg.FooterCredit != "" {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	choice, decided := askFooterCreditChoice(os.Stdin, os.Stdout)
	if !decided {
		return nil // couldn't read a clear answer; leave it for next time
	}
	cfg.FooterCredit = choice
	if err := proj.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving footer choice: %w", err)
	}
	if choice == project.FooterCreditOn {
		fmt.Fprintln(os.Stdout, "Thank you. You can change this anytime with 'crofty credit off'.")
	} else {
		fmt.Fprintln(os.Stdout, "Removed. You can turn it back on anytime with 'crofty credit on'.")
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

// askFooterCreditChoice prints the neutral choice and reads the answer. It
// returns (value, true) once the reader gives a clear keep/remove, or
// ("", false) if the input ends without one (so the caller leaves it undecided
// rather than guessing a default). No option is preselected.
func askFooterCreditChoice(r io.Reader, w io.Writer) (string, bool) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Before this goes live — one quiet question.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "crofty can leave a single line in your site's footer:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    via crofty")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Keep it only if crofty was useful and you'd point someone here.")
	fmt.Fprintln(w, "Removing it changes nothing about how your site works — no feature is")
	fmt.Fprintln(w, "limited — and you can switch it anytime with 'crofty credit on|off'.")
	fmt.Fprintln(w)

	br := bufio.NewReader(r)
	for attempts := 0; attempts < 5; attempts++ {
		fmt.Fprint(w, "  Keep the line?  [k] keep   [r] remove : ")
		line, err := br.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "k", "keep", "y", "yes":
			return project.FooterCreditOn, true
		case "r", "remove", "n", "no":
			return project.FooterCreditOff, true
		}
		if err != nil {
			return "", false // EOF / read error: no clear answer
		}
	}
	return "", false
}
