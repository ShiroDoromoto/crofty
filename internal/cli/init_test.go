package cli

import (
	"bufio"
	"strings"
	"testing"
)

func reader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

// chooseNames keeps the display title and the deploy/pages.dev slug separate.
// Cover flags, interactive prompts, and folder-derived defaults.
func TestChooseNames(t *testing.T) {
	cases := []struct {
		name        string
		abs         string
		titleFlag   string
		projectFlag string
		interactive bool
		input       string
		wantTitle   string
		wantSlug    string
	}{
		{
			name:        "flags win, non-interactive (title may be free text)",
			abs:         "/x/whatever",
			titleFlag:   "example.com 技術ブログ",
			projectFlag: "example-blog",
			wantTitle:   "example.com 技術ブログ",
			wantSlug:    "example-blog",
		},
		{
			name:      "non-interactive, derive both from the folder",
			abs:       "/x/My Cool Blog",
			wantTitle: "My Cool Blog",
			wantSlug:  "my-cool-blog",
		},
		{
			name:        "interactive, both entered (project sanitised)",
			abs:         "/x/existing",
			interactive: true,
			input:       "My Awesome Site\nCool Blog\n",
			wantTitle:   "My Awesome Site",
			wantSlug:    "cool-blog",
		},
		{
			name:        "interactive, both empty → folder defaults",
			abs:         "/x/blog",
			interactive: true,
			input:       "\n\n",
			wantTitle:   "blog",
			wantSlug:    "blog",
		},
		{
			name:        "interactive, title only → slug derived from title",
			abs:         "/x/blog",
			interactive: true,
			input:       "My Title\n\n",
			wantTitle:   "My Title",
			wantSlug:    "my-title",
		},
		{
			name:        "project flag set, title asked",
			abs:         "/x/blog",
			projectFlag: "Prod-Site",
			interactive: true,
			input:       "Hello\n",
			wantTitle:   "Hello",
			wantSlug:    "prod-site",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, slug := chooseNames(reader(c.input), c.abs, c.titleFlag, c.projectFlag, c.interactive)
			if title != c.wantTitle {
				t.Errorf("title = %q, want %q", title, c.wantTitle)
			}
			if slug != c.wantSlug {
				t.Errorf("slug = %q, want %q", slug, c.wantSlug)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"My Cool Blog": "my-cool-blog",
		"example.com":     "example-com",
		"  Trim Me  ":  "trim-me",
		"---":          "my-site",
		"":             "my-site",
		"already-ok":   "already-ok",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
