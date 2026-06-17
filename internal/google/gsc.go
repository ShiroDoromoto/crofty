package google

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// gscMetrics are the four numbers Search Console returns for every query —
// always all of them, so they're the trailing headers on every search report.
var gscMetrics = []string{"clicks", "impressions", "ctr", "position"}

// GSCPresets map a name to the dimension(s) to group by. The metrics above come
// back regardless. Mirrors the Python gsc_report.py.
var GSCPresets = map[string][]string{
	"queries":   {"query"},
	"pages":     {"page"},
	"countries": {"country"},
	"devices":   {"device"},
	"overview":  {"date"},
}

// GSCQuery is one searchAnalytics.query call.
type GSCQuery struct {
	Dimensions []string
	Start      string // resolved to YYYY-MM-DD before sending
	End        string
	Limit      int
}

// SearchAnalytics runs a Search Console searchAnalytics.query and flattens it
// into a Report (dimensions + clicks/impressions/ctr/position).
func (c *Client) SearchAnalytics(site string, q GSCQuery) (*Report, error) {
	start, end := ResolveDate(q.Start), ResolveDate(q.End)
	body, err := json.Marshal(map[string]any{
		"startDate":  start,
		"endDate":    end,
		"dimensions": q.Dimensions,
		"rowLimit":   q.Limit,
	})
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/webmasters/v3/sites/%s/searchAnalytics/query", gscBase, encodeSite(site))
	raw, err := c.do(ScopeSearchRead, "POST", u, body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Rows []struct {
			Keys        []string `json:"keys"`
			Clicks      float64  `json:"clicks"`
			Impressions float64  `json:"impressions"`
			CTR         float64  `json:"ctr"`
			Position    float64  `json:"position"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("could not parse the Search Console response: %w", err)
	}

	headers := append(append([]string{}, q.Dimensions...), gscMetrics...)
	rep := &Report{
		Site:      site,
		DateRange: DateRange{Start: start, End: end},
		Headers:   headers,
	}
	for _, r := range resp.Rows {
		row := map[string]string{}
		for i, k := range r.Keys {
			if i < len(q.Dimensions) {
				row[q.Dimensions[i]] = k
			}
		}
		row["clicks"] = strconv.Itoa(int(r.Clicks))
		row["impressions"] = strconv.Itoa(int(r.Impressions))
		row["ctr"] = fmt.Sprintf("%.1f%%", r.CTR*100)
		row["position"] = fmt.Sprintf("%.1f", r.Position)
		rep.Rows = append(rep.Rows, row)
	}
	rep.RowCount = len(rep.Rows)
	return rep, nil
}

// GSCSite is one property a key can reach, with the access level it has.
type GSCSite struct {
	SiteURL    string `json:"siteUrl"`
	Permission string `json:"permissionLevel"`
}

// Sites lists the Search Console properties the service account can reach — the
// quickest way to confirm the key is wired up and see the exact property string.
func (c *Client) Sites() ([]GSCSite, error) {
	raw, err := c.do(ScopeSearchRead, "GET", gscBase+"/webmasters/v3/sites", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		SiteEntry []GSCSite `json:"siteEntry"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.SiteEntry, nil
}

// Sitemap is one submitted sitemap and its processing status.
type Sitemap struct {
	Path           string `json:"path"`
	LastSubmitted  string `json:"lastSubmitted"`
	LastDownloaded string `json:"lastDownloaded"`
	IsPending      bool   `json:"isPending"`
	Errors         string `json:"errors"`
	Warnings       string `json:"warnings"`
}

// Sitemaps lists the sitemaps submitted for a property and their status.
func (c *Client) Sitemaps(site string) ([]Sitemap, error) {
	u := fmt.Sprintf("%s/webmasters/v3/sites/%s/sitemaps", gscBase, encodeSite(site))
	raw, err := c.do(ScopeSearchRead, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Sitemap []Sitemap `json:"sitemap"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Sitemap, nil
}

// SubmitSitemap (re)submits a sitemap URL for a property. This is the one write
// operation, so it asks for the write scope; Google downloads it asynchronously.
func (c *Client) SubmitSitemap(site, feedpath string) error {
	u := fmt.Sprintf("%s/webmasters/v3/sites/%s/sitemaps/%s", gscBase, encodeSite(site), url.QueryEscape(feedpath))
	_, err := c.do(ScopeSearchWrite, "PUT", u, nil)
	return err
}

// encodeSite percent-encodes a Search Console property for use as a path segment.
// Properties look like "sc-domain:example.com" or "https://example.com/" — both
// the colon and the slashes must be encoded, which QueryEscape does (site URLs
// have no spaces, so the "+"-for-space quirk never bites).
func encodeSite(site string) string {
	return url.QueryEscape(site)
}

// ResolveDate turns a date spec into the YYYY-MM-DD form Search Console wants.
// It accepts "today", "NdaysAgo", or an already-ISO date (passed through).
func ResolveDate(spec string) string {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "" || spec == "today":
		if spec == "" {
			return spec
		}
		return nowFunc().Format("2006-01-02")
	case strings.HasSuffix(spec, "daysAgo"):
		n, err := strconv.Atoi(strings.TrimSuffix(spec, "daysAgo"))
		if err != nil {
			return spec
		}
		return nowFunc().AddDate(0, 0, -n).Format("2006-01-02")
	default:
		return spec // assume YYYY-MM-DD
	}
}
