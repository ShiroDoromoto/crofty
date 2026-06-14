package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
