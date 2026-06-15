package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// The catalogue is the source of truth for both outputs, so guard its shape:
// every entry filled in, and every status one the text renderer groups under.
func TestFeatureCatalog_WellFormed(t *testing.T) {
	cat := featureCatalog()
	if len(cat) == 0 {
		t.Fatal("feature catalogue is empty")
	}
	known := map[string]bool{"built-in": true, "config": true, "command": true}
	seen := map[string]bool{}
	for _, f := range cat {
		if f.Name == "" || f.What == "" || f.Enable == "" {
			t.Errorf("incomplete feature: %+v", f)
		}
		if !known[f.Status] {
			t.Errorf("feature %q has unknown status %q", f.Name, f.Status)
		}
		if seen[f.Name] {
			t.Errorf("duplicate feature name %q", f.Name)
		}
		seen[f.Name] = true
	}
}

// --json must be valid JSON and carry the whole catalogue, since agents read it.
func TestFeatures_JSON(t *testing.T) {
	out, err := captureStdout(t, func() error { return runFeatures([]string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Features []feature `json:"features"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got.Features) != len(featureCatalog()) {
		t.Errorf("json has %d features, catalogue has %d", len(got.Features), len(featureCatalog()))
	}
}

// The human view should group everything: no entry left out by a status the
// renderer doesn't print.
func TestFeatures_TextCoversCatalog(t *testing.T) {
	out, err := captureStdout(t, func() error { return runFeatures(nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range featureCatalog() {
		if !strings.Contains(out, f.Name) {
			t.Errorf("text output is missing feature %q", f.Name)
		}
	}
}
