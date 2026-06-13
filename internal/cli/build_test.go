package cli

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFutureDatedPosts(t *testing.T) {
	root := t.TempDir()
	content := filepath.Join(root, "content")

	// A past post (included) and a future one (excluded by Hugo).
	pastDir := filepath.Join(content, "posts", "past")
	mkdir(t, pastDir)
	write(t, filepath.Join(pastDir, "index.md"),
		"---\ntitle: \"Past\"\ndate: 2026-06-14T01:00:00+09:00\n---\nbody\n")

	futDir := filepath.Join(content, "posts", "future")
	mkdir(t, futDir)
	write(t, filepath.Join(futDir, "index.md"),
		"---\ntitle: \"Future\"\ndate: 2026-06-14T02:00:00+09:00\n---\nbody\n")

	// "Now" sits between the two posts' dates.
	now, _ := time.Parse(time.RFC3339, "2026-06-14T01:30:00+09:00")

	got := futureDatedPosts(content, now)
	if len(got) != 1 {
		t.Fatalf("want 1 future-dated post, got %d: %+v", len(got), got)
	}
	if filepath.Base(filepath.Dir(got[0].path)) != "future" {
		t.Errorf("flagged the wrong post: %s", got[0].path)
	}
}

func TestFutureDatedPosts_NoneWhenAllPast(t *testing.T) {
	root := t.TempDir()
	content := filepath.Join(root, "content")
	dir := filepath.Join(content, "posts", "p")
	mkdir(t, dir)
	write(t, filepath.Join(dir, "index.md"),
		"---\ntitle: \"P\"\ndate: 2026-06-14T01:00:00+09:00\n---\nbody\n")

	now, _ := time.Parse(time.RFC3339, "2026-06-14T12:00:00+09:00")
	if got := futureDatedPosts(content, now); len(got) != 0 {
		t.Errorf("expected none, got %+v", got)
	}
}

func TestFutureDatedPosts_MissingContentDir(t *testing.T) {
	if got := futureDatedPosts(filepath.Join(t.TempDir(), "nope"), time.Now()); got != nil {
		t.Errorf("expected nil for a missing content dir, got %+v", got)
	}
}
