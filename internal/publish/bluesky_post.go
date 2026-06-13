package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Bluesky is a Publisher for a Bluesky account (AT Protocol).
type Bluesky struct {
	name  string
	creds BlueskyCreds
}

// NewBluesky returns a Bluesky publisher named name using the given credentials.
func NewBluesky(name string, creds BlueskyCreds) *Bluesky {
	return &Bluesky{name: name, creds: creds}
}

func (b *Bluesky) Name() string { return b.name }

// Capabilities for Bluesky: 300-char posts, not editable once posted, renders a
// link card from an external embed.
func (b *Bluesky) Capabilities() Capabilities {
	return Capabilities{MaxChars: 300, Editable: false, LinkCard: true}
}

// Plan builds the post text from the fragment and adapts it to Bluesky's limit.
// No network, no side effects.
func (b *Bluesky) Plan(f Fragment) (Preview, error) {
	if strings.TrimSpace(f.CanonicalURL) == "" {
		return Preview{}, fmt.Errorf("bluesky: a canonical URL is required")
	}
	text := strings.TrimSpace(f.Summary)
	if text == "" {
		text = strings.TrimSpace(f.Title)
	}
	var notes []string
	if max := b.Capabilities().MaxChars; max > 0 {
		if r := []rune(text); len(r) > max {
			text = strings.TrimSpace(string(r[:max-1])) + "…"
			notes = append(notes, fmt.Sprintf("post text trimmed to %d characters for Bluesky", max))
		}
	}
	return Preview{Target: b.name, Text: text, Frag: f, Notes: notes}, nil
}

// Execute signs in and creates the post with an external link card pointing at
// the canonical URL. Only the fragment is sent — never a body.
func (b *Bluesky) Execute(p Preview) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := blueskyCreateSession(ctx, b.creds)
	if err != nil {
		return Result{}, err
	}

	external := map[string]any{
		"uri":         p.Frag.CanonicalURL,
		"title":       p.Frag.Title,
		"description": p.Frag.Summary,
	}
	if p.Frag.ImageURL != "" {
		// Best-effort thumbnail; a fetch/upload failure still posts the card.
		if thumb, err := b.uploadBlob(ctx, sess.AccessJwt, p.Frag.ImageURL); err == nil && thumb != nil {
			external["thumb"] = thumb
		}
	}

	record := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      p.Text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"embed": map[string]any{
			"$type":    "app.bsky.embed.external",
			"external": external,
		},
	}
	body, _ := json.Marshal(map[string]any{
		"repo":       sess.Did,
		"collection": "app.bsky.feed.post",
		"record":     record,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.creds.server()+"/xrpc/com.atproto.repo.createRecord", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sess.AccessJwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, apiError("bluesky", resp)
	}
	var out struct {
		URI string `json:"uri"`
		Cid string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, err
	}

	rkey := out.URI[strings.LastIndex(out.URI, "/")+1:]
	postURL := fmt.Sprintf("https://bsky.app/profile/%s/post/%s", b.creds.Handle, rkey)
	return Result{Target: b.name, PostID: out.URI, URL: postURL, PublishedAt: time.Now().UTC()}, nil
}

// uploadBlob fetches imageURL and uploads it, returning the blob ref to embed as
// a card thumbnail. Best-effort: the caller ignores errors and posts without it.
func (b *Bluesky) uploadBlob(ctx context.Context, accessJwt, imageURL string) (any, error) {
	greq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	gresp, err := http.DefaultClient.Do(greq)
	if err != nil {
		return nil, err
	}
	defer gresp.Body.Close()
	if gresp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch image: %s", gresp.Status)
	}
	const maxBlob = 1 << 20 // 1 MiB (Bluesky blob limit)
	data, err := io.ReadAll(io.LimitReader(gresp.Body, maxBlob+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBlob {
		return nil, fmt.Errorf("image too large for a Bluesky thumbnail")
	}
	mime := gresp.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.creds.server()+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mime)
	req.Header.Set("Authorization", "Bearer "+accessJwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError("bluesky", resp)
	}
	var out struct {
		Blob json.RawMessage `json:"blob"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Blob) == 0 {
		return nil, fmt.Errorf("empty blob response")
	}
	var blob any
	if err := json.Unmarshal(out.Blob, &blob); err != nil {
		return nil, err
	}
	return blob, nil
}
