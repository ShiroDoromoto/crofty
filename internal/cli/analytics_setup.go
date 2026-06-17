package cli

import (
	"errors"
	"fmt"
	"os"

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
	fmt.Fprintln(w, "\nFind it: GA4 → Admin → Property settings → \"Property ID\" (a number like 541957758;")
	fmt.Fprintln(w, "this is NOT the G-XXXXXXX measurement tag). Then add it under params.crofty.analytics")
	fmt.Fprintln(w, "in hugo.yaml (crofty doesn't edit hugo.yaml for you):")
	fmt.Fprintln(w, "\n    params:")
	fmt.Fprintln(w, "      crofty:")
	fmt.Fprintln(w, "        analytics:")
	fmt.Fprintln(w, `          ga4_property: "541957758"`)
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

// guidanceForAPIError is steps 3 and 4: the call reached Google but came back
// 403. Either the API isn't enabled (turn it on) or the service account isn't a
// member of the property (grant it access) — and crofty knows the project and
// the SA email from the loaded key, so it fills them in.
func guidanceForAPIError(err error, c *google.Client, source string) error {
	var ae *google.APIError
	if !errors.As(err, &ae) {
		return err
	}
	w := os.Stderr
	switch {
	case ae.Disabled():
		fmt.Fprintf(w, "The %s API isn't enabled in your project yet.\n", sourceLabel(source))
		fmt.Fprintf(w, "\n  Enable it: %s\n", enableAPIURL(apiHost(source), c.ProjectID()))
		fmt.Fprintln(w, "  (it can take a minute to take effect, then re-run this command)")
	case ae.Forbidden():
		fmt.Fprintf(w, "The service account can't access this %s property yet.\n", sourceLabel(source))
		fmt.Fprintf(w, "\n  Add this email as a viewer:\n    %s\n", c.Email())
		if source == "search" {
			fmt.Fprintf(w, "\n  Search Console → Settings → Users and permissions: %s\n", gscUsersURL)
		} else {
			fmt.Fprintf(w, "\n  GA4 → Admin → Property access management → add as Viewer: %s\n", ga4AdminURL)
		}
	default:
		fmt.Fprintf(w, "%s API error: %s\n", sourceLabel(source), ae.Error())
	}
	return errSilent
}

func sourceLabel(source string) string {
	if source == "search" {
		return "Search Console"
	}
	return "GA4"
}
