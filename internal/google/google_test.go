package google

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testCreds builds a real (test-generated) service-account credential whose
// token endpoint points at the given URL, so the whole sign→exchange→call path
// runs for real against an httptest server.
func testCreds(t *testing.T, tokenURL string) *Credentials {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	saJSON, _ := json.Marshal(saKeyFile{
		Type:        "service_account",
		ProjectID:   "my-proj",
		PrivateKey:  string(pemBytes),
		ClientEmail: "sa@my-proj.iam.gserviceaccount.com",
		TokenURI:    tokenURL,
	})
	creds, err := ParseCredentials(saJSON)
	if err != nil {
		t.Fatalf("ParseCredentials: %v", err)
	}
	return creds
}

// pinClock fixes nowFunc and the base URLs for one test, restoring them after.
func pinClock(t *testing.T, ga4, gsc string) {
	t.Helper()
	prevNow, prevGA4, prevGSC := nowFunc, ga4Base, gscBase
	nowFunc = func() time.Time { return time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC) }
	ga4Base, gscBase = ga4, gsc
	t.Cleanup(func() { nowFunc, ga4Base, gscBase = prevNow, prevGA4, prevGSC })
}

func TestRunReport(t *testing.T) {
	var gotAuth, gotBody string
	var tokenHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			tokenHits++
			if err := r.ParseForm(); err != nil {
				t.Errorf("parsing token form: %v", err)
			}
			if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("grant_type = %q", r.FormValue("grant_type"))
			}
			if r.FormValue("assertion") == "" {
				t.Error("no assertion in token request")
			}
			fmt.Fprint(w, `{"access_token":"tok-123","expires_in":3600}`)
		case strings.HasSuffix(r.URL.Path, ":runReport"):
			gotAuth = r.Header.Get("Authorization")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			fmt.Fprint(w, `{"rows":[
				{"dimensionValues":[{"value":"/a"},{"value":"A"}],"metricValues":[{"value":"10"},{"value":"3"}]},
				{"dimensionValues":[{"value":"/b"},{"value":"B"}],"metricValues":[{"value":"5"},{"value":"2"}]}
			],"rowCount":2}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	pinClock(t, srv.URL, srv.URL)

	c := NewClient(testCreds(t, srv.URL+"/token"))
	p := GA4Presets["top-pages"]
	rep, err := c.RunReport("541957758", GA4Query{
		Metrics: p.Metrics, Dimensions: p.Dimensions, Start: "28daysAgo", End: "today", Limit: 25, OrderBy: p.OrderBy,
	})
	if err != nil {
		t.Fatalf("RunReport: %v", err)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("auth = %q, want Bearer tok-123", gotAuth)
	}
	if strings.Contains(gotBody, `"property"`) {
		t.Errorf("runReport body should not carry property (it's in the URL): %s", gotBody)
	}
	if !strings.Contains(gotBody, `"screenPageViews"`) {
		t.Errorf("request body missing the metric: %s", gotBody)
	}
	if rep.Property != "541957758" || rep.RowCount != 2 || len(rep.Rows) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Rows[0]["pagePath"] != "/a" || rep.Rows[0]["screenPageViews"] != "10" {
		t.Errorf("row0 = %v", rep.Rows[0])
	}
	if rep.DateRange.Start != "28daysAgo" {
		t.Errorf("dateRange = %+v", rep.DateRange)
	}

	// A second call reuses the cached access token (no second token exchange).
	if _, err := c.RunReport("541957758", GA4Query{Metrics: []string{"sessions"}, Start: "7daysAgo", End: "today"}); err != nil {
		t.Fatalf("second RunReport: %v", err)
	}
	if tokenHits != 1 {
		t.Errorf("token exchanged %d times, want 1 (cached)", tokenHits)
	}
}

func TestSearchAnalytics(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			fmt.Fprint(w, `{"access_token":"tok-xyz"}`)
		case strings.HasSuffix(r.URL.Path, "/searchAnalytics/query"):
			gotPath = r.RequestURI // raw, still-encoded path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			fmt.Fprint(w, `{"rows":[
				{"keys":["hello"],"clicks":12,"impressions":340,"ctr":0.0353,"position":4.2}
			]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	pinClock(t, srv.URL, srv.URL)

	c := NewClient(testCreds(t, srv.URL+"/token"))
	rep, err := c.SearchAnalytics("sc-domain:shiro-doro.site", GSCQuery{
		Dimensions: GSCPresets["queries"], Start: "28daysAgo", End: "today", Limit: 25,
	})
	if err != nil {
		t.Fatalf("SearchAnalytics: %v", err)
	}
	// The property must be percent-encoded into the path (colon encoded).
	if !strings.Contains(gotPath, "sc-domain%3Ashiro-doro.site") {
		t.Errorf("path missing encoded site: %s", gotPath)
	}
	if !strings.Contains(gotBody, `"startDate":"2026-05-20"`) {
		t.Errorf("28daysAgo not resolved to ISO in body: %s", gotBody)
	}
	if rep.Site != "sc-domain:shiro-doro.site" || len(rep.Rows) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	r0 := rep.Rows[0]
	if r0["query"] != "hello" || r0["clicks"] != "12" || r0["impressions"] != "340" || r0["ctr"] != "3.5%" || r0["position"] != "4.2" {
		t.Errorf("row0 = %v", r0)
	}
}

func TestSitesAndSitemaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			fmt.Fprint(w, `{"access_token":"t"}`)
		case strings.HasSuffix(r.URL.Path, "/sites"):
			fmt.Fprint(w, `{"siteEntry":[{"siteUrl":"sc-domain:shiro-doro.site","permissionLevel":"siteFullUser"}]}`)
		case strings.HasSuffix(r.URL.Path, "/sitemaps"):
			fmt.Fprint(w, `{"sitemap":[{"path":"https://shiro-doro.site/sitemap.xml","isPending":false}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	pinClock(t, srv.URL, srv.URL)
	c := NewClient(testCreds(t, srv.URL+"/token"))

	sites, err := c.Sites()
	if err != nil || len(sites) != 1 || sites[0].Permission != "siteFullUser" {
		t.Fatalf("Sites: %v / %+v", err, sites)
	}
	sm, err := c.Sitemaps("sc-domain:shiro-doro.site")
	if err != nil || len(sm) != 1 || sm[0].Path == "" {
		t.Fatalf("Sitemaps: %v / %+v", err, sm)
	}
}

func TestAPIErrorClassification(t *testing.T) {
	disabled := parseAPIError([]byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"Analytics Data API has not been used in project 123 before or it is disabled.","details":[{"reason":"SERVICE_DISABLED"}]}}`), 403)
	if !disabled.Disabled() {
		t.Error("expected Disabled() for a SERVICE_DISABLED error")
	}
	if disabled.Forbidden() {
		t.Error("a disabled-API error must not classify as a plain access denial")
	}

	denied := parseAPIError([]byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"User does not have sufficient permissions for this property."}}`), 403)
	if !denied.Forbidden() {
		t.Error("expected Forbidden() for a permission error")
	}
	if denied.Disabled() {
		t.Error("a permission error must not classify as disabled")
	}

	// Legacy webmasters shape (errors[] with a reason).
	legacy := parseAPIError([]byte(`{"error":{"code":403,"message":"Access denied.","errors":[{"reason":"accessNotConfigured"}]}}`), 403)
	if !legacy.Disabled() {
		t.Error("expected Disabled() for legacy accessNotConfigured")
	}
}

func TestRunReportSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t"}`)
			return
		}
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"no access","details":[{"reason":"SERVICE_DISABLED"}]}}`)
	}))
	defer srv.Close()
	pinClock(t, srv.URL, srv.URL)
	c := NewClient(testCreds(t, srv.URL+"/token"))

	_, err := c.RunReport("1", GA4Query{Metrics: []string{"sessions"}, Start: "today", End: "today"})
	var apiErr *APIError
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if !apiErr.Disabled() {
		t.Errorf("expected a disabled-API classification, got %+v", apiErr)
	}
}

func TestResolveDate(t *testing.T) {
	pinClock(t, "", "")
	if got := ResolveDate("today"); got != "2026-06-17" {
		t.Errorf("today = %q", got)
	}
	if got := ResolveDate("7daysAgo"); got != "2026-06-10" {
		t.Errorf("7daysAgo = %q", got)
	}
	if got := ResolveDate("2026-01-01"); got != "2026-01-01" {
		t.Errorf("iso = %q", got)
	}
}
