package cli

import (
	"reflect"
	"testing"
)

func TestHighlightOn(t *testing.T) {
	on := map[string]any{"markup": map[string]any{"highlight": map[string]any{"noClasses": false}}}
	if !highlightOn(on) {
		t.Error("noClasses:false should mean class-based highlight is on")
	}
	inline := map[string]any{"markup": map[string]any{"highlight": map[string]any{"noClasses": true}}}
	if highlightOn(inline) {
		t.Error("noClasses:true means inline styles — highlight (theme-following) is off")
	}
	if highlightOn(map[string]any{}) {
		t.Error("absent highlight config defaults to off (Hugo's noClasses default is true)")
	}
}

func TestAnalyticsProviders(t *testing.T) {
	cfg := map[string]any{"params": map[string]any{"crofty": map[string]any{"analytics": map[string]any{
		"google_tag": "G-X",
		"cloudflare": "", // empty → not configured
		"adsense":    map[string]any{"client": "ca-pub-1"},
	}}}}
	got := analyticsProviders(cfg)
	want := []string{"adsense", "google_tag"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("analyticsProviders = %v, want %v", got, want)
	}
	if got := analyticsProviders(map[string]any{}); len(got) != 0 {
		t.Errorf("no analytics → empty, got %v", got)
	}
}

func TestSiteTitle(t *testing.T) {
	if got := siteTitle(map[string]any{"title": "Top"}); got != "Top" {
		t.Errorf("siteTitle top-level = %q, want Top", got)
	}
	multi := map[string]any{
		"defaultContentLanguage": "ja",
		"languages": map[string]any{
			"ja": map[string]any{"title": "日本語タイトル"},
			"en": map[string]any{"title": "English"},
		},
	}
	if got := siteTitle(multi); got != "日本語タイトル" {
		t.Errorf("siteTitle multilingual = %q, want the default language's title", got)
	}
}
