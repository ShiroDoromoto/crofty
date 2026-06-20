package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

// underGit detects a git working tree at the root or any ancestor, so the build
// hint only fires for sites actually tracked in git.
func TestUnderGit(t *testing.T) {
	t.Run("no git anywhere", func(t *testing.T) {
		if underGit(t.TempDir()) {
			t.Error("expected false with no .git present")
		}
	})
	t.Run("git at root", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !underGit(dir) {
			t.Error("expected true with .git at root")
		}
	})
	t.Run("git at ancestor", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(dir, "site", "nested")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if !underGit(sub) {
			t.Error("expected true with .git at an ancestor")
		}
	})
}

// ensureGitignore creates a minimal .gitignore for a fresh site but must never
// clobber one an author already has (the 'init .' case).
func TestEnsureGitignore(t *testing.T) {
	t.Run("creates when absent", func(t *testing.T) {
		dir := t.TempDir()
		if err := ensureGitignore(dir); err != nil {
			t.Fatalf("ensureGitignore: %v", err)
		}
		body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatalf("reading .gitignore: %v", err)
		}
		for _, want := range []string{"/dist/", "/.crofty/themes/"} {
			if !strings.Contains(string(body), want) {
				t.Errorf("generated .gitignore missing %q:\n%s", want, body)
			}
		}
		if strings.Contains(string(body), "/.crofty/config.json") {
			t.Errorf(".gitignore should keep .crofty/config.json tracked, got:\n%s", body)
		}
	})

	t.Run("leaves an existing file untouched", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".gitignore")
		const existing = "my-own-rules/\n"
		if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ensureGitignore(dir); err != nil {
			t.Fatalf("ensureGitignore: %v", err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != existing {
			t.Errorf("existing .gitignore was modified: got %q want %q", body, existing)
		}
	})
}

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
