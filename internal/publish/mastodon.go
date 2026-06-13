package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MastodonCreds are the user's own Mastodon credentials, read from the keychain.
// Unlike Bluesky there is no shared default instance — the user owns an account
// on a specific server, so Server is required.
type MastodonCreds struct {
	Server      string // instance base URL, e.g. https://mastodon.social
	Handle      string // non-secret account identifier, e.g. @you@mastodon.social
	AccessToken string
}

func (c MastodonCreds) server() (string, error) {
	s := strings.TrimRight(strings.TrimSpace(c.Server), "/")
	if s == "" {
		return "", fmt.Errorf("mastodon: an instance URL is required (e.g. https://mastodon.social)")
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	return s, nil
}

// mastodonAccount is the subset of the verify_credentials response we use.
type mastodonAccount struct {
	Username string `json:"username"`
	Acct     string `json:"acct"`
	URL      string `json:"url"`
}

// mastodonVerify confirms the token by reading the account it belongs to.
func mastodonVerify(ctx context.Context, c MastodonCreds) (*mastodonAccount, error) {
	server, err := c.server()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		server+"/api/v1/accounts/verify_credentials", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError("mastodon", resp)
	}
	var a mastodonAccount
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return nil, err
	}
	if a.Username == "" {
		return nil, fmt.Errorf("mastodon: empty account response")
	}
	return &a, nil
}

// VerifyMastodon checks credentials by reading the account they belong to.
func VerifyMastodon(c MastodonCreds) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := mastodonVerify(ctx, c)
	return err
}
