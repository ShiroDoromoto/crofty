package secret

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestStore_RoundTripAndNamespacing(t *testing.T) {
	keyring.MockInit() // in-memory keychain; no real OS access in tests

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
