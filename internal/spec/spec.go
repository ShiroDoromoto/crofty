// Package spec is the crofty Markdown contract (spec v0) expressed as code: the
// rules `crofty validate` checks a content file against. Output is neutral and
// source-agnostic — the same findings can be read and fixed by hand or handed to
// any assistant; the product never assumes a particular tool.
package spec

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Severity ranks a finding. Only Error blocks build/publish.
type Severity string

const (
	SeverityError Severity = "error" // spec violation; blocks build/publish
	SeverityWarn  Severity = "warn"  // recommended, non-blocking
	SeverityInfo  Severity = "info"
)

// Issue is one finding about one field. The JSON shape matches spec v0 §7-2.
type Issue struct {
	Field    string   `json:"field"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Got      any      `json:"got"`
	Expected string   `json:"expected"`
	FixHint  string   `json:"fixHint"`
}

func issue(field, rule string, sev Severity, got any, expected, fix string) Issue {
	return Issue{Field: field, Rule: rule, Severity: sev, Got: got, Expected: expected, FixHint: fix}
}

// PageKind scopes rules that only apply to dated entries. A post must carry a
// date; section (_index.md) and standalone pages (e.g. about.md) need not.
type PageKind int

const (
	KindPost    PageKind = iota // dated entry; date required
	KindPage                    // standalone page; date optional
	KindSection                 // _index.md; date optional
)

// v0 vocabularies (spec §3-2 / §7-3).
var (
	allowedTiers    = []string{"full", "summary-only"}
	allowedTargets  = []string{"bluesky", "mastodon"}
	allowedExposure = []string{"open", "reserved"}

	slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	ulidRe = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`) // Crockford base32, 26 chars
)

// Validate checks parsed front matter of a file of the given kind against spec
// v0 and returns the findings in field order.
func Validate(fm Frontmatter, kind PageKind) []Issue {
	var issues []Issue
	add := func(i Issue) { issues = append(issues, i) }

	// title — required, non-empty string.
	switch v, ok := fm["title"]; {
	case !ok || v == nil:
		add(issue("title", "required", SeverityError, nil, "non-empty string",
			`Add 'title: "..."' to the frontmatter.`))
	default:
		if s, isStr := v.(string); !isStr {
			add(issue("title", "type", SeverityError, v, "non-empty string",
				`Set 'title' to a quoted string, e.g. title: "My post".`))
		} else if strings.TrimSpace(s) == "" {
			add(issue("title", "required", SeverityError, s, "non-empty string",
				`'title' is empty — give it a value.`))
		}
	}

	// date — required for posts; when present anywhere, must be a valid date.
	switch v, ok := fm["date"]; {
	case (!ok || v == nil) && kind == KindPost:
		add(issue("date", "required", SeverityError, nil, "a date (YYYY-MM-DD or RFC3339)",
			`Add 'date: YYYY-MM-DD' to the frontmatter.`))
	case ok && v != nil && !isValidDate(v):
		add(issue("date", "format", SeverityError, v, "a date (YYYY-MM-DD or RFC3339)",
			`Use 'date: 2026-06-14' or a full RFC3339 timestamp.`))
	}

	// description — optional; recommended (warn) when missing.
	switch v, ok := fm["description"]; {
	case !ok || v == nil || isEmptyString(v):
		add(issue("description", "recommended", SeverityWarn, nil, "a 1–2 sentence summary",
			`Optional — it falls back to the start of your body. Add 'description:' for cleaner link previews and RSS.`))
	default:
		if _, isStr := v.(string); !isStr {
			add(issue("description", "type", SeverityError, v, "a string",
				`Set 'description' to a quoted string.`))
		}
	}

	// slug — when present, lowercase and URL-safe.
	if v, ok := fm["slug"]; ok && v != nil {
		if s, isStr := v.(string); !isStr {
			add(issue("slug", "type", SeverityError, v, "a lowercase, URL-safe string",
				`Set 'slug' to a string like morning-walk-dog.`))
		} else if !slugRe.MatchString(s) {
			add(issue("slug", "format", SeverityError, s, "lowercase letters, digits and hyphens",
				`Use only a-z, 0-9 and hyphens, e.g. morning-walk-dog.`))
		}
	}

	// tags — when present, a list of strings.
	if v, ok := fm["tags"]; ok && v != nil {
		if !isStringSlice(v) {
			add(issue("tags", "type", SeverityError, v, "a list of strings",
				`Write tags as a list, e.g. tags: ["diary", "walk"].`))
		}
	}

	// crofty.* — when present, a map of known settings.
	if v, ok := fm["crofty"]; ok && v != nil {
		if cm, isMap := toStringMap(v); !isMap {
			add(issue("crofty", "type", SeverityError, v, "a map of crofty settings",
				`Nest crofty settings under a 'crofty:' map.`))
		} else {
			validateCrofty(cm, add)
		}
	}

	return issues
}

func validateCrofty(cm map[string]any, add func(Issue)) {
	if v, ok := cm["tier"]; ok && v != nil && !isOneOf(v, allowedTiers) {
		add(issue("crofty.tier", "enum", SeverityError, v, "full | summary-only",
			`Set crofty.tier to 'full' or 'summary-only'.`))
	}

	if v, ok := cm["targets"]; ok && v != nil {
		if bad, isList := unknownInSet(v, allowedTargets); !isList {
			add(issue("crofty.targets", "type", SeverityError, v, "a list of channel names",
				`Write targets as a list, e.g. targets: [bluesky].`))
		} else if len(bad) > 0 {
			add(issue("crofty.targets", "enum", SeverityError, bad, "only: bluesky, mastodon",
				`Remove unknown channels; v0 supports bluesky and mastodon.`))
		}
	}

	if v, ok := cm["ai_exposure"]; ok && v != nil && !isOneOf(v, allowedExposure) {
		add(issue("crofty.ai_exposure", "enum", SeverityError, v, "open | reserved",
			`Set crofty.ai_exposure to 'open' or 'reserved'.`))
	}

	if v, ok := cm["visibility"]; ok && v != nil {
		switch s, _ := v.(string); s {
		case "public":
			// ok
		case "members":
			add(issue("crofty.visibility", "unsupported", SeverityError, v, "public",
				`Member-gated posts aren't supported yet — remove this or set it to 'public'.`))
		default:
			add(issue("crofty.visibility", "enum", SeverityError, v, "public",
				`v0 supports only 'public'.`))
		}
	}

	if v, ok := cm["id"]; ok && v != nil {
		if s, isStr := v.(string); !isStr || !ulidRe.MatchString(strings.ToUpper(s)) {
			add(issue("crofty.id", "format", SeverityError, v, "a ULID (26 characters)",
				`Leave crofty.id out — crofty assigns it on first publish — or restore the original value.`))
		}
	}
}

// --- helpers -------------------------------------------------------------

func isValidDate(v any) bool {
	switch t := v.(type) {
	case time.Time:
		return !t.IsZero()
	case string:
		for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
			if _, err := time.Parse(layout, t); err == nil {
				return true
			}
		}
	}
	return false
}

func isEmptyString(v any) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

func isStringSlice(v any) bool {
	s, ok := v.([]any)
	if !ok {
		return false
	}
	for _, e := range s {
		if _, ok := e.(string); !ok {
			return false
		}
	}
	return true
}

func isOneOf(v any, set []string) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, x := range set {
		if s == x {
			return true
		}
	}
	return false
}

// unknownInSet reports list members not in set. isList is false when v is not a
// list of strings at all.
func unknownInSet(v any, set []string) (unknown []string, isList bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	for _, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		found := false
		for _, x := range set {
			if s == x {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, s)
		}
	}
	return unknown, true
}

func toStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case Frontmatter: // yaml.v3 decodes nested maps into the outer named map type
		return map[string]any(m), true
	case map[any]any: // defensive; yaml.v3 normally yields map[string]any
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	}
	return nil, false
}
