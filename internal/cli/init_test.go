package cli

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/crofty/internal/access"
	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/runner"
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

// themesIgnoreState classifies how git treats .crofty/themes/ so the build hint
// fires only when the regenerated theme would actually be committed — keyed off
// git's effective rules, not just whether a .gitignore exists.
func TestThemesIgnoreState(t *testing.T) {
	gitInit := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		if out, err := runner.Capture(dir, "git", "init"); err != nil {
			t.Skipf("git unavailable: %v (%s)", err, out)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".crofty", "themes", "crofty"), 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("not a git repo", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".crofty", "themes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := themesIgnoreState(dir); got != themesOK {
			t.Errorf("got %q, want themesOK for a non-git folder", got)
		}
	})

	t.Run("ignored by .gitignore", func(t *testing.T) {
		dir := gitInit(t)
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/.crofty/themes/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := themesIgnoreState(dir); got != themesOK {
			t.Errorf("got %q, want themesOK when the rule is present", got)
		}
	})

	t.Run("under git but not ignored", func(t *testing.T) {
		dir := gitInit(t) // no .gitignore at all
		if got := themesIgnoreState(dir); got != themesUnignored {
			t.Errorf("got %q, want themesUnignored", got)
		}
	})

	t.Run("already committed", func(t *testing.T) {
		dir := gitInit(t)
		f := filepath.Join(dir, ".crofty", "themes", "crofty", "index.html")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Stage it (no commit needed — ls-files reads the index).
		if out, err := runner.Capture(dir, "git", "add", ".crofty/themes"); err != nil {
			t.Fatalf("git add: %v (%s)", err, out)
		}
		if got := themesIgnoreState(dir); got != themesTracked {
			t.Errorf("got %q, want themesTracked", got)
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
		"example.com":  "example-com",
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

// The bug from the field: a user parked crofty at .crofty/bin/crofty.exe, and
// `crofty init .` mistook the folder for an existing project, fell into the
// configure path, and exited 0 without writing a site. The marker is
// .crofty/config.json, so init must scaffold here — and leave the user's own
// files under .crofty/ alone (D-2).
func TestInitDot_StrayMarkerDirIsNotAProject(t *testing.T) {
	t.Setenv(project.HomeEnv, t.TempDir()) // keep the project registry out of the real one

	site := t.TempDir()
	parked := filepath.Join(site, ".crofty", "bin", "crofty.exe")
	if err := os.MkdirAll(filepath.Dir(parked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parked, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(site)

	if err := runInit([]string{"--lang", "en", "--title", "T", "--project", "t", "."}); err != nil {
		t.Fatalf("init .: %v", err)
	}

	for _, rel := range []string{"hugo.yaml", filepath.Join(".crofty", "config.json")} {
		if _, err := os.Stat(filepath.Join(site, rel)); err != nil {
			t.Errorf("init . did not create %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(parked); err != nil {
		t.Errorf("init . clobbered the user's own file under .crofty/: %v", err)
	}
}

// The registry is a convenience for discovery, not the site. When crofty cannot
// write it — a sandbox that refuses the OS config dir — init must still leave a
// working site behind and exit 0, instead of reporting "Access is denied." over
// a site that exists (D-1).
func TestInit_SurvivesAnUnwritableRegistry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not deny writes on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	locked := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Setenv(project.HomeEnv, filepath.Join(locked, "crofty"))

	site := t.TempDir()
	t.Chdir(site)

	if err := runInit([]string{"--lang", "en", "--title", "T", "--project", "t", "."}); err != nil {
		t.Fatalf("init failed over an unwritable registry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(site, ".crofty", "config.json")); err != nil {
		t.Errorf("init did not write the site: %v", err)
	}
}

// What init could not do is reported as a fork the author picks from, so the AI
// reading it asks instead of rewriting the author's environment (D-1).
func TestReportRegisterFailure_ShowsTheChoices(t *testing.T) {
	var buf bytes.Buffer
	err := access.Deny("record this project", "/state/projects.json",
		&fs.PathError{Op: "open", Path: "/state/projects.json", Err: fs.ErrPermission},
		access.Choice{Do: "keep crofty's state somewhere it may write", Permission: "setting " + project.HomeEnv},
	)

	reportRegisterFailure(&buf, err)

	out := buf.String()
	for _, want := range []string{"written and whole", "keep crofty's state somewhere it may write", access.AgentRule} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
}

// Anything that is not a permission wall is still worth a line — and still not
// a failed init.
func TestReportRegisterFailure_PlainErrorStaysANote(t *testing.T) {
	var buf bytes.Buffer

	reportRegisterFailure(&buf, fs.ErrInvalid)

	if !strings.Contains(buf.String(), "Nothing is missing from the site") {
		t.Errorf("plain error did not reassure the reader:\n%s", buf.String())
	}
}
