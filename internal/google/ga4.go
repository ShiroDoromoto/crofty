package google

import (
	"encoding/json"
	"fmt"
)

// GA4Preset is a named runReport query — the common questions, so the author (or
// their agent) doesn't have to know GA4's metric/dimension vocabulary. Order is
// the headline metric, sorted descending (or "date" for a trend).
type GA4Preset struct {
	Metrics    []string
	Dimensions []string
	OrderBy    string
}

// GA4Presets are the built-in reports. They mirror the Python ga4_report.py so a
// migrating user sees the same numbers. Custom queries bypass these entirely.
var GA4Presets = map[string]GA4Preset{
	"top-pages": {
		Metrics:    []string{"screenPageViews", "activeUsers", "averageSessionDuration"},
		Dimensions: []string{"pagePath", "pageTitle"},
		OrderBy:    "screenPageViews",
	},
	"traffic": {
		Metrics:    []string{"sessions", "activeUsers", "engagedSessions"},
		Dimensions: []string{"sessionDefaultChannelGroup", "sessionSource"},
		OrderBy:    "sessions",
	},
	"devices": {
		Metrics:    []string{"activeUsers", "sessions"},
		Dimensions: []string{"deviceCategory"},
		OrderBy:    "activeUsers",
	},
	"countries": {
		Metrics:    []string{"activeUsers", "sessions"},
		Dimensions: []string{"country"},
		OrderBy:    "activeUsers",
	},
	"overview": {
		Metrics:    []string{"activeUsers", "newUsers", "sessions", "screenPageViews", "averageSessionDuration", "bounceRate"},
		Dimensions: []string{"date"},
		OrderBy:    "date",
	},
}

// GA4Query is a single runReport call: a property, a date window, the metrics and
// dimensions, an optional sort, and a row cap. Build it from a preset or pass raw
// metrics/dimensions straight through (the escape hatch the Python tool kept).
type GA4Query struct {
	Metrics    []string
	Dimensions []string
	Start      string // "28daysAgo" | "today" | "YYYY-MM-DD" (GA4 accepts these natively)
	End        string
	Limit      int
	OrderBy    string // a metric (sorted desc) or dimension name; empty = API default
}

// runReportRequest is the GA4 Data API runReport body.
type runReportRequest struct {
	DateRanges []ga4DateRange `json:"dateRanges"`
	Dimensions []ga4Name      `json:"dimensions,omitempty"`
	Metrics    []ga4Name      `json:"metrics"`
	OrderBys   []ga4OrderBy   `json:"orderBys,omitempty"`
	Limit      int64          `json:"limit,omitempty"`
}

type ga4DateRange struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}
type ga4Name struct {
	Name string `json:"name"`
}
type ga4OrderBy struct {
	Desc      bool             `json:"desc"`
	Metric    *ga4MetricOrder  `json:"metric,omitempty"`
	Dimension *ga4DimensionOrd `json:"dimension,omitempty"`
}
type ga4MetricOrder struct {
	MetricName string `json:"metricName"`
}
type ga4DimensionOrd struct {
	DimensionName string `json:"dimensionName"`
}

// RunReport runs a GA4 Data API runReport and flattens it into a Report.
func (c *Client) RunReport(propertyID string, q GA4Query) (*Report, error) {
	if len(q.Metrics) == 0 {
		return nil, fmt.Errorf("a GA4 report needs at least one metric")
	}
	body := runReportRequest{
		DateRanges: []ga4DateRange{{StartDate: q.Start, EndDate: q.End}},
		Metrics:    namesOf(q.Metrics),
		Dimensions: namesOf(q.Dimensions),
		Limit:      int64(q.Limit),
	}
	if ob := ga4OrderByFor(q); ob != nil {
		body.OrderBys = []ga4OrderBy{*ob}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v1beta/properties/%s:runReport", ga4Base, propertyID)
	raw, err := c.do(ScopeAnalyticsRead, "POST", url, payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Rows []struct {
			DimensionValues []struct {
				Value string `json:"value"`
			} `json:"dimensionValues"`
			MetricValues []struct {
				Value string `json:"value"`
			} `json:"metricValues"`
		} `json:"rows"`
		RowCount int `json:"rowCount"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("could not parse the GA4 response: %w", err)
	}

	headers := append(append([]string{}, q.Dimensions...), q.Metrics...)
	rep := &Report{
		Property:  propertyID,
		DateRange: DateRange{Start: q.Start, End: q.End},
		Headers:   headers,
		RowCount:  resp.RowCount,
		Rows:      []map[string]string{}, // non-nil so empty serializes as [] not null
	}
	for _, r := range resp.Rows {
		row := map[string]string{}
		for i, d := range r.DimensionValues {
			if i < len(q.Dimensions) {
				row[q.Dimensions[i]] = d.Value
			}
		}
		for i, m := range r.MetricValues {
			if i < len(q.Metrics) {
				row[q.Metrics[i]] = m.Value
			}
		}
		rep.Rows = append(rep.Rows, row)
	}
	if rep.RowCount == 0 {
		rep.RowCount = len(rep.Rows)
	}
	return rep, nil
}

// ga4OrderByFor turns the requested sort key into the API's orderBys entry,
// deciding metric-vs-dimension the same way the Python tool did.
func ga4OrderByFor(q GA4Query) *ga4OrderBy {
	if q.OrderBy == "" {
		return nil
	}
	isMetric := contains(q.Metrics, q.OrderBy)
	if q.OrderBy == "date" || contains(q.Dimensions, q.OrderBy) {
		isMetric = false
	}
	if isMetric {
		return &ga4OrderBy{Desc: true, Metric: &ga4MetricOrder{MetricName: q.OrderBy}}
	}
	return &ga4OrderBy{Desc: true, Dimension: &ga4DimensionOrd{DimensionName: q.OrderBy}}
}

func namesOf(xs []string) []ga4Name {
	if len(xs) == 0 {
		return nil
	}
	out := make([]ga4Name, len(xs))
	for i, x := range xs {
		out[i] = ga4Name{Name: x}
	}
	return out
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
