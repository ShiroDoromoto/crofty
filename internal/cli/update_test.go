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
		exe, goos    string
		wantContains []string
	}{
		// Homebrew and Scoop are dead channels: their taps are frozen, so the hint
		// must send people out rather than back to an upgrade that does nothing.
		{"/opt/homebrew/Cellar/crofty/0.9.0/bin/crofty", "darwin", []string{"brew uninstall", "no longer ships to Homebrew"}},
		{"/usr/local/Cellar/crofty/0.9.0/bin/crofty", "darwin", []string{"brew uninstall", "no longer ships to Homebrew"}},
		{`C:\Users\me\scoop\apps\crofty\current\crofty.exe`, "windows", []string{"scoop uninstall", "no longer ships to Scoop"}},
		// The .deb/.rpm are a dead channel too, and no repo ever stood behind them,
		// so this hint is the only thing that tells those users to leave.
		{"/usr/bin/crofty", "linux", []string{"apt remove", "no longer ships a .deb/.rpm"}},
		{"/home/me/go/bin/crofty", "linux", []string{"releases"}},           // go install -> fallback
		{"/somewhere/odd/crofty", "darwin", []string{"releases"}},           // unknown -> fallback
		{"/Users/me/.local/bin/crofty", "darwin", []string{"install.sh"}},   // per-user script (macOS)
		{"/home/me/.local/bin/crofty", "linux", []string{"install.sh"}},     // per-user script (Linux)
		{"/usr/local/bin/crofty", "linux", []string{"install.sh", "sudo"}},  // PREFIX=/usr/local script
		{"/usr/local/bin/crofty", "darwin", []string{"install.sh", "sudo"}}, // same, and not the .deb hint
		// %LOCALAPPDATA%\crofty\bin holds both the click installer's copy and
		// install.ps1's, so the hint names the installer, which fixes either.
		{`C:\Users\me\AppData\Local\crofty\bin\crofty.exe`, "windows", []string{"crofty-setup.exe"}},
	}
	for _, c := range cases {
		got := upgradeHintFor(c.exe, c.goos)
		for _, want := range c.wantContains {
			if !strings.Contains(got, want) {
				t.Errorf("upgradeHintFor(%q, %q) = %q; want it to contain %q", c.exe, c.goos, got, want)
			}
		}
		// `irm ... | iex` failed in schannel on a real Windows box. No hint may
		// route an update back through it, whatever the binary's path.
		if strings.Contains(got, "iex") {
			t.Errorf("upgradeHintFor(%q, %q) = %q; must not send an update through 'irm | iex'", c.exe, c.goos, got)
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
