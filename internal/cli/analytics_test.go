package cli

import (
	"os"
	"path/filepath"
	"testing"
)

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
		"https://example.com/":      "https://example.com/sitemap.xml",
		"https://example.com":       "https://example.com/sitemap.xml",
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
