package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ShiroDoromoto/crofty/internal/google"
)

// The one-time setup for reading your own analytics lives in Google's consoles,
// which crofty can't drive for you. So instead crofty inspects the prerequisites
// in order and, at the first gap, prints the one next step with a link — and,
// once the key is loaded, the exact service-account email and project to paste.
// All four step printers live here so `crofty analytics status` and the live
// commands' error handlers say the same thing.

const (
	gcpServiceAccountsURL = "https://console.cloud.google.com/iam-admin/serviceaccounts"
	ga4AdminURL           = "https://analytics.google.com/"
	gscUsersURL           = "https://search.google.com/search-console/users"
)

// enableAPIURL deep-links the "enable this API" page, pinned to the service
// account's own project when known.
func enableAPIURL(api, project string) string {
	u := "https://console.cloud.google.com/apis/library/" + api
	if project != "" {
		u += "?project=" + project
	}
	return u
}

// apiHost names the GCP API to enable for each source.
func apiHost(source string) string {
	if source == "search" {
		return "searchconsole.googleapis.com"
	}
	return "analyticsdata.googleapis.com"
}

// printSetupPropertyMissing is step 1: the property to read isn't in hugo.yaml.
// crofty never writes hugo.yaml, so it shows the exact lines to paste.
func printSetupPropertyMissing(source string) {
	w := os.Stderr
	if source == "search" {
		fmt.Fprintln(w, "No Search Console property set yet.")
		fmt.Fprintln(w, "\nFind it: it's the property string in Search Console (e.g. a domain property")
		fmt.Fprintln(w, "looks like sc-domain:your-site.com). Then add it under params.crofty.analytics")
		fmt.Fprintln(w, "in hugo.yaml (crofty doesn't edit hugo.yaml for you):")
		fmt.Fprintln(w, "\n    params:")
		fmt.Fprintln(w, "      crofty:")
		fmt.Fprintln(w, "        analytics:")
		fmt.Fprintln(w, `          search_console: "sc-domain:your-site.com"`)
		fmt.Fprintf(w, "\n  Open Search Console: %s\n", gscUsersURL)
		return
	}
	fmt.Fprintln(w, "No GA4 property id set yet.")
	fmt.Fprintln(w, "\nFind it: GA4 → Admin → Property settings → \"Property ID\" (a number like 123456789;")
	fmt.Fprintln(w, "this is NOT the G-XXXXXXX measurement tag). Then add it under params.crofty.analytics")
	fmt.Fprintln(w, "in hugo.yaml (crofty doesn't edit hugo.yaml for you):")
	fmt.Fprintln(w, "\n    params:")
	fmt.Fprintln(w, "      crofty:")
	fmt.Fprintln(w, "        analytics:")
	fmt.Fprintln(w, `          ga4_property: "123456789"`)
	fmt.Fprintf(w, "\n  Open GA4 admin: %s\n", ga4AdminURL)
}

// printSetupKeyMissing is step 2: there's no service-account key to authenticate
// with. It points at where to make one and how to load it.
func printSetupKeyMissing() {
	w := os.Stderr
	fmt.Fprintln(w, "No Google service-account key loaded yet.")
	fmt.Fprintln(w, "\nReading your analytics needs a service-account JSON key (a robot credential")
	fmt.Fprintln(w, "for your own data — kept in your OS keychain, never in the repo):")
	fmt.Fprintf(w, "\n  1. Create a service account + JSON key: %s\n", gcpServiceAccountsURL)
	fmt.Fprintln(w, "       → Create service account → Done → ⋮ Manage keys → Add key → JSON")
	fmt.Fprintln(w, "  2. Load it into crofty (then you can delete the downloaded file):")
	fmt.Fprintln(w, "       crofty analytics connect --key ~/Downloads/your-key.json")
}

// guidanceForAPIError is steps 3 and 4: the call reached Google but came back an
// error. crofty's job here is not to guess the single cause — Google returns the
// same 403 for a wrong/non-existent id and for missing access, on purpose — but
// to hand the caller (often the author's AI agent) everything it needs to reason
// about the cause itself: which property was tried, Google's own message, and the
// candidate causes spelled out. target is the configured property/site the failing
// call used; asJSON emits the structured form an agent can branch on.
//
// Google's own message is always carried through (never swallowed): it's the one
// place "User does not have sufficient permissions ... see property-id" reaches
// the caller.
func guidanceForAPIError(err error, c *google.Client, source, target string, asJSON bool) error {
	var ae *google.APIError
	if !errors.As(err, &ae) {
		return err
	}
	switch {
	case ae.Disabled():
		return emitAPIError(asJSON, apiErrorBody{
			Source:      source,
			Kind:        "disabled",
			GoogleError: googleErrorOf(ae),
			Next:        "enable the API: " + enableAPIURL(apiHost(source), c.ProjectID()),
		}, []string{
			fmt.Sprintf("The %s API isn't enabled in your project yet.", sourceLabel(source)),
			"",
			"  Enable it: " + enableAPIURL(apiHost(source), c.ProjectID()),
			"  (it can take a minute to take effect, then re-run this command)",
		}, ae)
	case ae.Forbidden():
		return forbiddenGuidance(ae, c.Email(), source, target, asJSON)
	default:
		return emitAPIError(asJSON, apiErrorBody{
			Source:      source,
			Kind:        "apiError",
			GoogleError: googleErrorOf(ae),
		}, []string{fmt.Sprintf("%s API error: %s", sourceLabel(source), ae.Error())}, ae)
	}
}

// forbiddenGuidance handles the 403 that isn't a disabled API. Two different
// problems land here as the identical PERMISSION_DENIED — a wrong/non-existent
// property id, and a service account that simply isn't a member — and Google won't
// say which (it hides id existence on purpose). So crofty doesn't pretend to know:
// it names both candidate causes, points at how to check each, and lets the caller
// (or its agent) decide. The wrong-id case is listed first because it's the easy
// one to overlook (the old "add a viewer" wording sent people only down the access
// path).
func forbiddenGuidance(ae *google.APIError, email, source, target string, asJSON bool) error {
	if source == "search" {
		causes := []string{
			"the configured Search Console property is wrong or doesn't exist",
			"this service account isn't a user on the property",
			"access was granted but is still propagating",
		}
		lines := []string{
			fmt.Sprintf("Search Console returned 403 for %s.", orDash(target)),
			"Google returns the same 403 whether the property is wrong and whether the",
			"service account just isn't a user, so check both:",
			"",
			fmt.Sprintf("  1. Is %s the exact property string? (a domain property looks like", orDash(target)),
			"     sc-domain:your-site.com). Fix search_console under params.crofty.analytics",
			"     in hugo.yaml if not, or run 'crofty analytics search sites' to list the",
			"     properties this key can reach.",
			"  2. Is this service account a user on it?",
			"       " + email,
			"     Search Console → Settings → Users and permissions: " + gscUsersURL,
		}
		return emitAPIError(asJSON, apiErrorBody{
			Source: source, Kind: "forbidden", GoogleError: googleErrorOf(ae),
			ConfiguredProperty: target, ServiceAccount: email, PossibleCauses: causes,
			Next: "verify the property string ('crofty analytics search sites' lists reachable ones) and that the service account is a user",
		}, lines, ae)
	}
	causes := []string{
		"the configured GA4 property id is wrong or doesn't exist",
		"this service account isn't a member (viewer) of the property",
		"access was granted but is still propagating",
	}
	lines := []string{
		fmt.Sprintf("GA4 returned 403 for property %s.", orDash(target)),
		"Google returns the same 403 whether the id is wrong and whether the service",
		"account just isn't a member, so check both:",
		"",
		fmt.Sprintf("  1. Is %s the right Property ID?", orDash(target)),
		`     GA4 → Admin → Property settings → "Property ID" (a number, not the`,
		"     G-XXXXXXX measurement tag). Fix ga4_property under params.crofty.analytics",
		"     in hugo.yaml if not.",
		"  2. Is this service account a Viewer on it?",
		"       " + email,
		"     GA4 → Admin → Property access management → add as Viewer: " + ga4AdminURL,
	}
	return emitAPIError(asJSON, apiErrorBody{
		Source: source, Kind: "forbidden", GoogleError: googleErrorOf(ae),
		ConfiguredProperty: target, ServiceAccount: email, PossibleCauses: causes,
		Next: "verify the property id (GA4 → Admin → Property settings) and that the service account is a Viewer",
	}, lines, ae)
}

// apiErrorBody is the structured failure --json mode emits so an agent can branch
// on the error class (kind), read Google's own envelope (googleError), and reason
// about the candidate causes (possibleCauses) instead of scraping prose. It goes
// to stdout — so --json always yields parseable stdout, success or failure — and
// the command still exits non-zero.
type apiErrorBody struct {
	Source             string          `json:"source"`
	Kind               string          `json:"kind"` // disabled|forbidden|apiError
	GoogleError        googleErrorJSON `json:"googleError"`
	ConfiguredProperty string          `json:"configuredProperty,omitempty"`
	ServiceAccount     string          `json:"serviceAccount,omitempty"`
	PossibleCauses     []string        `json:"possibleCauses,omitempty"`
	Next               string          `json:"next,omitempty"`
}

type googleErrorJSON struct {
	Code    int    `json:"code"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

func googleErrorOf(ae *google.APIError) googleErrorJSON {
	return googleErrorJSON{Code: ae.Code, Status: ae.Status, Message: ae.Message}
}

// emitAPIError prints the failure: structured JSON to stdout under --json, else
// the human lines to stderr with Google's raw message appended so its "check the
// property-id" hint never gets lost. Always returns errSilent (the command has
// reported; the harness exits non-zero without re-printing).
func emitAPIError(asJSON bool, body apiErrorBody, lines []string, ae *google.APIError) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(struct {
			Error apiErrorBody `json:"error"`
		}{body})
		return errSilent
	}
	w := os.Stderr
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	// The apiError branch already shows ae.Error() (== Message) in its lines;
	// every other branch substitutes crofty's own prose, so carry Google's real
	// message through there. Skip the synthetic "Google API error (HTTP n)" stand-in
	// parseAPIError uses when Google sent no message of its own.
	if body.Kind != "apiError" && ae.Message != "" && !strings.HasPrefix(ae.Message, "Google API error (HTTP") {
		fmt.Fprintf(w, "\n  Google said: %s\n", ae.Message)
	}
	return errSilent
}

func sourceLabel(source string) string {
	if source == "search" {
		return "Search Console"
	}
	return "GA4"
}
