package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetProfileSupportCreatesFile(t *testing.T) {
	root := t.TempDir()
	if err := setProfileSupport(root, "stripe", "https://buy.stripe.com/x"); err != nil {
		t.Fatal(err)
	}
	sup := currentSupport(root)
	if sup["stripe"] != "https://buy.stripe.com/x" {
		t.Fatalf("stripe not saved: %+v", sup)
	}
}

// setProfileSupport must preserve support entries and other top-level keys a
// user (or a previous run) already wrote — it edits, never clobbers.
func TestSetProfileSupportPreservesExisting(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "support:\n  github_sponsors: octocat\n  message: hi\nname: Ada\n"
	if err := os.WriteFile(profilePath(root), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setProfileSupport(root, "stripe", "https://buy.stripe.com/x"); err != nil {
		t.Fatal(err)
	}

	m, err := loadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	sup, _ := asStringMap(m["support"])
	if sup["stripe"] != "https://buy.stripe.com/x" {
		t.Errorf("new stripe link missing: %+v", sup)
	}
	if sup["github_sponsors"] != "octocat" || sup["message"] != "hi" {
		t.Errorf("existing support entries were clobbered: %+v", sup)
	}
	if m["name"] != "Ada" {
		t.Errorf("unrelated top-level key was lost: %+v", m)
	}
}

func TestCurrentSupportEmptyWhenAbsent(t *testing.T) {
	if sup := currentSupport(t.TempDir()); len(sup) != 0 {
		t.Fatalf("expected no support for a fresh project, got %+v", sup)
	}
}
