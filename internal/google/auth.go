// Package google is crofty's thin client for the Google APIs that answer "who
// reached my site?" — GA4 (the Data API: who visited, what they read) and Search
// Console (queries, pages, sitemaps). It talks to those APIs directly over HTTP
// with a service-account JWT, the same no-SDK discipline crofty uses for the
// Cloudflare API, so the single binary stays lean and pulls in no Google SDK.
//
// Auth is two-legged OAuth: sign a short-lived RS256 JWT with the service
// account's private key, exchange it at the token endpoint for an access token,
// and send that as a bearer. All of it is stdlib crypto — no new dependency.
package google

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth scopes crofty asks for, narrowest-first. Reads never request write.
const (
	ScopeAnalyticsRead = "https://www.googleapis.com/auth/analytics.readonly"
	ScopeSearchRead    = "https://www.googleapis.com/auth/webmasters.readonly"
	ScopeSearchWrite   = "https://www.googleapis.com/auth/webmasters" // submit-sitemap only
)

// Credentials is the subset of a service-account JSON key crofty needs. The
// private key is parsed once at construction; ClientEmail and ProjectID are
// surfaced so the CLI can print copy-pasteable setup guidance ("add this email
// as a Viewer", "enable the API in this project").
type Credentials struct {
	ClientEmail string
	ProjectID   string
	TokenURI    string

	key *rsa.PrivateKey
}

// saKeyFile mirrors the fields of a Google service-account JSON key.
type saKeyFile struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// ParseCredentials reads a service-account JSON key (the file Google hands you
// when you create a key on a service account) and parses its RSA private key.
func ParseCredentials(saJSON []byte) (*Credentials, error) {
	var f saKeyFile
	if err := json.Unmarshal(saJSON, &f); err != nil {
		return nil, fmt.Errorf("this doesn't look like a service-account JSON key: %w", err)
	}
	if f.ClientEmail == "" || f.PrivateKey == "" {
		return nil, fmt.Errorf("the service-account key is missing client_email or private_key — re-download it from the GCP console")
	}
	key, err := parsePrivateKey(f.PrivateKey)
	if err != nil {
		return nil, err
	}
	tokenURI := f.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	return &Credentials{
		ClientEmail: f.ClientEmail,
		ProjectID:   f.ProjectID,
		TokenURI:    tokenURI,
		key:         key,
	}, nil
}

// parsePrivateKey decodes the PEM-wrapped PKCS#8 RSA key Google embeds in the
// JSON (the "-----BEGIN PRIVATE KEY-----" blob).
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("could not decode the PEM private key in the service-account file")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, fmt.Errorf("the service-account private key is not an RSA key")
	}
	// Older keys may be PKCS#1.
	if rk, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rk, nil
	}
	return nil, fmt.Errorf("could not parse the service-account private key")
}

// accessToken signs a JWT for the requested scope and exchanges it for an OAuth
// access token. now is injected so the JWT's iat/exp are deterministic in tests.
func (c *Credentials) accessToken(client *http.Client, scope string, now time.Time) (string, error) {
	assertion, err := c.signJWT(scope, now)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	resp, err := client.PostForm(c.TokenURI, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("token endpoint returned an unreadable response: %w", err)
	}
	if out.AccessToken == "" {
		if out.Error != "" {
			return "", fmt.Errorf("token exchange failed: %s (%s)", out.Error, out.ErrorDescription)
		}
		return "", fmt.Errorf("token exchange returned no access token")
	}
	return out.AccessToken, nil
}

// signJWT builds and RS256-signs the assertion Google's token endpoint expects.
func (c *Credentials) signJWT(scope string, now time.Time) (string, error) {
	header := b64url([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]any{
		"iss":   c.ClientEmail,
		"scope": scope,
		"aud":   c.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := header + "." + b64url(cb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + b64url(sig), nil
}

func b64url(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}
