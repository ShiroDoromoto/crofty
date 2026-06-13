package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/shirodoromoto/crofty/internal/secret"
)

// TestPublish_EndToEndMock drives the whole publish flow against a mock Bluesky
// PDS: id write-back, state recording, and idempotency (no double-post).
func TestPublish_EndToEndMock(t *testing.T) {
	keyring.MockInit()
	stubLive(t, liveYes)

	var createRecordCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_, _ = w.Write([]byte(`{"accessJwt":"jwt","did":"did:plc:abc","handle":"me.bsky.social"}`))
		case "/xrpc/com.atproto.repo.createRecord":
			createRecordCalls++
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:abc/app.bsky.feed.post/3kabc","cid":"baf"}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	write(t, filepath.Join(root, "hugo.yaml"), "baseURL: \"https://example.com/\"\ntitle: T\n")
	mkdir(t, filepath.Join(root, ".crofty"))
	write(t, filepath.Join(root, ".crofty", "config.json"),
		`{"workspace":"WS","deploy":{"provider":"cloudflare","project":"x"},`+
			`"targets":{"bluesky":{"type":"bluesky","handle":"me.bsky.social","server":"`+srv.URL+`"}}}`)
	postDir := filepath.Join(root, "content", "posts", "hello")
	mkdir(t, postDir)
	article := filepath.Join(postDir, "index.md")
	write(t, article, "---\ntitle: \"Hi\"\ndate: 2026-06-14\ndescription: \"A summary.\"\nslug: hi\ncrofty:\n  targets: [bluesky]\n---\nBody stays home.\n")

	if err := secret.New("WS").Set("bluesky", "app_password", "pw"); err != nil {
		t.Fatal(err)
	}

	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runPublish([]string{"--yes", "content/posts/hello/index.md"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if createRecordCalls != 1 {
		t.Fatalf("expected 1 createRecord call, got %d", createRecordCalls)
	}

	// crofty.id written back into the post.
	if b := read(t, article); !strings.Contains(b, "id:") {
		t.Errorf("crofty.id not written back:\n%s", b)
	}
	// state.json records the post URL.
	if st := read(t, filepath.Join(root, ".crofty", "state.json")); !strings.Contains(st, "bsky.app/profile") {
		t.Errorf("state missing post url:\n%s", st)
	}

	// Second run is idempotent — unchanged fragment must not post again.
	if err := runPublish([]string{"--yes", "content/posts/hello/index.md"}); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if createRecordCalls != 1 {
		t.Errorf("idempotency broken: createRecord called %d times", createRecordCalls)
	}
}

// TestPublish_DeployGuardBlocksUndeployed verifies that a --yes run refuses to
// syndicate a post that is not live on the site, and that --skip-deploy-check
// overrides the guard.
func TestPublish_DeployGuardBlocksUndeployed(t *testing.T) {
	keyring.MockInit()
	stubLive(t, liveNo)

	var createRecordCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_, _ = w.Write([]byte(`{"accessJwt":"jwt","did":"did:plc:abc","handle":"me.bsky.social"}`))
		case "/xrpc/com.atproto.repo.createRecord":
			createRecordCalls++
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:abc/app.bsky.feed.post/3kabc","cid":"baf"}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	write(t, filepath.Join(root, "hugo.yaml"), "baseURL: \"https://example.com/\"\ntitle: T\n")
	mkdir(t, filepath.Join(root, ".crofty"))
	write(t, filepath.Join(root, ".crofty", "config.json"),
		`{"workspace":"WS","deploy":{"provider":"cloudflare","project":"x"},`+
			`"targets":{"bluesky":{"type":"bluesky","handle":"me.bsky.social","server":"`+srv.URL+`"}}}`)
	postDir := filepath.Join(root, "content", "posts", "hello")
	mkdir(t, postDir)
	article := filepath.Join(postDir, "index.md")
	write(t, article, "---\ntitle: \"Hi\"\ndate: 2026-06-14\ndescription: \"A summary.\"\nslug: hi\ncrofty:\n  targets: [bluesky]\n---\nBody stays home.\n")

	if err := secret.New("WS").Set("bluesky", "app_password", "pw"); err != nil {
		t.Fatal(err)
	}

	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	// --yes must be blocked when the post is not live: nothing is sent.
	if err := runPublish([]string{"--yes", "content/posts/hello/index.md"}); err != errSilent {
		t.Fatalf("expected guard to block with errSilent, got %v", err)
	}
	if createRecordCalls != 0 {
		t.Fatalf("guard let a dead-link post through: %d createRecord calls", createRecordCalls)
	}

	// --skip-deploy-check overrides the guard and publishes.
	if err := runPublish([]string{"--yes", "--skip-deploy-check", "content/posts/hello/index.md"}); err != nil {
		t.Fatalf("skip-deploy-check publish: %v", err)
	}
	if createRecordCalls != 1 {
		t.Fatalf("expected 1 createRecord call after override, got %d", createRecordCalls)
	}
}

// stubLive replaces the liveness probe for the duration of a test.
func stubLive(t *testing.T, l liveness) {
	t.Helper()
	prev := checkLive
	checkLive = func(string) liveness { return l }
	t.Cleanup(func() { checkLive = prev })
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
