// Package publish syndicates a post's fragment (title + description + canonical
// link) to the user's own destinations. It never receives the post body — the
// asset/fragment line (spec 06 §1) is enforced by the function signatures, not
// by policy. All network calls go only to destinations the user chose (A7).
package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultBlueskyServer = "https://bsky.social"

// BlueskyCreds are the user's own Bluesky credentials, read from the keychain.
type BlueskyCreds struct {
	Server      string // PDS base URL; defaults to https://bsky.social
	Handle      string
	AppPassword string
}

func (c BlueskyCreds) server() string {
	if strings.TrimSpace(c.Server) == "" {
		return defaultBlueskyServer
	}
	return strings.TrimRight(c.Server, "/")
}

type blueskySession struct {
	AccessJwt string `json:"accessJwt"`
	Did       string `json:"did"`
	Handle    string `json:"handle"`
}

// blueskyCreateSession authenticates and returns a session (access JWT + DID).
func blueskyCreateSession(ctx context.Context, c BlueskyCreds) (*blueskySession, error) {
	body, _ := json.Marshal(map[string]string{
		"identifier": c.Handle,
		"password":   c.AppPassword,
	})
	url := c.server() + "/xrpc/com.atproto.server.createSession"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, apiError("bluesky", resp)
	}
	var s blueskySession
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	if s.AccessJwt == "" {
		return nil, fmt.Errorf("bluesky: empty session response")
	}
	return &s, nil
}

// VerifyBluesky checks credentials by creating (and discarding) a session.
func VerifyBluesky(c BlueskyCreds) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := blueskyCreateSession(ctx, c)
	return err
}

// apiError extracts a human-readable error from an XRPC/REST error response.
func apiError(svc string, resp *http.Response) error {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	switch {
	case e.Message != "":
		return fmt.Errorf("%s: %s (%s)", svc, e.Message, resp.Status)
	case e.Error != "":
		return fmt.Errorf("%s: %s (%s)", svc, e.Error, resp.Status)
	default:
		return fmt.Errorf("%s: unexpected response %s", svc, resp.Status)
	}
}
