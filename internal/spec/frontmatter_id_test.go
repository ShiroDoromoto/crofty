package spec

import (
	"strings"
	"testing"
)

func TestEnsureCroftyID_AddsAndPreserves(t *testing.T) {
	src := []byte("---\ntitle: \"Walk\"\ndate: 2026-06-14\ncrofty:\n  tier: full\n---\n\nBody stays.\n")

	gotID, created, out, err := EnsureCroftyID(src, "01J9X8Z7Q2H4M5N6P7R8S9T0AB")
	if err != nil {
		t.Fatal(err)
	}
	if !created || gotID != "01J9X8Z7Q2H4M5N6P7R8S9T0AB" {
		t.Fatalf("created=%v id=%q", created, gotID)
	}
	s := string(out)
	if !strings.Contains(s, "id: 01J9X8Z7Q2H4M5N6P7R8S9T0AB") {
		t.Errorf("id not written: %s", s)
	}
	if !strings.Contains(s, "tier: full") {
		t.Errorf("existing crofty keys lost: %s", s)
	}
	if !strings.HasSuffix(s, "Body stays.\n") {
		t.Errorf("body not preserved: %q", s)
	}

	// The result must re-parse and now carry the id.
	fm, err := parseFrontmatter(out)
	if err != nil {
		t.Fatal(err)
	}
	if cm, ok := toStringMap(fm["crofty"]); !ok || cm["id"] != "01J9X8Z7Q2H4M5N6P7R8S9T0AB" {
		t.Errorf("re-parsed id mismatch: %+v", fm["crofty"])
	}
}

func TestEnsureCroftyID_AddsCroftyBlockWhenMissing(t *testing.T) {
	src := []byte("---\ntitle: \"Walk\"\ndate: 2026-06-14\n---\nBody\n")
	id, created, out, err := EnsureCroftyID(src, "01J9X8Z7Q2H4M5N6P7R8S9T0AB")
	if err != nil || !created || id == "" {
		t.Fatalf("created=%v id=%q err=%v", created, id, err)
	}
	if !strings.Contains(string(out), "crofty:") || !strings.Contains(string(out), "id: ") {
		t.Errorf("crofty block not added: %s", out)
	}
}

func TestEnsureCroftyID_NoopWhenPresent(t *testing.T) {
	const existing = "01EXISTING0000000000000000"
	src := []byte("---\ntitle: \"Walk\"\ndate: 2026-06-14\ncrofty:\n  id: " + existing + "\n---\nBody\n")
	id, created, out, err := EnsureCroftyID(src, "01NEWNEWNEWNEWNEWNEWNEWNEW")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("should not create when id already present")
	}
	if id != existing {
		t.Errorf("returned id = %q, want %q", id, existing)
	}
	if string(out) != string(src) {
		t.Error("content changed despite existing id")
	}
}
