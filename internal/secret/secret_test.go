package secret

import (
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain puts this package on an in-memory keychain before a single test runs.
// These tests write and delete secrets; against the real keychain that would be
// the developer's own, so mocking is not a habit each test has to keep — it is
// the only keychain the package's tests can reach.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// freshKeychain empties the in-memory keychain for this test, so what it asserts
// about presence and absence can't be inherited from an earlier one.
func freshKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInit()
}

func TestStore_RoundTripAndNamespacing(t *testing.T) {
	freshKeychain(t)

	s := New("ws1")

	if _, err := s.Get("bluesky", "app_password"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound before set, got %v", err)
	}

	if err := s.Set("bluesky", "app_password", "secret-123"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("bluesky", "app_password")
	if err != nil || got != "secret-123" {
		t.Fatalf("get after set: %q %v", got, err)
	}

	// A different workspace must not see ws1's secret.
	if _, err := New("ws2").Get("bluesky", "app_password"); !errors.Is(err, ErrNotFound) {
		t.Errorf("workspace namespacing leaked: %v", err)
	}

	if err := s.Delete("bluesky", "app_password"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("bluesky", "app_password"); err != nil {
		t.Errorf("deleting an absent secret should be a no-op, got %v", err)
	}
	if _, err := s.Get("bluesky", "app_password"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_HasAnswersWithoutHandingOverTheValue(t *testing.T) {
	// A status report needs to say "there is a credential here" and no more —
	// so presence is a question the caller can ask without holding the secret.
	freshKeychain(t)

	s := New("cloudflare")
	if s.Has("acct", "api_token") {
		t.Error("nothing is stored yet, so Has must be false")
	}
	if err := s.Set("acct", "api_token", "a-token"); err != nil {
		t.Fatal(err)
	}
	if !s.Has("acct", "api_token") {
		t.Error("a stored secret must read as present")
	}
	if New("sftp").Has("acct", "api_token") {
		t.Error("presence leaked across workspaces")
	}
}
