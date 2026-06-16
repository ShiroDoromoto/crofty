package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/project"
)

func withCFServer(t *testing.T, h http.HandlerFunc) func() {
	t.Helper()
	srv := httptest.NewServer(h)
	prev := cfAPIBase
	cfAPIBase = srv.URL
	return func() { cfAPIBase = prev; srv.Close() }
}

func TestCFListAccounts(t *testing.T) {
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"message":"Invalid token"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"acct1","name":"My Account"}]}`))
	})()

	accts, err := cfListAccounts("good")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(accts) != 1 || accts[0].id != "acct1" || accts[0].name != "My Account" {
		t.Fatalf("got %+v", accts)
	}

	if _, err := cfListAccounts("bad"); err == nil {
		t.Fatal("expected an error for a rejected token")
	}
}

func TestPickAccountKeepsReachablePin(t *testing.T) {
	// A pinned account the token can still reach is used as-is, without listing.
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			t.Errorf("should not list accounts when the pin is still reachable")
		}
		w.Write([]byte(`{"success":true,"result":[]}`))
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "pinned"
	got, ok, err := pickAccount("tok", cfg, "")
	if err != nil || !ok || got.id != "pinned" {
		t.Fatalf("got (%+v, %v, %v), want pinned", got, ok, err)
	}
}

func TestPickAccountStalePinFallsThrough(t *testing.T) {
	// The token can't reach the pinned account but can list exactly one other —
	// crofty must switch to it instead of dead-ending on --account.
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/old/pages/projects":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
		case "/accounts":
			w.Write([]byte(`{"success":true,"result":[{"id":"new","name":"New Account"}]}`))
		default:
			w.Write([]byte(`{"success":true,"result":[]}`))
		}
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "old"
	got, ok, err := pickAccount("tok", cfg, "")
	if err != nil || !ok || got.id != "new" {
		t.Fatalf("got (%+v, %v, %v), want new", got, ok, err)
	}
}

func TestPickAccountExplicitFlagWins(t *testing.T) {
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts/chosen/pages/projects" {
			w.Write([]byte(`{"success":true,"result":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
	})()

	cfg := &project.Config{}
	cfg.Deploy.AccountID = "old"
	got, ok, err := pickAccount("tok", cfg, "chosen")
	if err != nil || !ok || got.id != "chosen" {
		t.Fatalf("got (%+v, %v, %v), want chosen", got, ok, err)
	}
}

func TestParseMenuChoice(t *testing.T) {
	cases := []struct {
		line string
		max  int
		n    int
		ok   bool
	}{
		{"1\n", 3, 1, true},
		{"  2 \n", 3, 2, true},
		{"3", 3, 3, true},
		{"0\n", 3, 0, false},
		{"4\n", 3, 0, false},
		{"x\n", 3, 0, false},
		{"\n", 3, 0, false},
	}
	for _, c := range cases {
		n, ok := parseMenuChoice(c.line, c.max)
		if n != c.n || ok != c.ok {
			t.Errorf("parseMenuChoice(%q,%d) = (%d,%v), want (%d,%v)", c.line, c.max, n, ok, c.n, c.ok)
		}
	}
}

func TestCFVerifyPagesAccess(t *testing.T) {
	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts/ok/pages/projects" {
			w.Write([]byte(`{"success":true,"result":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false,"errors":[{"message":"no access"}]}`))
	})()

	if err := cfVerifyPagesAccess("tok", "ok"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if err := cfVerifyPagesAccess("tok", "nope"); err == nil {
		t.Fatal("expected an error for an account the token can't reach")
	}
}
