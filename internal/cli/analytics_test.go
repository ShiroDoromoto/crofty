package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/google"
)

// captureOutput runs fn with os.Stdout and os.Stderr redirected to pipes and
// returns what each received — so the guidance printers can be asserted against.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()
	fn()
	wOut.Close()
	wErr.Close()
	ob, _ := io.ReadAll(rOut)
	eb, _ := io.ReadAll(rErr)
	return string(ob), string(eb)
}

func TestAnalyticsTargets(t *testing.T) {
	dir := t.TempDir()
	// ga4_property is intentionally unquoted here — YAML parses it as an int, and
	// analyticsTargets must still surface it as a string (a common author mistake
	// that should just work, not silently read as empty).
	hugo := `params:
  crofty:
    analytics:
      google_tag: "G-XXXX"
      ga4_property: 123456789
      search_console: "sc-domain:example.com"
`
	if err := os.WriteFile(filepath.Join(dir, "hugo.yaml"), []byte(hugo), 0o644); err != nil {
		t.Fatal(err)
	}
	ga4, sc := analyticsTargets(dir)
	if ga4 != "123456789" {
		t.Errorf("ga4 property = %q, want 123456789 (unquoted int must coerce)", ga4)
	}
	if sc != "sc-domain:example.com" {
		t.Errorf("search console = %q", sc)
	}
}

func TestAnalyticsTargetsAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hugo.yaml"), []byte("title: hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ga4, sc := analyticsTargets(dir)
	if ga4 != "" || sc != "" {
		t.Errorf("expected empty targets, got %q / %q", ga4, sc)
	}
}

func TestDefaultSitemapURL(t *testing.T) {
	cases := map[string]string{
		"sc-domain:example.com": "https://example.com/sitemap.xml",
		"https://example.com/":  "https://example.com/sitemap.xml",
		"https://example.com":   "https://example.com/sitemap.xml",
	}
	for in, want := range cases {
		if got := defaultSitemapURL(in); got != want {
			t.Errorf("defaultSitemapURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAsString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"123456789", "123456789"},
		{123456789, "123456789"},
		{int64(123456789), "123456789"},
		{float64(123456789), "123456789"},
		{nil, ""},
		{true, ""},
	}
	for _, c := range cases {
		if got := asString(c.in); got != c.want {
			t.Errorf("asString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitComma(t *testing.T) {
	got := splitComma(" a, b ,,c ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitComma = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitComma = %v, want %v", got, want)
		}
	}
}

func TestAnalyticsUnknownSubcommand(t *testing.T) {
	if err := runAnalytics([]string{"bogus"}); err == nil {
		t.Error("expected an error for an unknown analytics subcommand")
	}
}

// A 403 against a property the key can't see must be diagnosed as a wrong id, not
// as missing access — the regression this whole change exists for. The JSON form
// (what an agent reads) must carry kind=wrongProperty, the configured id, the
// reachable candidates, and Google's own message.
func TestGA4WrongPropertyGuidanceJSON(t *testing.T) {
	ae := &google.APIError{Code: 403, Status: "PERMISSION_DENIED",
		Message: "User does not have sufficient permissions for this property."}
	props := []google.GA4Property{
		{ID: "123456789", Name: "example.com"},
		{ID: "987654321", Name: "example.org"},
	}
	out, _ := captureOutput(t, func() {
		_ = ga4WrongPropertyGuidance(ae, "sa@example.iam.gserviceaccount.com", "555555555", props, true)
	})
	var got struct {
		Error apiErrorBody `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("emitted non-JSON under --json: %v\n%s", err, out)
	}
	if got.Error.Kind != "wrongProperty" {
		t.Errorf("kind = %q, want wrongProperty", got.Error.Kind)
	}
	if got.Error.ConfiguredProperty != "555555555" {
		t.Errorf("configuredProperty = %q", got.Error.ConfiguredProperty)
	}
	if len(got.Error.AvailableProperties) != 2 || got.Error.AvailableProperties[0].ID != "123456789" {
		t.Errorf("availableProperties = %+v", got.Error.AvailableProperties)
	}
	if !strings.Contains(got.Error.GoogleError.Message, "sufficient permissions") {
		t.Errorf("googleError.message not carried through: %q", got.Error.GoogleError.Message)
	}
}

// The wrong-property text form must list the candidate ids and must NOT tell the
// author to add a viewer (the misdirection that cost time in the incident).
func TestGA4WrongPropertyGuidanceText(t *testing.T) {
	ae := &google.APIError{Code: 403, Status: "PERMISSION_DENIED",
		Message: "User does not have sufficient permissions for this property."}
	props := []google.GA4Property{{ID: "123456789", Name: "example.com"}}
	_, errOut := captureOutput(t, func() {
		_ = ga4WrongPropertyGuidance(ae, "sa@example.iam.gserviceaccount.com", "555555555", props, false)
	})
	if !strings.Contains(errOut, "123456789") || !strings.Contains(errOut, "isn't one this service account can see") {
		t.Errorf("text guidance missing candidate id / wrong-id framing:\n%s", errOut)
	}
	if strings.Contains(errOut, "Add this email as a viewer") {
		t.Errorf("wrong-id case must not give the viewer instruction:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Google said:") {
		t.Errorf("Google's raw message should be carried through:\n%s", errOut)
	}
}

// A genuine access 403 keeps the viewer instruction and still surfaces Google's
// own message (proposal B) without the synthetic fallback leaking through.
func TestAccessGuidanceCarriesGoogleMessage(t *testing.T) {
	ae := &google.APIError{Code: 403, Status: "PERMISSION_DENIED",
		Message: "User does not have sufficient permissions for this property."}
	_, errOut := captureOutput(t, func() {
		_ = accessGuidance(ae, "sa@example.iam.gserviceaccount.com", "ga4", "123456789", false)
	})
	if !strings.Contains(errOut, "Add this email as a viewer") {
		t.Errorf("access case should still give the viewer instruction:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Google said: User does not have sufficient permissions") {
		t.Errorf("Google's message should be appended:\n%s", errOut)
	}
}

// When Google sent no message of its own, parseAPIError fills a synthetic
// "Google API error (HTTP n)" — that stand-in must not be echoed as "Google said".
func TestEmitAPIErrorSkipsSyntheticMessage(t *testing.T) {
	ae := &google.APIError{Code: 403, Message: "Google API error (HTTP 403)"}
	_, errOut := captureOutput(t, func() {
		_ = accessGuidance(ae, "sa@example.iam.gserviceaccount.com", "ga4", "1", false)
	})
	if strings.Contains(errOut, "Google said:") {
		t.Errorf("synthetic message must not be echoed:\n%s", errOut)
	}
}
