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
// error. Either the API isn't enabled (turn it on), the configured property id
// isn't one the key can see (fix the id), or the service account isn't a member
// (grant it access) — and crofty knows the project and the SA email from the
// loaded key, so it fills them in. target is the configured property/site the
// failing call used (for context and for the wrong-id check); asJSON emits a
// structured failure an agent can branch on instead of scraping prose.
//
// Whatever the cause, Google's own message is carried through (never swallowed):
// it's the one place "User does not have sufficient permissions ... see
// property-id" reaches the author.
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
		// Distinguish "wrong destination" from "no access": Google returns the same
		// PERMISSION_DENIED for a non-existent id and an unauthorized one, so ask the
		// Admin API what this key can actually reach and compare. Only when the list
		// call succeeds and the configured id is genuinely absent — otherwise fall
		// through to the access guidance (a failed list, e.g. Admin API not enabled,
		// must not mask a real permission problem).
		if source == "ga4" && target != "" {
			if props, lerr := c.AccountSummaries(); lerr == nil && len(props) > 0 && !containsProperty(props, target) {
				return ga4WrongPropertyGuidance(ae, c.Email(), target, props, asJSON)
			}
		}
		return accessGuidance(ae, c.Email(), source, target, asJSON)
	default:
		return emitAPIError(asJSON, apiErrorBody{
			Source:      source,
			Kind:        "apiError",
			GoogleError: googleErrorOf(ae),
		}, []string{fmt.Sprintf("%s API error: %s", sourceLabel(source), ae.Error())}, ae)
	}
}

// ga4WrongPropertyGuidance is the case the 403 plumbing exists for: the
// configured ga4_property is not one the service account can see. Adding a viewer
// won't fix it — the id is wrong. So crofty lists the ids the key actually reaches
// and points at hugo.yaml.
func ga4WrongPropertyGuidance(ae *google.APIError, email, target string, props []google.GA4Property, asJSON bool) error {
	lines := []string{
		fmt.Sprintf("The configured GA4 property %s isn't one this service account can see.", target),
		fmt.Sprintf("This service account (%s) currently has access to:", email),
	}
	for _, p := range props {
		lines = append(lines, fmt.Sprintf("    %s  %s", p.ID, p.Name))
	}
	lines = append(lines,
		"",
		"Update params.crofty.analytics.ga4_property in hugo.yaml to the right id",
		fmt.Sprintf("(crofty doesn't edit hugo.yaml for you), or grant the SA access to %s", target),
		"in GA4 → Admin → Property access management.",
	)
	return emitAPIError(asJSON, apiErrorBody{
		Source:              "ga4",
		Kind:                "wrongProperty",
		GoogleError:         googleErrorOf(ae),
		ConfiguredProperty:  target,
		AvailableProperties: props,
		Next:                "set ga4_property in hugo.yaml to one of availableProperties",
	}, lines, ae)
}

// accessGuidance is the genuine "not a member yet (or role still propagating)"
// 403 — the one the viewer instruction actually fixes.
func accessGuidance(ae *google.APIError, email, source, target string, asJSON bool) error {
	lines := []string{
		fmt.Sprintf("The service account can't access this %s property yet.", sourceLabel(source)),
		"",
		"  Add this email as a viewer:",
		"    " + email,
	}
	if source == "search" {
		lines = append(lines, "", "  Search Console → Settings → Users and permissions: "+gscUsersURL)
	} else {
		lines = append(lines, "", "  GA4 → Admin → Property access management → add as Viewer: "+ga4AdminURL)
	}
	return emitAPIError(asJSON, apiErrorBody{
		Source:             source,
		Kind:               "forbidden",
		GoogleError:        googleErrorOf(ae),
		ConfiguredProperty: target,
		Next:               "grant the service account viewer access, then wait a minute and re-run",
	}, lines, ae)
}

// apiErrorBody is the structured failure --json mode emits so an agent can branch
// on the real cause (kind) and read Google's own envelope (googleError) instead
// of scraping prose. It goes to stdout — so --json always yields parseable stdout,
// success or failure — and the command still exits non-zero.
type apiErrorBody struct {
	Source              string               `json:"source"`
	Kind                string               `json:"kind"` // disabled|wrongProperty|forbidden|apiError
	GoogleError         googleErrorJSON      `json:"googleError"`
	ConfiguredProperty  string               `json:"configuredProperty,omitempty"`
	AvailableProperties []google.GA4Property `json:"availableProperties,omitempty"`
	Next                string               `json:"next,omitempty"`
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

// containsProperty reports whether id is among the reachable GA4 properties.
func containsProperty(props []google.GA4Property, id string) bool {
	for _, p := range props {
		if p.ID == id {
			return true
		}
	}
	return false
}

func sourceLabel(source string) string {
	if source == "search" {
		return "Search Console"
	}
	return "GA4"
}
