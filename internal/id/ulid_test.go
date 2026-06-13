package id

import (
	"regexp"
	"testing"
)

// ulidRe mirrors the validator's ULID rule (internal/spec). Generated ids must
// satisfy it.
var ulidRe = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

func TestNewULID_FormatAndUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		u, err := NewULID()
		if err != nil {
			t.Fatal(err)
		}
		if !ulidRe.MatchString(u) {
			t.Fatalf("generated id is not a valid ULID: %q", u)
		}
		if seen[u] {
			t.Fatalf("duplicate ULID: %q", u)
		}
		seen[u] = true
	}
}
