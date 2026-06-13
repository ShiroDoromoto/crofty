// Package secret stores per-target credentials in the OS keychain (A5). crofty
// never sends these anywhere of ours — there is no server (A7). They are the
// user's own tokens for the user's own destinations, kept locally. Secret values
// are entered through a hidden prompt and never pass through agent context.
package secret

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// service groups all crofty keychain entries under one service name so an
// uninstall can reason about them (A5).
const service = "crofty"

// ErrNotFound is returned when no secret exists for the given target/field.
var ErrNotFound = errors.New("secret not found")

// Store reads and writes credentials for one workspace. Keys are namespaced
// workspace:target:field so multiple sites and targets never collide (A5).
type Store struct {
	workspace string
}

// New returns a Store scoped to a workspace id.
func New(workspace string) *Store { return &Store{workspace: workspace} }

func (s *Store) account(target, field string) string {
	return s.workspace + ":" + target + ":" + field
}

// Get returns the stored secret, or ErrNotFound if absent.
func (s *Store) Get(target, field string) (string, error) {
	v, err := keyring.Get(service, s.account(target, field))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

// Set stores (or replaces) a secret.
func (s *Store) Set(target, field, value string) error {
	return keyring.Set(service, s.account(target, field), value)
}

// Delete removes a secret; absence is not an error.
func (s *Store) Delete(target, field string) error {
	err := keyring.Delete(service, s.account(target, field))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
