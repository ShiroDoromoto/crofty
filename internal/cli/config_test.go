package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// An agent asks `crofty config` where crofty's state lives and whether it may
// write there, instead of learning it by running something that writes (#25).
func TestStateDirState_Writable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(project.HomeEnv, dir)

	s := stateDirState()

	if s.Dir != dir || !s.FromEnv || !s.Writable {
		t.Errorf("stateDirState() = %+v, want %s, from the env, writable", s, dir)
	}
	if s.Env != project.HomeEnv {
		t.Errorf("Env = %q, want %s", s.Env, project.HomeEnv)
	}
	if s.Wall != nil || s.Reason != "" {
		t.Errorf("a writable directory has no wall to report: %+v", s)
	}
}

// The wall is data here, not a failure: config reports it with the ways on, the
// same ones init would have offered, and still prints the rest of the report.
func TestStateDirState_ReportsTheWallWithItsChoices(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only bit does not deny writes on Windows; ACLs do")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only folder")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	mkdir(t, locked)
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	t.Setenv(project.HomeEnv, locked)

	s := stateDirState()

	if s.Writable {
		t.Fatal("Writable = true over a read-only directory")
	}
	if s.Reason == "" {
		t.Error("an unwritable state directory must say why")
	}
	if s.Wall == nil || len(s.Wall.Choices) == 0 {
		t.Fatalf("a permission wall must carry the ways on: %+v", s)
	}
	if !s.Wall.Choices[0].NeedsPermission {
		t.Error("the first choice needs the author's permission and must say so")
	}
	if s.AgentRule == "" {
		t.Error("the rule against inventing a workaround belongs where an agent meets the wall")
	}
}

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
