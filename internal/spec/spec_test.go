package spec

import (
	"testing"
	"time"
)

// hasIssue reports whether issues contains one for field with the given severity.
func hasIssue(issues []Issue, field string, sev Severity) bool {
	for _, i := range issues {
		if i.Field == field && i.Severity == sev {
			return true
		}
	}
	return false
}

func TestValidate_ValidPost(t *testing.T) {
	fm := Frontmatter{
		"title":       "Morning walk",
		"date":        time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		"description": "A short note.",
		"tags":        []any{"diary", "walk"},
		"slug":        "morning-walk-dog",
		"crofty": map[string]any{
			"tier":        "full",
			"targets":     []any{"bluesky", "mastodon"},
			"ai_exposure": "open",
			"id":          "01J9X8Z7Q2H4M5N6P7R8S9T0AB",
		},
	}
	if got := Validate(fm, KindPost); len(got) != 0 {
		t.Fatalf("expected no issues, got %+v", got)
	}
}

func TestValidate_MissingTitleAndDate(t *testing.T) {
	got := Validate(Frontmatter{}, KindPost)
	if !hasIssue(got, "title", SeverityError) {
		t.Errorf("missing title should be an error: %+v", got)
	}
	if !hasIssue(got, "date", SeverityError) {
		t.Errorf("missing date should be an error on a post: %+v", got)
	}
	if !hasIssue(got, "description", SeverityWarn) {
		t.Errorf("missing description should be a warning: %+v", got)
	}
}

func TestValidate_DateOptionalForNonPosts(t *testing.T) {
	fm := Frontmatter{"title": "Crofty", "description": "x"}
	for _, kind := range []PageKind{KindSection, KindPage} {
		if hasIssue(Validate(fm, kind), "date", SeverityError) {
			t.Errorf("date should be optional for kind %d", kind)
		}
	}
	if !hasIssue(Validate(fm, KindPost), "date", SeverityError) {
		t.Errorf("date should be required for a post")
	}
}

func TestValidate_EnumsAndFormats(t *testing.T) {
	cases := []struct {
		name  string
		fm    Frontmatter
		field string
	}{
		{"bad slug", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "slug": "Not Safe"}, "slug"},
		{"non-string tags", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "tags": []any{1, 2}}, "tags"},
		{"bad tier", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "crofty": map[string]any{"tier": "half"}}, "crofty.tier"},
		{"unknown target", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "crofty": map[string]any{"targets": []any{"twitter"}}}, "crofty.targets"},
		{"bad exposure", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "crofty": map[string]any{"ai_exposure": "maybe"}}, "crofty.ai_exposure"},
		{"members visibility", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "crofty": map[string]any{"visibility": "members"}}, "crofty.visibility"},
		{"bad ulid", Frontmatter{"title": "t", "date": "2026-06-14", "description": "d", "crofty": map[string]any{"id": "not-a-ulid"}}, "crofty.id"},
		{"bad date", Frontmatter{"title": "t", "date": "yesterday", "description": "d"}, "date"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !hasIssue(Validate(c.fm, KindPost), c.field, SeverityError) {
				t.Errorf("expected error on %s, got %+v", c.field, Validate(c.fm, KindPost))
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	src := []byte("---\ntitle: \"Hi\"\ncrofty:\n  tier: full\n---\n\nbody text\n")
	fm, err := parseFrontmatter(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if fm["title"] != "Hi" {
		t.Errorf("title not parsed: %+v", fm)
	}
	if _, ok := toStringMap(fm["crofty"]); !ok {
		t.Errorf("crofty block should be recognized as a map, got %T", fm["crofty"])
	}

	if _, err := parseFrontmatter([]byte("no frontmatter here")); err != ErrNoFrontmatter {
		t.Errorf("expected ErrNoFrontmatter, got %v", err)
	}
}

// TestValidate_NestedCroftyFromYAML exercises the real parse→validate path: the
// crofty: block decodes to the named map type, which the rules must still read.
func TestValidate_NestedCroftyFromYAML(t *testing.T) {
	src := []byte("---\ntitle: \"t\"\ndate: 2026-06-14\ndescription: \"d\"\ncrofty:\n  tier: half\n---\nbody\n")
	fm, err := parseFrontmatter(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !hasIssue(Validate(fm, KindPost), "crofty.tier", SeverityError) {
		t.Errorf("bad crofty.tier from YAML should be flagged, got %+v", Validate(fm, KindPost))
	}
}
