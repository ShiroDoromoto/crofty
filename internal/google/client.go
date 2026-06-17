package google

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Base URLs are package vars so tests can point them at an httptest server. The
// token endpoint is not here — it comes from the service-account key's TokenURI,
// which tests set directly on the credentials.
var (
	ga4Base = "https://analyticsdata.googleapis.com"
	gscBase = "https://www.googleapis.com"
)

// nowFunc returns the current time. It is a package var so tests can pin the JWT
// iat/exp; production uses the real clock.
var nowFunc = time.Now

// Client is a thin, authenticated HTTP client over one service account. It
// caches the access token per scope for the life of a single CLI run (a run
// makes at most a couple of calls, so this just avoids a redundant exchange).
type Client struct {
	creds  *Credentials
	http   *http.Client
	tokens map[string]string // scope -> access token
}

// NewClient returns a client bound to the given credentials.
func NewClient(creds *Credentials) *Client {
	return &Client{
		creds:  creds,
		http:   &http.Client{Timeout: 30 * time.Second},
		tokens: map[string]string{},
	}
}

// Email is the service account's address — used by the CLI to tell the author
// exactly which member to grant access to.
func (c *Client) Email() string { return c.creds.ClientEmail }

// ProjectID is the GCP project the key belongs to — used to deep-link the right
// "enable this API" console page.
func (c *Client) ProjectID() string { return c.creds.ProjectID }

func (c *Client) token(scope string) (string, error) {
	if t := c.tokens[scope]; t != "" {
		return t, nil
	}
	t, err := c.creds.accessToken(c.http, scope, nowFunc())
	if err != nil {
		return "", err
	}
	c.tokens[scope] = t
	return t, nil
}

// do runs an authenticated request and returns the raw body, or a typed
// *APIError when Google reports one. method/url/body are the HTTP request; scope
// selects (and caches) the access token.
func (c *Client) do(scope, method, url string, body []byte) ([]byte, error) {
	tok, err := c.token(scope)
	if err != nil {
		return nil, err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(b, resp.StatusCode)
	}
	return b, nil
}

// APIError is a Google API error, classified enough that the CLI can map it to a
// concrete fix (enable the API, or grant the service account access).
type APIError struct {
	Code    int
	Status  string // e.g. PERMISSION_DENIED (GA4 / google.rpc.Status)
	Reason  string // e.g. SERVICE_DISABLED, accessNotConfigured (legacy)
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Google API error (HTTP %d)", e.Code)
}

// Disabled reports that the API itself is not enabled in the GCP project — the
// fix is to turn it on, not to grant access.
func (e *APIError) Disabled() bool {
	r := strings.ToUpper(e.Reason)
	if r == "SERVICE_DISABLED" || r == "ACCESSNOTCONFIGURED" {
		return true
	}
	m := strings.ToLower(e.Message)
	return strings.Contains(m, "has not been used") || strings.Contains(m, "is disabled") ||
		strings.Contains(m, "before or it is disabled")
}

// Forbidden reports a 403 that is an access problem (the service account is not
// a member of the property), as opposed to a disabled API.
func (e *APIError) Forbidden() bool {
	return e.Code == http.StatusForbidden && !e.Disabled()
}

// parseAPIError unpacks Google's error envelope, which comes in two shapes: the
// newer google.rpc.Status (GA4 Data API) and the legacy errors[] form (Search
// Console / webmasters v3). It reads whichever is present.
func parseAPIError(body []byte, status int) *APIError {
	var env struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
			Errors  []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"errors"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	e := &APIError{Code: status}
	if json.Unmarshal(body, &env) == nil && (env.Error.Code != 0 || env.Error.Message != "") {
		if env.Error.Code != 0 {
			e.Code = env.Error.Code
		}
		e.Status = env.Error.Status
		e.Message = env.Error.Message
		for _, d := range env.Error.Details {
			if d.Reason != "" {
				e.Reason = d.Reason
				break
			}
		}
		if e.Reason == "" && len(env.Error.Errors) > 0 {
			e.Reason = env.Error.Errors[0].Reason
		}
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("Google API error (HTTP %d)", status)
	}
	return e
}

// Report is the shared shape both GA4 and Search Console results serialize to —
// a header row plus rows keyed by header, mirroring the Python scripts' JSON so
// an agent parses one format for both sources.
type Report struct {
	Property  string              `json:"property,omitempty"` // GA4 numeric property id
	Site      string              `json:"site,omitempty"`     // Search Console property
	DateRange DateRange           `json:"dateRange"`
	Headers   []string            `json:"headers"`
	Rows      []map[string]string `json:"rows"`
	RowCount  int                 `json:"rowCount"`
}

// DateRange is the window a report covers, echoed back for clarity.
type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
