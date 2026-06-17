package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ShiroDoromoto/crofty/internal/google"
	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/secret"
)

// runAnalytics reads the author's own traffic: GA4 (who visited, what they read)
// and Search Console (which queries surfaced the site, what's indexed). It closes
// the loop on the analytics tag crofty already helps emit — your data, read with
// your own tool, as neutral JSON your agent can parse, no SaaS dashboard.
//
// The two sources are different Google APIs, so the source is named explicitly:
// `crofty analytics ga4 <report>` and `crofty analytics search <report>`.
func runAnalytics(args []string) error {
	if len(args) == 0 {
		analyticsUsage()
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		analyticsUsage()
		return nil
	case "ga4":
		return runAnalyticsGA4(args[1:])
	case "search":
		return runAnalyticsSearch(args[1:])
	case "connect":
		return runAnalyticsConnect(args[1:])
	case "status":
		return runAnalyticsStatus(args[1:])
	default:
		return fmt.Errorf("unknown analytics subcommand %q (try: crofty analytics ga4 | search | connect | status)", args[0])
	}
}

func analyticsUsage() {
	fmt.Println("crofty analytics — read your own traffic (GA4) and search performance (Search Console)")
	fmt.Println("\nUsage:")
	fmt.Println("  crofty analytics ga4 <report>      who visited, what they read")
	fmt.Println("    reports: " + strings.Join(sortedPresetNames(ga4PresetNames()), ", "))
	fmt.Println("    custom:  crofty analytics ga4 --metrics <m,m> --dimensions <d,d> [--order-by <m>]")
	fmt.Println("  crofty analytics search <report>   how Google sees the site")
	fmt.Println("    reports: " + strings.Join(sortedPresetNames(gscPresetNames()), ", ") + ", sites, sitemaps, submit-sitemap")
	fmt.Println("  crofty analytics connect --key <sa.json>   load a service-account key (once)")
	fmt.Println("  crofty analytics status [--json]           what's set up and the next step")
	fmt.Println("\nCommon flags: --start 28daysAgo  --end today  --limit 25  --json")
	fmt.Println("Run 'crofty analytics status' if a command says it isn't set up yet.")
}

// --- ga4 ------------------------------------------------------------------

func runAnalyticsGA4(args []string) error {
	fs := flag.NewFlagSet("analytics ga4", flag.ContinueOnError)
	start := fs.String("start", "28daysAgo", "start date (YYYY-MM-DD or NdaysAgo)")
	end := fs.String("end", "today", "end date (YYYY-MM-DD or 'today')")
	limit := fs.Int("limit", 25, "max rows")
	metrics := fs.String("metrics", "", "comma-separated metrics (raw query; overrides the preset)")
	dimensions := fs.String("dimensions", "", "comma-separated dimensions (raw query; overrides the preset)")
	orderBy := fs.String("order-by", "", "metric or dimension to sort by, descending")
	property := fs.String("property", "", "GA4 numeric property id (overrides hugo.yaml)")
	key := fs.String("key", "", "path to a service-account JSON key (overrides the saved one)")
	asJSON := fs.Bool("json", false, "emit the report as JSON (for tools/agents)")
	fs.Usage = analyticsUsage
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}

	var q google.GA4Query
	q.Start, q.End, q.Limit = *start, *end, *limit
	if len(rest) > 0 {
		preset, ok := google.GA4Presets[rest[0]]
		if !ok {
			return fmt.Errorf("unknown ga4 report %q (try: %s, or a custom --metrics query)",
				rest[0], strings.Join(sortedPresetNames(ga4PresetNames()), ", "))
		}
		q.Metrics, q.Dimensions, q.OrderBy = preset.Metrics, preset.Dimensions, preset.OrderBy
	}
	if *metrics != "" {
		q.Metrics = splitComma(*metrics)
	}
	if *dimensions != "" {
		q.Dimensions = splitComma(*dimensions)
	}
	if *orderBy != "" {
		q.OrderBy = *orderBy
	} else if q.OrderBy == "" && len(q.Metrics) > 0 {
		q.OrderBy = q.Metrics[0]
	}
	if len(q.Metrics) == 0 {
		return fmt.Errorf("pick a report (e.g. 'crofty analytics ga4 top-pages') or pass --metrics")
	}

	propertyID := *property
	if propertyID == "" {
		propertyID, _ = analyticsTargets(proj.Root)
	}
	if propertyID == "" {
		printSetupPropertyMissing("ga4")
		return errSilent
	}

	client, err := loadGoogleClient(proj, *key)
	if err != nil {
		return err
	}
	rep, err := client.RunReport(propertyID, q)
	if err != nil {
		return guidanceForAPIError(err, client, "ga4")
	}
	return emitReport(rep, *asJSON, "GA4 has no data for this window yet (new property, or it lags a little)")
}

// --- search ---------------------------------------------------------------

func runAnalyticsSearch(args []string) error {
	fs := flag.NewFlagSet("analytics search", flag.ContinueOnError)
	start := fs.String("start", "28daysAgo", "start date (YYYY-MM-DD or NdaysAgo)")
	end := fs.String("end", "today", "end date (YYYY-MM-DD or 'today')")
	limit := fs.Int("limit", 25, "max rows")
	site := fs.String("site", "", "Search Console property (overrides hugo.yaml)")
	sitemap := fs.String("sitemap", "", "sitemap URL for submit-sitemap (default: derived from the property)")
	key := fs.String("key", "", "path to a service-account JSON key (overrides the saved one)")
	asJSON := fs.Bool("json", false, "emit the report as JSON (for tools/agents)")
	fs.Usage = analyticsUsage
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	cmd := "queries"
	if len(rest) > 0 {
		cmd = rest[0]
	}

	siteURL := *site
	if siteURL == "" {
		_, siteURL = analyticsTargets(proj.Root)
	}
	// `sites` is the one report that doesn't need a configured property — it's how
	// you discover which properties the key can reach.
	if siteURL == "" && cmd != "sites" {
		printSetupPropertyMissing("search")
		return errSilent
	}

	client, err := loadGoogleClient(proj, *key)
	if err != nil {
		return err
	}

	switch cmd {
	case "sites":
		sites, err := client.Sites()
		if err != nil {
			return guidanceForAPIError(err, client, "search")
		}
		return emitSites(sites, *asJSON)
	case "sitemaps":
		sms, err := client.Sitemaps(siteURL)
		if err != nil {
			return guidanceForAPIError(err, client, "search")
		}
		return emitSitemaps(sms, siteURL, *asJSON)
	case "submit-sitemap":
		feed := *sitemap
		if feed == "" {
			feed = defaultSitemapURL(siteURL)
		}
		if err := client.SubmitSitemap(siteURL, feed); err != nil {
			return guidanceForAPIError(err, client, "search")
		}
		fmt.Printf("submitted %s to %s\n", feed, siteURL)
		fmt.Println("Google downloads it asynchronously — re-run 'crofty analytics search sitemaps' for status.")
		return nil
	}

	dims, ok := google.GSCPresets[cmd]
	if !ok {
		return fmt.Errorf("unknown search report %q (try: %s, sites, sitemaps, submit-sitemap)",
			cmd, strings.Join(sortedPresetNames(gscPresetNames()), ", "))
	}
	rep, err := client.SearchAnalytics(siteURL, google.GSCQuery{Dimensions: dims, Start: *start, End: *end, Limit: *limit})
	if err != nil {
		return guidanceForAPIError(err, client, "search")
	}
	return emitReport(rep, *asJSON, "no search data yet — normal for a new/low-traffic property (GSC also lags ~2-3 days)")
}

// --- connect --------------------------------------------------------------

func runAnalyticsConnect(args []string) error {
	fs := flag.NewFlagSet("analytics connect", flag.ContinueOnError)
	key := fs.String("key", "", "path to the service-account JSON key to load")
	fs.Usage = analyticsUsage
	if err := fs.Parse(args); err != nil {
		return err
	}
	proj, err := findProject()
	if err != nil {
		return err
	}
	if *key == "" {
		printSetupKeyMissing()
		return errSilent
	}
	raw, err := os.ReadFile(*key)
	if err != nil {
		return fmt.Errorf("reading --key %s: %w", *key, err)
	}
	creds, err := google.ParseCredentials(raw)
	if err != nil {
		return err
	}
	if err := analyticsStore(proj).Set("google", "sa_key", string(raw)); err != nil {
		return fmt.Errorf("saving the key to the keychain: %w", err)
	}
	fmt.Printf("Loaded the service-account key for %s into your keychain.\n", creds.ClientEmail)
	fmt.Println("You can delete the downloaded JSON file now — crofty reads it from the keychain.")
	fmt.Println("\nNext: make sure that service account can see your data —")
	fmt.Printf("  GA4    → Admin → Property access management → add %s as a Viewer\n", creds.ClientEmail)
	fmt.Printf("  Search → Settings → Users and permissions → add the same email\n")
	fmt.Println("Then: crofty analytics status")
	return nil
}

// --- status ---------------------------------------------------------------

type analyticsStatusReport struct {
	GA4Property    string `json:"ga4Property"`
	SearchConsole  string `json:"searchConsole"`
	KeyLoaded      bool   `json:"keyLoaded"`
	ServiceAccount string `json:"serviceAccount,omitempty"`
	ProjectID      string `json:"projectId,omitempty"`
	Ready          bool   `json:"ready"`
	Next           string `json:"next,omitempty"`
}

func runAnalyticsStatus(args []string) error {
	fs := flag.NewFlagSet("analytics status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit status as JSON (for tools/agents)")
	fs.Usage = analyticsUsage
	if err := fs.Parse(args); err != nil {
		return err
	}
	proj, err := findProject()
	if err != nil {
		return err
	}
	ga4Prop, scSite := analyticsTargets(proj.Root)
	st := analyticsStatusReport{GA4Property: ga4Prop, SearchConsole: scSite}
	if v, err := analyticsStore(proj).Get("google", "sa_key"); err == nil && v != "" {
		st.KeyLoaded = true
		if creds, perr := google.ParseCredentials([]byte(v)); perr == nil {
			st.ServiceAccount = creds.ClientEmail
			st.ProjectID = creds.ProjectID
		}
	}
	st.Ready = st.KeyLoaded && (ga4Prop != "" || scSite != "")
	switch {
	case !st.KeyLoaded:
		st.Next = "load a service-account key: crofty analytics connect --key <sa.json>"
	case ga4Prop == "" && scSite == "":
		st.Next = "set ga4_property and/or search_console under params.crofty.analytics in hugo.yaml"
	default:
		st.Next = "try a report, e.g. crofty analytics ga4 top-pages"
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}
	printAnalyticsStatus(st)
	return nil
}

func printAnalyticsStatus(st analyticsStatusReport) {
	fmt.Println("crofty analytics — setup status:")
	fmt.Println()
	fmt.Printf("  %s service-account key loaded\n", mark(st.KeyLoaded))
	if st.ServiceAccount != "" {
		fmt.Printf("      %s (project %s)\n", st.ServiceAccount, st.ProjectID)
	}
	fmt.Printf("  %s GA4 property        %s\n", mark(st.GA4Property != ""), orDash(st.GA4Property))
	fmt.Printf("  %s Search Console      %s\n", mark(st.SearchConsole != ""), orDash(st.SearchConsole))
	fmt.Println()
	if st.Next != "" {
		fmt.Println("Next: " + st.Next)
	}
}

// --- shared helpers -------------------------------------------------------

// analyticsTargets reads the read-side analytics config the author pastes into
// hugo.yaml: the GA4 numeric property id and the Search Console property. Same
// dig as analyticsProviders (the emission side), one layer down.
func analyticsTargets(root string) (ga4Property, searchConsole string) {
	cfg, err := loadHugoConfig(root)
	if err != nil {
		return "", ""
	}
	params, _ := cfg["params"].(map[string]any)
	crofty, _ := params["crofty"].(map[string]any)
	an, _ := crofty["analytics"].(map[string]any)
	return asString(an["ga4_property"]), asString(an["search_console"])
}

// analyticsStore namespaces the service-account key per project (its workspace
// id), so sites with different Google accounts keep separate keys (A5).
func analyticsStore(proj *project.Project) *secret.Store {
	return secret.New(analyticsWorkspace(proj))
}

func analyticsWorkspace(proj *project.Project) string {
	if cfg, err := proj.LoadConfig(); err == nil && cfg.Workspace != "" {
		return cfg.Workspace
	}
	return "analytics:" + proj.Root // stable fallback for pre-workspace projects
}

// loadGoogleClient resolves the service-account key (an explicit --key path, else
// the keychain) and returns an authenticated client. A missing key prints the
// setup step and returns errSilent.
func loadGoogleClient(proj *project.Project, keyFlag string) (*google.Client, error) {
	var raw []byte
	if keyFlag != "" {
		b, err := os.ReadFile(keyFlag)
		if err != nil {
			return nil, fmt.Errorf("reading --key %s: %w", keyFlag, err)
		}
		raw = b
	} else {
		v, err := analyticsStore(proj).Get("google", "sa_key")
		if err != nil || v == "" {
			printSetupKeyMissing()
			return nil, errSilent
		}
		raw = []byte(v)
	}
	creds, err := google.ParseCredentials(raw)
	if err != nil {
		return nil, err
	}
	return google.NewClient(creds), nil
}

// emitReport prints a GA4/GSC report as an aligned table, or JSON with --json.
// emptyNote explains a zero-row result (a new property, API lag) so it doesn't
// read as an error.
func emitReport(rep *google.Report, asJSON bool, emptyNote string) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	rows := make([][]string, 0, len(rep.Rows))
	for _, r := range rep.Rows {
		row := make([]string, len(rep.Headers))
		for i, h := range rep.Headers {
			row[i] = r[h]
		}
		rows = append(rows, row)
	}
	printTable(rep.Headers, rows)
	target := rep.Property
	if target == "" {
		target = rep.Site
	}
	fmt.Printf("\n(%d rows · %s→%s · %s)\n", len(rows), rep.DateRange.Start, rep.DateRange.End, target)
	if len(rows) == 0 && emptyNote != "" {
		fmt.Println("  " + emptyNote)
	}
	return nil
}

func emitSites(sites []google.GSCSite, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Sites []google.GSCSite `json:"sites"`
		}{sites})
	}
	rows := make([][]string, 0, len(sites))
	for _, s := range sites {
		rows = append(rows, []string{s.SiteURL, s.Permission})
	}
	printTable([]string{"siteUrl", "permission"}, rows)
	fmt.Printf("\n(%d properties this key can reach)\n", len(rows))
	if len(rows) == 0 {
		fmt.Println("  none — add the service-account email as a user in Search Console.")
	}
	return nil
}

func emitSitemaps(sms []google.Sitemap, site string, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Site     string           `json:"site"`
			Sitemaps []google.Sitemap `json:"sitemaps"`
		}{site, sms})
	}
	rows := make([][]string, 0, len(sms))
	for _, s := range sms {
		rows = append(rows, []string{s.Path, s.LastSubmitted, s.LastDownloaded, strconv.FormatBool(s.IsPending), orZero(s.Errors), orZero(s.Warnings)})
	}
	printTable([]string{"path", "submitted", "downloaded", "pending", "err", "warn"}, rows)
	fmt.Printf("\n%s (%d sitemaps)\n", site, len(rows))
	if len(rows) == 0 {
		fmt.Println("  none submitted yet — run: crofty analytics search submit-sitemap")
	}
	return nil
}

// printTable prints a left-aligned table (headers + rows), the same plain shape
// the Python scripts produced.
func printTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i := range headers {
			if i < len(r) && len(r[i]) > widths[i] {
				widths[i] = len(r[i])
			}
		}
	}
	fmt.Println(joinPadded(headers, widths))
	dashes := make([]string, len(headers))
	for i := range headers {
		dashes[i] = strings.Repeat("-", widths[i])
	}
	fmt.Println(joinPadded(dashes, widths))
	for _, r := range rows {
		cells := make([]string, len(headers))
		for i := range headers {
			if i < len(r) {
				cells[i] = r[i]
			}
		}
		fmt.Println(joinPadded(cells, widths))
	}
}

func joinPadded(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = c + strings.Repeat(" ", widths[i]-len(c))
	}
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}

// defaultSitemapURL derives https://<host>/sitemap.xml from a property string,
// covering both sc-domain:host and a URL-prefix property.
func defaultSitemapURL(site string) string {
	if strings.HasPrefix(site, "sc-domain:") {
		return "https://" + strings.TrimPrefix(site, "sc-domain:") + "/sitemap.xml"
	}
	return strings.TrimSuffix(site, "/") + "/sitemap.xml"
}

func ga4PresetNames() []string {
	out := make([]string, 0, len(google.GA4Presets))
	for k := range google.GA4Presets {
		out = append(out, k)
	}
	return out
}

func gscPresetNames() []string {
	out := make([]string, 0, len(google.GSCPresets))
	for k := range google.GSCPresets {
		out = append(out, k)
	}
	return out
}

func sortedPresetNames(in []string) []string {
	sort.Strings(in)
	return in
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// asString coerces a hugo.yaml value to a string, tolerating an unquoted numeric
// property id (YAML parses 123456789 as an int).
func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	default:
		return ""
	}
}

func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func orDash(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}
