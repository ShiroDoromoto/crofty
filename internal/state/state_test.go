package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestState_RoundTripAndIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".crofty", "state.json")

	s, err := Load(path) // missing file → empty
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Posts) != 0 {
		t.Fatalf("expected empty state, got %+v", s)
	}

	const id = "01J9X8Z7Q2H4M5N6P7R8S9T0AB"
	rec := PublishRecord{
		PostID:          "at://did/app.bsky.feed.post/3k",
		URL:             "https://bsky.app/profile/me/post/3k",
		PublishedAt:     time.Unix(1, 0).UTC(),
		DescriptionHash: Hash("Title", "Summary"),
	}
	s.Record(id, "bluesky", rec)
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Get(id, "bluesky")
	if !ok {
		t.Fatal("record not found after reload")
	}
	if got.URL != rec.URL || got.DescriptionHash != rec.DescriptionHash {
		t.Errorf("reloaded record mismatch: %+v", got)
	}

	// Idempotency signal: same fragment → same hash; changed → different.
	if Hash("Title", "Summary") != rec.DescriptionHash {
		t.Error("hash should be stable for identical input")
	}
	if Hash("Title", "Summary 2") == rec.DescriptionHash {
		t.Error("hash should change when the fragment changes")
	}
}
