package cli

import "testing"

func TestCurrentSupportEmptyWhenAbsent(t *testing.T) {
	if sup := currentSupport(t.TempDir()); len(sup) != 0 {
		t.Fatalf("expected no support for a fresh project, got %+v", sup)
	}
}
