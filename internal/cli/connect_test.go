package cli

import (
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

// reconfigureDeploy is the engine behind `crofty connect --provider …`: switching
// the deploy backend (and updating sftp/ftps fields) from the CLI instead of
// hand-editing .crofty/config.json. These tests run non-interactively (stdin is
// not a TTY under `go test`), so chooseDeploy takes the flag values as given.

func TestReconfigureDeploy_NoChangeIsNoop(t *testing.T) {
	p := &project.Project{Root: t.TempDir()}
	cfg := &project.Config{Workspace: "ws", Deploy: project.DeployConfig{Provider: "cloudflare", Project: "blog", AccountID: "acc"}}
	if err := reconfigureDeploy(p, cfg, "cloudflare", "", initDeployFlags{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Deploy.Provider != "cloudflare" || cfg.Deploy.Project != "blog" || cfg.Deploy.AccountID != "acc" {
		t.Fatalf("config should be untouched, got %+v", cfg.Deploy)
	}
}

func TestReconfigureDeploy_SwitchCloudflareToSFTP(t *testing.T) {
	p := &project.Project{Root: t.TempDir()}
	// No AccountID, so no keychain delete is attempted on switch.
	cfg := &project.Config{Workspace: "ws", Deploy: project.DeployConfig{Provider: "cloudflare", Project: "blog"}}

	err := reconfigureDeploy(p, cfg, "cloudflare", "sftp", initDeployFlags{
		provider: "sftp", host: "example.com", user: "me", path: "/var/www/site", port: 2222,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Deploy.Provider != "sftp" || cfg.Deploy.Host != "example.com" || cfg.Deploy.User != "me" ||
		cfg.Deploy.Path != "/var/www/site" || cfg.Deploy.Port != 2222 {
		t.Fatalf("in-memory config not switched: %+v", cfg.Deploy)
	}
	// The change must be persisted to .crofty/config.json.
	got, err := p.LoadConfig()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Deploy.Provider != "sftp" || got.Deploy.Host != "example.com" {
		t.Fatalf("persisted config not switched: %+v", got.Deploy)
	}
}

func TestReconfigureDeploy_UpdateSFTPFieldKeepsProvider(t *testing.T) {
	p := &project.Project{Root: t.TempDir()}
	cfg := &project.Config{Workspace: "ws", Deploy: project.DeployConfig{
		Provider: "sftp", Host: "old.example.com", User: "me", Path: "/srv",
	}}
	// No --provider, only a changed host: stay on sftp, update host, keep the rest.
	if err := reconfigureDeploy(p, cfg, "sftp", "", initDeployFlags{host: "new.example.com"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Deploy.Provider != "sftp" || cfg.Deploy.Host != "new.example.com" ||
		cfg.Deploy.User != "me" || cfg.Deploy.Path != "/srv" {
		t.Fatalf("field update wrong: %+v", cfg.Deploy)
	}
}

func TestReconfigureDeploy_SwitchSFTPToCloudflareKeepsProject(t *testing.T) {
	p := &project.Project{Root: t.TempDir()}
	cfg := &project.Config{Workspace: "ws", Deploy: project.DeployConfig{
		Provider: "sftp", Host: "h", User: "u", Path: "/srv", Project: "myblog",
	}}
	if err := reconfigureDeploy(p, cfg, "sftp", "cloudflare", initDeployFlags{provider: "cloudflare"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Deploy.Provider != "cloudflare" || cfg.Deploy.Project != "myblog" {
		t.Fatalf("switch to cloudflare wrong: %+v", cfg.Deploy)
	}
	// Server fields should be dropped on the cloudflare config.
	if cfg.Deploy.Host != "" || cfg.Deploy.Path != "" {
		t.Fatalf("server fields should be cleared: %+v", cfg.Deploy)
	}
}

func TestReconfigureDeploy_UnknownProvider(t *testing.T) {
	p := &project.Project{Root: t.TempDir()}
	cfg := &project.Config{Workspace: "ws", Deploy: project.DeployConfig{Provider: "cloudflare", Project: "blog"}}
	if err := reconfigureDeploy(p, cfg, "cloudflare", "azure", initDeployFlags{provider: "azure"}); err == nil {
		t.Fatal("expected an error for an unsupported provider")
	}
}
