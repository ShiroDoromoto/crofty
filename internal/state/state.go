// Package state persists what has been published where, so publish is
// idempotent and can retry only what failed (A4). It is plain JSON in .crofty/,
// excluded from deploy and never holding secrets — losing it never loses
// content. Records are keyed by the post's stable crofty.id, so renames and slug
// changes don't break the publish history.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is the whole publish ledger.
type State struct {
	Posts map[string]PostState `json:"posts"`
}

// PostState holds one post's publish records, keyed by target name.
type PostState struct {
	Publishes map[string]PublishRecord `json:"publishes"`
}

// PublishRecord is the outcome of one successful publish to one channel.
type PublishRecord struct {
	PostID          string    `json:"postId"`
	URL             string    `json:"url"`
	PublishedAt     time.Time `json:"publishedAt"`
	DescriptionHash string    `json:"descriptionHash"`
}

// Load reads the ledger, returning an empty one if the file does not exist.
func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{Posts: map[string]PostState{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Posts == nil {
		s.Posts = map[string]PostState{}
	}
	return &s, nil
}

// Save writes the ledger, creating the directory if needed.
func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Get returns the publish record for a post id + target, if any.
func (s *State) Get(postID, target string) (PublishRecord, bool) {
	p, ok := s.Posts[postID]
	if !ok {
		return PublishRecord{}, false
	}
	r, ok := p.Publishes[target]
	return r, ok
}

// Record stores (or replaces) a publish record for a post id + target.
func (s *State) Record(postID, target string, r PublishRecord) {
	p, ok := s.Posts[postID]
	if !ok || p.Publishes == nil {
		p = PostState{Publishes: map[string]PublishRecord{}}
	}
	p.Publishes[target] = r
	s.Posts[postID] = p
}

// Hash returns a stable fingerprint of the syndicated fragment, used to detect
// whether a post's shared content changed since it was last published.
func Hash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
