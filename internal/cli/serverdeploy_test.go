package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/ShiroDoromoto/crofty/internal/secret"
)

// writeTree writes a map of slash-relative paths → content into a fresh temp dir
// and returns it — a stand-in dist/ for the SFTP/FTPS E2E tests.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// assertTreeUploaded checks every file under src now exists under dst with
// identical bytes (same slash-relative layout).
func assertTreeUploaded(t *testing.T, src, dst string) {
	t.Helper()
	files, _, err := scanDistTree(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		want, err := os.ReadFile(f.abs)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(f.rel)))
		if err != nil {
			t.Errorf("missing uploaded file %s: %v", f.rel, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s = %q; want %q", f.rel, got, want)
		}
	}
}

// Every supported provider must be documented in `crofty init`'s --provider flag
// so an agent reading the brief learns all of them — and so the docs can't drift
// from supportedProviders() (the same guard agent_test.go applies to commands).
func TestSupportedProviders_DocumentedInInit(t *testing.T) {
	var desc string
	for _, f := range agentDetails()["init"].Flags {
		if strings.HasPrefix(f.Name, "--provider") {
			desc = f.Help
		}
	}
	if desc == "" {
		t.Fatal("agentDetails()[\"init\"] has no --provider flag")
	}
	for _, p := range supportedProviders() {
		if !strings.Contains(desc, p) {
			t.Errorf("--provider help %q does not mention %q", desc, p)
		}
	}
}

func TestIsSupportedProvider(t *testing.T) {
	for _, p := range supportedProviders() {
		if !isSupportedProvider(p) {
			t.Errorf("supportedProviders() lists %q but isSupportedProvider says no", p)
		}
	}
	if isSupportedProvider("ftp") {
		t.Error("plain ftp must not be supported")
	}
	if isSupportedProvider("") {
		t.Error("empty provider must not be reported as supported")
	}
}

// scanDistTree must collect every regular file with slash-relative paths — the
// _redirects file included, since a plain host stores it like any other — and
// flag server-side Functions inputs so SFTP/FTPS can warn they're inert.
func TestScanDistTree(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "home")
	write("posts/hello/index.html", "hi")
	write("assets/site.css", "css")
	write("_redirects", "/* /index.html 200")
	write("_worker.js", "export default {}")

	files, hasFunctions, err := scanDistTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFunctions {
		t.Error("expected hasFunctions true (a _worker.js file is present)")
	}
	got := make([]string, len(files))
	for i, f := range files {
		got[i] = f.rel
	}
	sort.Strings(got)
	want := []string{"_redirects", "_worker.js", "assets/site.css", "index.html", "posts/hello/index.html"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanDistTree rel paths = %v; want %v", got, want)
	}
}

// A content section named functions/ is uploaded like any other, and must not
// set off the warning about edge files that won't run on a plain host — there
// is nothing there for the author to move elsewhere.
func TestScanDistTreeDoesNotCallAContentSectionFunctions(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "home")
	mustWrite(t, dir, "functions/index.html", "<h1>Functions</h1>")

	files, hasFunctions, err := scanDistTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if hasFunctions {
		t.Error("a rendered content section was taken for Pages Functions source")
	}
	if len(files) != 2 {
		t.Errorf("files = %d, want 2 (the section is uploaded too): %+v", len(files), files)
	}
}

// remoteDirs must list every ancestor directory, shallowest-first, so each can be
// created after its parent (FTP has no recursive mkdir).
func TestRemoteDirs(t *testing.T) {
	files := []serverFile{
		{rel: "index.html"},
		{rel: "a/b/c/deep.html"},
		{rel: "a/sibling.html"},
	}
	got := remoteDirs(files)
	want := []string{"a", "a/b", "a/b/c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("remoteDirs = %v; want %v", got, want)
	}
	// Every directory must come after its parent.
	for i, d := range got {
		if parent := strings.LastIndex(d, "/"); parent > 0 {
			p := d[:parent]
			found := false
			for j := 0; j < i; j++ {
				if got[j] == p {
					found = true
				}
			}
			if !found {
				t.Errorf("%q appears before its parent %q", d, p)
			}
		}
	}
}

func TestJoinRemote(t *testing.T) {
	cases := []struct{ base, rel, want string }{
		{"/public_html", "index.html", "/public_html/index.html"},
		{"/public_html/", "a/b.html", "/public_html/a/b.html"},
		{"site", "index.html", "site/index.html"},
	}
	for _, c := range cases {
		if got := joinRemote(c.base, c.rel); got != c.want {
			t.Errorf("joinRemote(%q, %q) = %q; want %q", c.base, c.rel, got, c.want)
		}
	}
}

// requireServerConfig must name exactly the missing non-secret fields.
func TestRequireServerConfig(t *testing.T) {
	err := requireServerConfig(&deployServerConfig{host: "example.com"}, "cfg.json")
	if err == nil {
		t.Fatal("expected an error for missing user/path")
	}
	msg := err.Error()
	if strings.Contains(msg, "missing host") {
		t.Errorf("host was set but reported missing: %q", msg)
	}
	if !strings.Contains(msg, "user") || !strings.Contains(msg, "path") {
		t.Errorf("error should name the missing user and path: %q", msg)
	}

	if err := requireServerConfig(&deployServerConfig{host: "h", user: "u", path: "/p"}, "cfg.json"); err != nil {
		t.Errorf("complete config should pass, got %v", err)
	}
}

func TestResolveSecretPrefersTheEnvironmentAndKeepsNothing(t *testing.T) {
	keyring.MockInit()
	store := secret.New("sftp")
	if err := store.Set("host:user", "password", "keychain-pw"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sftpPasswordEnv, "  env-pw\n")

	got, err := resolveSecret(store, "host:user", "password", sftpPasswordEnv, "Password for user@host", false)
	if err != nil || got != "env-pw" {
		t.Fatalf("got (%q, %v), want the environment's password", got, err)
	}
	if kept, err := store.Get("host:user", "password"); err != nil || kept != "keychain-pw" {
		t.Fatalf("keychain holds %q (%v) — an env secret must not be stored", kept, err)
	}
}

func TestResolveSecretFallsBackToTheKeychain(t *testing.T) {
	// Without the variable, the keychain path is exactly what it was.
	keyring.MockInit()
	store := secret.New("ftps")
	if err := store.Set("host:user", "password", "keychain-pw"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ftpsPasswordEnv, "")

	got, err := resolveSecret(store, "host:user", "password", ftpsPasswordEnv, "Password for user@host", false)
	if err != nil || got != "keychain-pw" {
		t.Fatalf("got (%q, %v), want the saved password", got, err)
	}
}

func TestResolveSecretIgnoresAGenericVariableName(t *testing.T) {
	// SFTP_PASSWORD is somebody else's variable; reading it would send this
	// site's files with a credential crofty was never given.
	keyring.MockInit()
	withTerminal(t, false)
	t.Setenv(sftpPasswordEnv, "")
	t.Setenv("SFTP_PASSWORD", "generic-pw")

	got, err := resolveSecret(secret.New("sftp"), "host:user", "password", sftpPasswordEnv, "Password for user@host", false)
	if got != "" || err == nil {
		t.Fatalf("got (%q, %v), want a stop", got, err)
	}
	if !strings.Contains(err.Error(), sftpPasswordEnv) {
		t.Fatalf("error %q must name the variable a run with no terminal sets", err)
	}
}

func TestResolveSecretLetsReauthOutrankTheEnvironment(t *testing.T) {
	// `crofty connect` (and --reauth) exist to save a credential. If a variable
	// answered for it, the run would report a keychain entry it never wrote.
	keyring.MockInit()
	withTerminal(t, false)
	t.Setenv(sftpPasswordEnv, "env-pw")

	got, err := resolveSecret(secret.New("sftp"), "host:user", "password", sftpPasswordEnv, "Password for user@host", true)
	if got != "" || err == nil {
		t.Fatalf("got (%q, %v), want the prompt to be required", got, err)
	}
	if strings.Contains(err.Error(), sftpPasswordEnv) {
		t.Fatalf("error %q offers the variable as a way out of a run that exists to save one", err)
	}
}

func TestResolveSecretNamesEveryCredentialSeparately(t *testing.T) {
	// One variable per credential: a runner grants the passphrase without also
	// handing over the password, and vice versa.
	keyring.MockInit()
	withTerminal(t, false)
	t.Setenv(sftpPasswordEnv, "")
	t.Setenv(sftpPassphraseEnv, "env-passphrase")

	got, err := resolveSecret(secret.New("sftp"), "host:user", "key_passphrase", sftpPassphraseEnv, "Passphrase for /k", false)
	if err != nil || got != "env-passphrase" {
		t.Fatalf("passphrase: got (%q, %v)", got, err)
	}
	if _, err := resolveSecret(secret.New("sftp"), "host:user", "password", sftpPasswordEnv, "Password for user@host", false); err == nil {
		t.Fatal("the passphrase variable must not stand in for the password")
	}
}
