package publish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyBluesky(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/com.atproto.server.createSession" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var body struct {
			Identifier string `json:"identifier"`
			Password   string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Password != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"AuthenticationRequired","message":"Invalid identifier or password"}`))
			return
		}
		_, _ = w.Write([]byte(`{"accessJwt":"jwt","did":"did:plc:abc","handle":"x.bsky.social"}`))
	}))
	defer srv.Close()

	if err := VerifyBluesky(BlueskyCreds{Server: srv.URL, Handle: "x.bsky.social", AppPassword: "good"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if err := VerifyBluesky(BlueskyCreds{Server: srv.URL, Handle: "x.bsky.social", AppPassword: "bad"}); err == nil {
		t.Fatal("expected failure for a bad password")
	}
}
