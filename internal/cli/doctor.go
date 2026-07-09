package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ShiroDoromoto/crofty/internal/contract"
)

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit structured JSON (for tools)")
	fs.Usage = func() {
		fmt.Println("crofty doctor — check the built site against the crofty output contract")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty doctor         # checks ./dist (run 'crofty build' first)")
		fmt.Println("\nFlags:")
		fs.PrintDefaults()
	}
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	report, err := contract.Check(proj.DistDir())
	if err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		renderContract(report)
	}
	if report.HasError() {
		return errSilent // findings already shown; non-zero exit gates build/deploy
	}
	return nil
}

// renderContract prints a full human report plus the honest note about what the
// contract does not yet enforce (no silent caps — 08 §9).
func renderContract(r contract.Report) {
	if len(r.Findings) == 0 {
		fmt.Println("✓ output contract: all good")
	} else {
		errs, warns := renderContractFindings(r)
		fmt.Println()
		fmt.Printf("%s, %s\n", countLabel(errs, "error"), countLabel(warns, "warning"))
	}
	fmt.Println()
	fmt.Println("Checks: canonical link, feed, <html lang>/<title>/viewport, no phone-home.")
	fmt.Println("        Not yet enforced: deep static-body checks.")
}

// renderContractFindings prints the findings list and returns the error/warn
// counts. Shared by `doctor` and the deploy gate.
func renderContractFindings(r contract.Report) (errs, warns int) {
	for _, f := range r.Findings {
		mark := "·"
		switch f.Severity {
		case contract.SeverityError:
			mark, errs = "✗", errs+1
		case contract.SeverityWarn:
			mark, warns = "⚠", warns+1
		}
		where := f.File
		if where == "" {
			where = "(site)"
		}
		fmt.Printf("  %s %-5s %s [%s] — %s\n", mark, string(f.Severity), where, f.Check, f.Message)
		if f.Fix != "" {
			fmt.Printf("      ↳ %s\n", f.Fix)
		}
	}
	return errs, warns
}

// contractGate runs the output contract before a deploy. An Error blocks (the
// site and Markdown are untouched); Warnings are shown but don't block.
func contractGate(distDir string) error {
	report, err := contract.Check(distDir)
	if err != nil {
		return err
	}
	if report.HasError() {
		fmt.Println("✗ the built site is missing something crofty guarantees — not deploying:")
		fmt.Println()
		renderContractFindings(report)
		fmt.Println()
		fmt.Println("Fix these and rebuild, or run 'crofty doctor' for the full report. Nothing was deployed.")
		return errSilent
	}
	if len(report.Findings) > 0 {
		renderContractFindings(report) // warnings only — advisory, not blocking
		fmt.Println()
	}
	return nil
}

// contractNotice runs the contract after a build and surfaces any findings,
// without ever failing the build (the build output is still written; deploy is
// where Errors actually gate).
func contractNotice(distDir string) {
	report, err := contract.Check(distDir)
	if err != nil || len(report.Findings) == 0 {
		return
	}
	fmt.Println()
	errs, _ := renderContractFindings(report)
	if errs > 0 {
		fmt.Println("  (these block deploy — run 'crofty doctor' for details)")
	}
}
