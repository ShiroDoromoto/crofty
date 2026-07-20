package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCFHashFile locks the asset hashing to wrangler's algorithm:
// hex(blake3(base64(bytes) + extWithoutDot))[:32]. The golden value must not
// drift — a mismatch means deploys would silently upload broken sites.
func TestCFHashFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "index.html")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := cfHashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	const want = "a2b82584e50075886b08927390f2f573" // blake3(base64("hello")+"html")[:32]
	if got != want {
		t.Fatalf("hash = %q, want %q", got, want)
	}
	if len(got) != 32 {
		t.Fatalf("hash length = %d, want 32", len(got))
	}
}

func TestCFContentType(t *testing.T) {
	cases := map[string]string{
		"index.html":     "text/html; charset=utf-8",
		"a/b/style.css":  "text/css; charset=utf-8",
		"app.js":         "text/javascript; charset=utf-8",
		"data.json":      "application/json",
		"logo.svg":       "image/svg+xml",
		"font.woff2":     "font/woff2",
		"unknownext.zzz": "application/octet-stream",
	}
	for name, want := range cases {
		if got := cfContentType(name); got != want {
			t.Errorf("cfContentType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCFScanDir(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "<h1>hi</h1>")
	mustWrite(t, dir, "css/style.css", "body{}")
	mustWrite(t, dir, "_redirects", "/old /new 301")
	mustWrite(t, dir, "_headers", "/*\n  X-Test: 1")

	scan, err := cfScanDir(dir, cfParts())
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.assets) != 2 {
		t.Fatalf("assets = %d, want 2 (special files excluded): %+v", len(scan.assets), scan.assets)
	}
	b := assembleBundle(dir, dir)
	if b.parts[partHeaders] == "" || b.parts[partRedirects] == "" {
		t.Fatalf("_headers/_redirects not collected as parts: %+v", b.parts)
	}
	for _, a := range scan.assets {
		if a.hash == "" || a.contentType == "" {
			t.Fatalf("asset missing hash/contentType: %+v", a)
		}
		if strings.HasPrefix(a.name, "/") {
			t.Fatalf("asset name should not have a leading slash: %q", a.name)
		}
	}
}

func TestCFScanDirSkipsFunctionsTree(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "<h1>hi</h1>")
	mustWrite(t, dir, "functions/api/contact.js", "export function onRequest() {}")
	mustWrite(t, dir, "functions/_middleware.js", "export function onRequest() {}")

	scan, err := cfScanDir(dir, cfParts())
	if err != nil {
		t.Fatal(err)
	}
	if !scan.functionsDir {
		t.Fatal("a functions/ dir should be seen as server source, not assets")
	}
	for _, a := range scan.assets {
		if strings.HasPrefix(a.name, "functions/") {
			t.Errorf("server-side source published as an asset: %q", a.name)
		}
	}
	if len(scan.assets) != 1 {
		t.Fatalf("assets = %d, want 1 (only index.html): %+v", len(scan.assets), scan.assets)
	}
}

// A part that belongs at the project root, found in the build instead, got
// there through static/. crofty carries the copy at the root, so this one would
// be ignored — and a deploy that ignores the author's worker looks like it
// worked. Say where it belongs and stop.
func TestCFScanDirStopsOnARootPartInTheBuild(t *testing.T) {
	for _, name := range []string{"_worker.js", "_routes.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, dir, "index.html", "<h1>hi</h1>")
			mustWrite(t, dir, name, "{}")

			_, err := cfScanDir(dir, cfParts())
			if err == nil {
				t.Fatalf("%s in the build should stop the deploy", name)
			}
			if !strings.Contains(err.Error(), "project root") {
				t.Errorf("error = %q, want it to say where the file belongs", err)
			}
		})
	}
}

func TestCFScanDirRejectsOversizeFile(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, cfMaxAssetSize+1)
	if err := os.WriteFile(filepath.Join(dir, "huge.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cfScanDir(dir, cfParts()); err == nil {
		t.Fatal("expected an error for a file over the per-file limit")
	}
}

func TestCFJWTExpired(t *testing.T) {
	if !cfJWTExpired("") {
		t.Error("empty token should count as expired")
	}
	if !cfJWTExpired("not-a-jwt") {
		t.Error("a malformed (single-segment) token should count as expired")
	}
	if cfJWTExpired(makeJWT(t, time.Now().Add(time.Hour).Unix())) {
		t.Error("a token expiring in an hour should be valid")
	}
	if !cfJWTExpired(makeJWT(t, time.Now().Add(-time.Hour).Unix())) {
		t.Error("a token that expired an hour ago should be expired")
	}
}

func TestCFPickURL(t *testing.T) {
	got := cfPickURL("https://abc123.proj.pages.dev", []string{"https://proj.pages.dev"})
	if got != "https://proj.pages.dev" {
		t.Errorf("expected the stable alias, got %q", got)
	}
	// No alias → strip the hash label from the deploy URL.
	got = cfPickURL("https://abc123.proj.pages.dev", nil)
	if got != "https://proj.pages.dev" {
		t.Errorf("expected hash-stripped URL, got %q", got)
	}
}

// TestCFDeployBundle drives the whole Direct Upload sequence against a fake CF
// API, asserting the dual-auth split, the upload payload, and the returned URL.
func TestCFDeployBundle(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "<h1>hi</h1>")
	mustWrite(t, dir, "css/style.css", "body{color:red}")

	jwt := makeJWT(t, time.Now().Add(time.Hour).Unix())

	var mu sync.Mutex
	var uploadedKeys []string
	var gotManifest map[string]string
	authByPath := map[string]string{}

	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authByPath[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/upload-token"):
			fmt.Fprintf(w, `{"success":true,"result":{"jwt":%q}}`, jwt)

		case r.URL.Path == "/pages/assets/check-missing":
			// Report every hash as missing so all assets get uploaded.
			body, _ := io.ReadAll(r.Body)
			var in struct {
				Hashes []string `json:"hashes"`
			}
			json.Unmarshal(body, &in)
			out, _ := json.Marshal(map[string]any{"success": true, "result": in.Hashes})
			w.Write(out)

		case r.URL.Path == "/pages/assets/upload":
			body, _ := io.ReadAll(r.Body)
			var items []cfUploadItem
			json.Unmarshal(body, &items)
			mu.Lock()
			for _, it := range items {
				uploadedKeys = append(uploadedKeys, it.Key)
				if !it.Base64 {
					t.Errorf("upload item %s missing base64 flag", it.Key)
				}
			}
			mu.Unlock()
			w.Write([]byte(`{"success":true,"result":null}`))

		case r.URL.Path == "/pages/assets/upsert-hashes":
			w.Write([]byte(`{"success":true,"result":null}`))

		case strings.HasSuffix(r.URL.Path, "/pages/projects/site") && r.Method == http.MethodGet:
			w.Write([]byte(`{"success":true,"result":{"name":"site"}}`)) // project exists

		case strings.HasSuffix(r.URL.Path, "/deployments"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parsing deployment form: %v", err)
			}
			json.Unmarshal([]byte(r.FormValue("manifest")), &gotManifest)
			w.Write([]byte(`{"success":true,"result":{"url":"https://h.site.pages.dev","aliases":["https://site.pages.dev"]}}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})()

	url, err := cfDeployBundle("acct-token", "acct1", "site", "main", assembleBundle(dir, dir), func(string) {})
	if err != nil {
		t.Fatalf("cfDeployBundle: %v", err)
	}
	if url != "https://site.pages.dev" {
		t.Errorf("url = %q, want https://site.pages.dev", url)
	}
	if len(uploadedKeys) != 2 {
		t.Errorf("uploaded %d assets, want 2", len(uploadedKeys))
	}
	if gotManifest["/index.html"] == "" || gotManifest["/css/style.css"] == "" {
		t.Errorf("manifest missing expected paths: %+v", gotManifest)
	}

	// Dual auth: account token for /accounts/..., JWT for /pages/assets/...
	if a := authByPath["/pages/assets/upload"]; a != "Bearer "+jwt {
		t.Errorf("upload auth = %q, want the JWT", a)
	}
	if a := authByPath["/accounts/acct1/pages/projects/site/deployments"]; a != "Bearer acct-token" {
		t.Errorf("deployment auth = %q, want the account token", a)
	}
}

// cfDeployToFake runs a whole deploy against a fake CF API that answers every
// step, handing the deployment request back for inspection. The Direct Upload
// sequence itself is covered by TestCFDeployBundle; the tests below are only
// interested in what ends up on that last request.
func cfDeployToFake(t *testing.T, root, dist string) (*multipart.Form, []string) {
	t.Helper()
	jwt := makeJWT(t, time.Now().Add(time.Hour).Unix())
	var mu sync.Mutex
	var form *multipart.Form

	defer withCFServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/upload-token"):
			fmt.Fprintf(w, `{"success":true,"result":{"jwt":%q}}`, jwt)
		case r.URL.Path == "/pages/assets/check-missing":
			w.Write([]byte(`{"success":true,"result":[]}`))
		case r.URL.Path == "/pages/assets/upsert-hashes":
			w.Write([]byte(`{"success":true,"result":null}`))
		case strings.HasSuffix(r.URL.Path, "/pages/projects/site") && r.Method == http.MethodGet:
			w.Write([]byte(`{"success":true,"result":{"name":"site"}}`))
		case strings.HasSuffix(r.URL.Path, "/deployments"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parsing deployment form: %v", err)
			}
			mu.Lock()
			form = r.MultipartForm
			mu.Unlock()
			w.Write([]byte(`{"success":true,"result":{"url":"https://h.site.pages.dev","aliases":["https://site.pages.dev"]}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})()

	var said []string
	if _, err := cfDeployBundle("acct-token", "acct1", "site", "main", assembleBundle(root, dist), func(line string) {
		said = append(said, line)
	}); err != nil {
		t.Fatalf("cfDeployBundle: %v", err)
	}
	if form == nil {
		t.Fatal("no deployment was created")
	}
	return form, said
}

// filePart reads one file field out of a parsed multipart form: its bytes and
// the content type the sender put on it.
func filePart(t *testing.T, form *multipart.Form, field string) (body, contentType string, ok bool) {
	t.Helper()
	fh := form.File[field]
	if len(fh) == 0 {
		return "", "", false
	}
	f, err := fh[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), fh[0].Header.Get("Content-Type"), true
}

// The worker has to arrive in the shape Pages runs: the deployment's
// _worker.bundle field, holding a form of its own whose Content-Type names the
// inner boundary. Send it as a plain file and the upload is accepted, and never
// runs. _routes.json rides with it byte for byte as the author wrote it —
// crofty synthesizes no default, or the same site would behave differently
// under wrangler.
func TestCFDeployBundleCarriesTheWorker(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "index.html", "<h1>hi</h1>")
	const workerSrc = "export default { fetch: () => new Response('ok') }"
	const routesSrc = `{"version":1,"include":["/api/*"],"exclude":[]}`
	mustWrite(t, root, "_worker.js", workerSrc)
	mustWrite(t, root, "_routes.json", routesSrc)

	form, said := cfDeployToFake(t, root, dist)

	gotRoutes, _, ok := filePart(t, form, "_routes.json")
	if !ok || gotRoutes != routesSrc {
		t.Errorf("_routes.json = %q (present %v), want the author's file verbatim", gotRoutes, ok)
	}
	gotWorker, gotWorkerType, ok := filePart(t, form, "_worker.bundle")
	if !ok {
		t.Fatal("the deployment carries no _worker.bundle")
	}
	_, params, err := mime.ParseMediaType(gotWorkerType)
	if err != nil || params["boundary"] == "" {
		t.Fatalf("_worker.bundle content type = %q, which names no inner boundary (err %v)", gotWorkerType, err)
	}
	mr := multipart.NewReader(strings.NewReader(gotWorker), params["boundary"])
	seen := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		seen[p.FormName()] = string(b)
	}
	if seen["_worker.js"] != workerSrc {
		t.Errorf("bundled module = %q, want the worker verbatim", seen["_worker.js"])
	}
	if !strings.Contains(seen["metadata"], `"main_module":"_worker.js"`) {
		t.Errorf("metadata = %q, want it to name the entry module", seen["metadata"])
	}
	for _, line := range said {
		if strings.Contains(line, "_routes.json") {
			t.Errorf("warned about _routes.json when there is one: %q", line)
		}
	}
}

// A worker with no _routes.json runs on every request, static files included,
// and each one is billed. Nothing is broken by it, so the deploy goes on — but
// it must not go on in silence.
func TestCFDeployBundleWarnsWhenTheWorkerHasNoRoutes(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "index.html", "<h1>hi</h1>")
	mustWrite(t, root, "_worker.js", "export default { fetch: () => new Response('ok') }")

	form, said := cfDeployToFake(t, root, dist)
	if _, _, ok := filePart(t, form, "_worker.bundle"); !ok {
		t.Error("the worker should still travel without _routes.json")
	}
	if !strings.Contains(strings.Join(said, "\n"), "_routes.json") {
		t.Errorf("nothing said about the missing _routes.json: %v", said)
	}
}

// Without a worker there is nothing to route to, so _routes.json stays home
// rather than travelling on its own and meaning nothing.
func TestCFDeployBundleLeavesRoutesWhenThereIsNoWorker(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	mustWrite(t, dist, "index.html", "<h1>hi</h1>")
	mustWrite(t, root, "_routes.json", `{"include":["/api/*"]}`)

	form, _ := cfDeployToFake(t, root, dist)
	if _, _, ok := filePart(t, form, "_routes.json"); ok {
		t.Error("_routes.json travelled without a worker")
	}
}

func mustWrite(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeJWT(t *testing.T, exp int64) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
	return hdr + "." + payload + ".sig"
}
