package publish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBluesky_PlanAndExecute(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_, _ = w.Write([]byte(`{"accessJwt":"jwt","did":"did:plc:abc","handle":"me.bsky.social"}`))
		case "/xrpc/com.atproto.repo.createRecord":
			if r.Header.Get("Authorization") != "Bearer jwt" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var body struct {
				Record map[string]any `json:"record"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			posted = body.Record
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:abc/app.bsky.feed.post/3kabc","cid":"bafyxyz"}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	bp := NewBluesky("bluesky", BlueskyCreds{Server: srv.URL, Handle: "me.bsky.social", AppPassword: "x"})
	frag := Fragment{Title: "Hello", Summary: "A short summary.", CanonicalURL: "https://example.com/posts/hello/"}

	prev, err := bp.Plan(frag)
	if err != nil {
		t.Fatal(err)
	}
	if prev.Text != "A short summary." {
		t.Fatalf("planned text = %q", prev.Text)
	}

	res, err := bp.Execute(prev)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://bsky.app/profile/me.bsky.social/post/3kabc"; res.URL != want {
		t.Errorf("post URL = %q, want %q", res.URL, want)
	}
	if res.PostID != "at://did:plc:abc/app.bsky.feed.post/3kabc" {
		t.Errorf("post id = %q", res.PostID)
	}

	// The posted record must carry the fragment as a card and never a body.
	if posted["text"] != "A short summary." {
		t.Errorf("posted text = %v", posted["text"])
	}
	ext := posted["embed"].(map[string]any)["external"].(map[string]any)
	if ext["uri"] != "https://example.com/posts/hello/" {
		t.Errorf("card uri = %v", ext["uri"])
	}
	if _, hasBody := posted["body"]; hasBody {
		t.Error("posted record must not contain a body")
	}
}

func TestBluesky_PlanTrimsByRune(t *testing.T) {
	bp := NewBluesky("bluesky", BlueskyCreds{})
	long := strings.Repeat("あ", 400) // multibyte; must trim by rune, not byte
	prev, err := bp.Plan(Fragment{Title: "t", Summary: long, CanonicalURL: "https://x/"})
	if err != nil {
		t.Fatal(err)
	}
	if r := []rune(prev.Text); len(r) != 300 {
		t.Errorf("expected 300 runes, got %d", len(r))
	}
	if len(prev.Notes) == 0 {
		t.Error("expected a trim note")
	}
}
