package publish

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyMastodon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/accounts/verify_credentials" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"The access token is invalid"}`))
			return
		}
		_, _ = w.Write([]byte(`{"username":"me","acct":"me","url":"https://mastodon.social/@me"}`))
	}))
	defer srv.Close()

	if err := VerifyMastodon(MastodonCreds{Server: srv.URL, Handle: "@me@mastodon.social", AccessToken: "good"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if err := VerifyMastodon(MastodonCreds{Server: srv.URL, Handle: "@me@mastodon.social", AccessToken: "bad"}); err == nil {
		t.Fatal("expected failure for a bad token")
	}
	if err := VerifyMastodon(MastodonCreds{AccessToken: "good"}); err == nil {
		t.Fatal("expected failure when the instance URL is missing")
	}
}
