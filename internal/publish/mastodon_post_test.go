package publish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMastodon_PlanAndExecute(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/statuses" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		_, _ = w.Write([]byte(`{"id":"12345","url":"https://mastodon.social/@me/12345"}`))
	}))
	defer srv.Close()

	mp := NewMastodon("mastodon", MastodonCreds{Server: srv.URL, Handle: "@me@mastodon.social", AccessToken: "tok"})
	frag := Fragment{Title: "Hello", Summary: "A short summary.", CanonicalURL: "https://example.com/posts/hello/"}

	prev, err := mp.Plan(frag)
	if err != nil {
		t.Fatal(err)
	}
	// The canonical link must be in the status text so Mastodon builds the card.
	if !strings.Contains(prev.Text, "https://example.com/posts/hello/") {
		t.Fatalf("planned text missing canonical link: %q", prev.Text)
	}
	if !strings.HasPrefix(prev.Text, "A short summary.") {
		t.Fatalf("planned text = %q", prev.Text)
	}

	res, err := mp.Execute(prev)
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != "https://mastodon.social/@me/12345" {
		t.Errorf("post URL = %q", res.URL)
	}
	if res.PostID != "12345" {
		t.Errorf("post id = %q", res.PostID)
	}

	// The posted status carries the link text and never a body field.
	if s, _ := posted["status"].(string); !strings.Contains(s, "https://example.com/posts/hello/") {
		t.Errorf("posted status = %v", posted["status"])
	}
	if _, hasBody := posted["body"]; hasBody {
		t.Error("posted status must not contain a body")
	}
}

func TestMastodon_PlanTrimsByRune(t *testing.T) {
	mp := NewMastodon("mastodon", MastodonCreds{Server: "https://x"})
	long := strings.Repeat("あ", 600) // multibyte; must trim by rune, not byte
	prev, err := mp.Plan(Fragment{Title: "t", Summary: long, CanonicalURL: "https://x/p/"})
	if err != nil {
		t.Fatal(err)
	}
	// Summary is trimmed so the whole status (summary + blank line + link weight)
	// fits within the 500-char limit.
	if r := []rune(prev.Text); len(r) > 500 {
		t.Errorf("status exceeds limit: %d runes", len(r))
	}
	if len(prev.Notes) == 0 {
		t.Error("expected a trim note")
	}
	if !strings.HasSuffix(prev.Text, "https://x/p/") {
		t.Errorf("link must remain at the end: %q", prev.Text)
	}
}

func TestMastodon_PlanRequiresCanonicalURL(t *testing.T) {
	mp := NewMastodon("mastodon", MastodonCreds{Server: "https://x"})
	if _, err := mp.Plan(Fragment{Title: "t", Summary: "s"}); err == nil {
		t.Fatal("expected an error when the canonical URL is missing")
	}
}
