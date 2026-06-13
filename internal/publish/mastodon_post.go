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

// mastodonURLWeight is how many characters Mastodon counts for any URL in a
// status, regardless of its real length (matching the server's own counting).
const mastodonURLWeight = 23

// Mastodon is a Publisher for a Mastodon account.
type Mastodon struct {
	name  string
	creds MastodonCreds
}

// NewMastodon returns a Mastodon publisher named name using the given credentials.
func NewMastodon(name string, creds MastodonCreds) *Mastodon {
	return &Mastodon{name: name, creds: creds}
}

func (m *Mastodon) Name() string { return m.name }

// Capabilities for Mastodon: 500-char statuses by default (instances may differ;
// we use the common default since Plan must not hit the network), editable after
// posting, and a link card auto-generated from a URL in the text.
func (m *Mastodon) Capabilities() Capabilities {
	return Capabilities{MaxChars: 500, Editable: true, LinkCard: true}
}

// Plan builds the status text from the fragment. Mastodon renders the link card
// from a URL in the text itself, so the canonical link is appended to the body
// of the status (counted as a fixed weight). No network, no side effects.
func (m *Mastodon) Plan(f Fragment) (Preview, error) {
	link := strings.TrimSpace(f.CanonicalURL)
	if link == "" {
		return Preview{}, fmt.Errorf("mastodon: a canonical URL is required")
	}
	summary := strings.TrimSpace(f.Summary)
	if summary == "" {
		summary = strings.TrimSpace(f.Title)
	}

	var notes []string
	if max := m.Capabilities().MaxChars; max > 0 {
		// The link costs a fixed weight plus the blank line before it.
		budget := max - mastodonURLWeight - 2
		if budget < 0 {
			budget = 0
		}
		if r := []rune(summary); len(r) > budget {
			summary = strings.TrimSpace(string(r[:budget-1])) + "…"
			notes = append(notes, fmt.Sprintf("post text trimmed to fit Mastodon's %d-character limit", max))
		}
	}

	text := link
	if summary != "" {
		text = summary + "\n\n" + link
	}
	return Preview{Target: m.name, Text: text, Frag: f, Notes: notes}, nil
}

// Execute posts the status. Only the fragment-derived text (which carries the
// canonical link for the card) is sent — never a body.
func (m *Mastodon) Execute(p Preview) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server, err := m.creds.server()
	if err != nil {
		return Result{}, err
	}

	body, _ := json.Marshal(map[string]any{"status": p.Text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server+"/api/v1/statuses", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.creds.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, apiError("mastodon", resp)
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, err
	}
	if out.URL == "" {
		return Result{}, fmt.Errorf("mastodon: status posted but no URL returned")
	}
	return Result{Target: m.name, PostID: out.ID, URL: out.URL, PublishedAt: time.Now().UTC()}, nil
}
