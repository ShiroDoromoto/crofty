package cli

import (
	"strings"
	"testing"
)

func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.9.0", "0.10.0", true},    // 10 > 9 numerically, not lexically
		{"0.9.0", "0.9.1", true},     // patch bump
		{"0.9.0", "1.0.0", true},     // major bump
		{"1.0.0", "0.9.9", false},    // newer is not older
		{"0.9.0", "0.9.0", false},    // equal is not older
		{"v0.9.0", "v0.10.0", true},  // leading v ignored
		{"0.9.0", "0.9.1-rc1", true}, // pre-release suffix dropped on b
		{"0.9", "0.9.1", true},       // missing patch defaults to 0
		{"garbage", "0.9.0", false},  // unparsable -> never older (no nudge)
		{"0.9.0", "garbage", false},  // unparsable target -> no nudge
	}
	for _, c := range cases {
		if got := semverLess(c.a, c.b); got != c.want {
			t.Errorf("semverLess(%q, %q) = %v; want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	if v, ok := parseVersion("v1.2.3"); !ok || v != [3]int{1, 2, 3} {
		t.Errorf("parseVersion(v1.2.3) = %v, %v", v, ok)
	}
	if v, ok := parseVersion("0.10"); !ok || v != [3]int{0, 10, 0} {
		t.Errorf("parseVersion(0.10) = %v, %v", v, ok)
	}
	if _, ok := parseVersion("1.2.3.4"); ok {
		t.Error("parseVersion(1.2.3.4) should fail (too many components)")
	}
	if _, ok := parseVersion("1.x.3"); ok {
		t.Error("parseVersion(1.x.3) should fail (non-numeric)")
	}
}

func TestUpgradeHintFor(t *testing.T) {
	cases := []struct {
		exe, goos, wantContains string
	}{
		{"/opt/homebrew/Cellar/crofty/0.9.0/bin/crofty", "darwin", "brew upgrade"},
		{"/usr/local/Cellar/crofty/0.9.0/bin/crofty", "darwin", "brew upgrade"},
		{`C:\Users\me\scoop\apps\crofty\current\crofty.exe`, "windows", "scoop update"},
		{"/usr/bin/crofty", "linux", ".deb/.rpm"},                                     // distro package, no repo behind it
		{"/home/me/go/bin/crofty", "linux", "releases"},                               // go install -> fallback
		{"/somewhere/odd/crofty", "darwin", "releases"},                               // unknown -> fallback
		{"/Users/me/.local/bin/crofty", "darwin", "install.sh"},                       // per-user script (macOS)
		{"/home/me/.local/bin/crofty", "linux", "install.sh"},                         // per-user script (Linux)
		{`C:\Users\me\AppData\Local\crofty\bin\crofty.exe`, "windows", "install.ps1"}, // per-user script (Windows)
	}
	for _, c := range cases {
		if got := upgradeHintFor(c.exe, c.goos); !strings.Contains(got, c.wantContains) {
			t.Errorf("upgradeHintFor(%q, %q) = %q; want it to contain %q", c.exe, c.goos, got, c.wantContains)
		}
	}
}

// updateInfo must be inert for source builds and when opted out, so it never
// reaches the network in tests (where Version is the default "dev").
func TestUpdateInfo_InertWhenDevOrOptedOut(t *testing.T) {
	if latest, newer := updateInfo(); latest != "" || newer {
		t.Errorf("updateInfo() on dev build = (%q, %v); want (\"\", false)", latest, newer)
	}

	old := Version
	Version = "0.0.1"
	t.Cleanup(func() { Version = old })
	t.Setenv("CROFTY_NO_UPDATE_CHECK", "1")
	if latest, newer := updateInfo(); latest != "" || newer {
		t.Errorf("updateInfo() opted out = (%q, %v); want (\"\", false)", latest, newer)
	}
}
