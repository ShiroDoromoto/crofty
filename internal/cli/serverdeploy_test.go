package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
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
