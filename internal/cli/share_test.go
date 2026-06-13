package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

func shareProject(t *testing.T, description string) (root, article string) {
	t.Helper()
	root = t.TempDir()
	write(t, filepath.Join(root, "hugo.yaml"), "baseURL: \"https://example.com/\"\ntitle: T\n")
	mkdir(t, filepath.Join(root, ".crofty"))
	postDir := filepath.Join(root, "content", "posts", "hello")
	mkdir(t, postDir)
	article = filepath.Join(postDir, "index.md")
	write(t, article, "---\ntitle: \"Hi\"\ndate: 2026-06-14\ndescription: \""+description+"\"\nslug: hi\ncrofty:\n  targets: [bluesky]\n---\nBody stays home.\n")
	return root, article
}

func TestShare_JSON(t *testing.T) {
	root, _ := shareProject(t, "A short summary.")
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error {
		return runShare([]string{"--json", "--to", "x,bluesky", "content/posts/hello/index.md"})
	})
	if err != nil {
		t.Fatalf("share: %v", err)
	}

	var got struct {
		Link     string `json:"link"`
		Channels []struct {
			Channel string `json:"channel"`
			Text    string `json:"text"`
			Intent  string `json:"intent"`
		} `json:"channels"`
		Plain string `json:"plain"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if got.Link != "https://example.com/posts/hi/" {
		t.Errorf("link = %q", got.Link)
	}
	if len(got.Channels) != 2 {
		t.Fatalf("want 2 channels, got %d", len(got.Channels))
	}
	byName := map[string]string{}
	for _, c := range got.Channels {
		byName[c.Channel] = c.Intent
		if !strings.Contains(c.Text, got.Link) {
			t.Errorf("%s text missing the canonical link: %q", c.Channel, c.Text)
		}
	}
	// X has a compose intent; Bluesky does not.
	if !strings.Contains(byName["x"], "twitter.com/intent/tweet") {
		t.Errorf("x intent = %q", byName["x"])
	}
	if byName["bluesky"] != "" {
		t.Errorf("bluesky should have no intent, got %q", byName["bluesky"])
	}
	if !strings.Contains(got.Plain, got.Link) {
		t.Errorf("plain missing link: %q", got.Plain)
	}
}

func TestShare_Plain(t *testing.T) {
	root, _ := shareProject(t, "A short summary.")
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error {
		return runShare([]string{"--plain", "content/posts/hello/index.md"})
	})
	if err != nil {
		t.Fatalf("share --plain: %v", err)
	}
	got := strings.TrimSpace(out)
	if got != "A short summary. https://example.com/posts/hi/" {
		t.Errorf("plain output = %q", got)
	}
}

func TestShare_TrimsToLimitKeepingLink(t *testing.T) {
	link := "https://example.com/posts/hi/"
	long := strings.Repeat("あ", 400)
	out, _ := composeShareText(long, link, 280)
	if r := []rune(out); len(r) > 280 {
		t.Errorf("composed text exceeds 280 runes: %d", len(r))
	}
	if !strings.HasSuffix(out, link) {
		t.Errorf("link must remain at the end: %q", out)
	}
}
