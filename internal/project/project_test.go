package project

import (
	"testing"
)

// SaveConfig/LoadConfig must round-trip the full deploy config, including the
// SFTP/FTPS fields, so a configured destination survives across runs.
func TestConfigRoundTrip(t *testing.T) {
	proj := &Project{Root: t.TempDir()}
	want := &Config{
		Workspace: "ws123",
		Deploy: DeployConfig{
			Provider: "sftp",
			Host:     "example.com",
			Port:     2222,
			User:     "deploy",
			Path:     "/var/www/site",
			KeyPath:  "/home/me/.ssh/id_ed25519",
		},
	}
	if err := proj.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := proj.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Deploy != want.Deploy {
		t.Errorf("deploy round-trip\n got %+v\nwant %+v", got.Deploy, want.Deploy)
	}
	if got.Workspace != want.Workspace {
		t.Errorf("workspace = %q; want %q", got.Workspace, want.Workspace)
	}
}

// A Cloudflare config (the common case) must still round-trip unchanged.
func TestConfigRoundTrip_Cloudflare(t *testing.T) {
	proj := &Project{Root: t.TempDir()}
	want := &Config{
		Workspace: "ws",
		Deploy:    DeployConfig{Provider: "cloudflare", Project: "blog", Branch: "main", AccountID: "acc"},
	}
	if err := proj.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := proj.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Deploy != want.Deploy {
		t.Errorf("deploy round-trip\n got %+v\nwant %+v", got.Deploy, want.Deploy)
	}
}
