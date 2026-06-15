package cli

import "testing"

func TestDefaultLanguage(t *testing.T) {
	cases := []struct {
		cfg  map[string]any
		want string
	}{
		{map[string]any{"defaultContentLanguage": "ja"}, "ja"},
		{map[string]any{"locale": "fr"}, "fr"},                                // falls back to top-level locale
		{map[string]any{"defaultContentLanguage": "de", "locale": "x"}, "de"}, // explicit wins
		{map[string]any{}, "en"},                                              // Hugo's own default
	}
	for _, c := range cases {
		if got := defaultLanguage(c.cfg); got != c.want {
			t.Errorf("defaultLanguage(%v) = %q, want %q", c.cfg, got, c.want)
		}
	}
}

func TestConfiguredLanguages(t *testing.T) {
	if got := configuredLanguages(map[string]any{}); len(got) != 0 {
		t.Errorf("single-language site should report no languages, got %v", got)
	}
	cfg := map[string]any{"languages": map[string]any{"en": map[string]any{}, "ja": map[string]any{}}}
	if got := configuredLanguages(cfg); len(got) != 2 {
		t.Errorf("expected 2 configured languages, got %d", len(got))
	}
}

func TestLangLabel(t *testing.T) {
	if got := langLabel("ja"); got != "日本語" {
		t.Errorf("langLabel(ja) = %q, want 日本語", got)
	}
	if got := langLabel("xx"); got != "XX" { // unknown → uppercased code
		t.Errorf("langLabel(xx) = %q, want XX", got)
	}
}

func TestLangCodeRe(t *testing.T) {
	good := []string{"en", "ja", "fr", "zh-Hant", "pt-BR"}
	bad := []string{"", "e", "english", "EN", "ja_JP", "123"}
	for _, c := range good {
		if !langCodeRe.MatchString(c) {
			t.Errorf("%q should be a valid language code", c)
		}
	}
	for _, c := range bad {
		if langCodeRe.MatchString(c) {
			t.Errorf("%q should be rejected as a language code", c)
		}
	}
}
