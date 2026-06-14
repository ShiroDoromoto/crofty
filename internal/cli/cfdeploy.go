package cli

// Native Cloudflare Pages "Direct Upload" — the same wire protocol wrangler
// speaks, reimplemented over the CF API so crofty deploys with no Node/wrangler
// dependency (a single Go binary). The sequence, hashing, and bucketing mirror
// wrangler's so deployments are byte-identical:
//
//  1. GET  /accounts/{a}/pages/projects/{p}/upload-token   (account token) → JWT
//  2. POST /pages/assets/check-missing                      (JWT) → hashes to upload
//  3. POST /pages/assets/upload                             (JWT) per bucket
//  4. POST /pages/assets/upsert-hashes                      (JWT) → all hashes
//  5. POST /accounts/{a}/pages/projects/{p}/deployments     (account token, multipart)
//
// Two credentials are in play: the account API token for the /accounts/... calls
// and the short-lived JWT (minted by step 1) for the /pages/assets/... calls.
//
// Asset hash = hex(blake3(base64(fileBytes) + extWithoutDot))[:32], exactly as
// @cloudflare/deploy-helpers hashFile computes it.

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"lukechampine.com/blake3"
)

// Limits and bucketing constants, matching wrangler's pages/constants.ts.
const (
	cfMaxAssetSize      = 25 * 1024 * 1024 // per-file ceiling Cloudflare enforces
	cfMaxAssetCount     = 20000            // per-deployment file ceiling
	cfMaxBucketSize     = 40 * 1024 * 1024 // raw bytes per upload request
	cfMaxBucketFiles    = 2000             // files per upload request
	cfUploadConcurrency = 3                // buckets in flight at once
	cfMaxUploadAttempts = 5                // retries per bucket / check-missing
)

// cfUploadHTTP is for the large/slow asset-upload and deployment requests; the
// short cfHTTP timeout (used for token/check calls) would cut these off.
func cfUploadHTTP() *http.Client { return &http.Client{Timeout: 5 * time.Minute} }

// cfAsset is one file to be deployed.
type cfAsset struct {
	name        string // slash-joined path relative to dist, no leading slash
	path        string // absolute path on disk
	contentType string
	size        int64
	hash        string
}

// cfDirScan is the result of walking a built site: ordinary assets plus the
// special files Pages takes as their own deployment fields.
type cfDirScan struct {
	assets    []cfAsset
	headers   string // path to a root _headers file, if present
	redirects string // path to a root _redirects file, if present
	functions bool   // a _worker.js / functions/ Pages-Functions build (unsupported)
}

// cfScanDir walks dir, hashing every asset and setting aside the special files.
func cfScanDir(dir string) (cfDirScan, error) {
	var scan cfDirScan
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		name := filepath.ToSlash(rel)
		if d.IsDir() {
			// A Pages Functions build lives in a top-level functions/ dir.
			if name == "functions" {
				scan.functions = true
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, sockets, etc.
		}
		// Root-level special files are not ordinary assets.
		if !strings.Contains(name, "/") {
			switch name {
			case "_headers":
				scan.headers = p
				return nil
			case "_redirects":
				scan.redirects = p
				return nil
			case "_worker.js":
				scan.functions = true
				return nil
			case "_routes.json":
				return nil // a Functions field; not deployed by crofty
			}
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if info.Size() > cfMaxAssetSize {
			return fmt.Errorf("%s is %d bytes — over Cloudflare Pages' %d-byte per-file limit", name, info.Size(), cfMaxAssetSize)
		}
		h, herr := cfHashFile(p)
		if herr != nil {
			return herr
		}
		scan.assets = append(scan.assets, cfAsset{
			name:        name,
			path:        p,
			contentType: cfContentType(name),
			size:        info.Size(),
			hash:        h,
		})
		return nil
	})
	if err != nil {
		return cfDirScan{}, err
	}
	if len(scan.assets) > cfMaxAssetCount {
		return cfDirScan{}, fmt.Errorf("%d files — over Cloudflare Pages' %d-file limit", len(scan.assets), cfMaxAssetCount)
	}
	return scan, nil
}

// cfHashFile computes an asset's content hash the way wrangler does: blake3 over
// the base64 of the raw bytes concatenated with the extension (no leading dot,
// original case), keeping the first 32 hex chars.
func cfHashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(b)
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	sum := blake3.Sum256([]byte(b64 + ext))
	return hex.EncodeToString(sum[:])[:32], nil
}

// cfMimeOverrides pins the content types of common web assets so deploys are
// deterministic across platforms (Go's mime table varies by OS and omits some).
var cfMimeOverrides = map[string]string{
	".html":        "text/html; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".json":        "application/json",
	".xml":         "application/xml",
	".txt":         "text/plain; charset=utf-8",
	".svg":         "image/svg+xml",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".webmanifest": "application/manifest+json",
}

func cfContentType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := cfMimeOverrides[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// cfDeployDir runs the full Direct Upload sequence and returns the deployment's
// canonical URL. progress receives short human lines so deploy.go can print them.
func cfDeployDir(token, accountID, project, branch, dir string, progress func(string)) (string, error) {
	scan, err := cfScanDir(dir)
	if err != nil {
		return "", err
	}
	if len(scan.assets) == 0 {
		return "", fmt.Errorf("no files to deploy in %s", dir)
	}
	if scan.functions {
		progress("⚠ this site uses Pages Functions (_worker.js / functions/) — crofty's native")
		progress("  deploy uploads static assets only; those parts won't run. (Tell us if you need them.)")
	}

	if err := cfEnsureProject(token, accountID, project, branch); err != nil {
		return "", fmt.Errorf("preparing the Pages project: %w", err)
	}

	u := &cfUploader{token: token, accountID: accountID, project: project}
	if _, err := u.currentJWT(); err != nil {
		return "", fmt.Errorf("getting an upload token: %w", err)
	}

	// Dedupe by hash: identical content+extension uploads once but can map from
	// many manifest paths.
	byHash := map[string]cfAsset{}
	manifest := make(map[string]string, len(scan.assets))
	for _, a := range scan.assets {
		manifest["/"+a.name] = a.hash
		if _, seen := byHash[a.hash]; !seen {
			byHash[a.hash] = a
		}
	}
	allHashes := make([]string, 0, len(byHash))
	for h := range byHash {
		allHashes = append(allHashes, h)
	}

	missing, err := u.checkMissing(allHashes)
	if err != nil {
		return "", fmt.Errorf("checking which files are new: %w", err)
	}
	newCount := len(missing)
	progress(fmt.Sprintf("Uploading %d files (%d new, %d already on Cloudflare)…", len(scan.assets), newCount, len(allHashes)-newCount))

	if newCount > 0 {
		buckets := cfBucketize(missing, byHash)
		if err := u.uploadBuckets(buckets); err != nil {
			return "", fmt.Errorf("uploading files: %w", err)
		}
	}
	if err := u.upsertHashes(allHashes); err != nil {
		return "", fmt.Errorf("finalizing the upload: %w", err)
	}

	progress("Creating the deployment…")
	url, aliases, err := cfCreateDeployment(token, accountID, project, branch, manifest, scan.headers, scan.redirects)
	if err != nil {
		return "", fmt.Errorf("creating the deployment: %w", err)
	}
	return cfPickURL(url, aliases), nil
}

// cfBucketize packs the missing files into upload buckets bounded by byte size
// and file count. Largest-first keeps the bucket count near-minimal.
func cfBucketize(missing []string, byHash map[string]cfAsset) [][]cfAsset {
	files := make([]cfAsset, 0, len(missing))
	for _, h := range missing {
		if a, ok := byHash[h]; ok {
			files = append(files, a)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].size > files[j].size })

	var buckets [][]cfAsset
	var cur []cfAsset
	var curSize int64
	for _, f := range files {
		if len(cur) > 0 && (curSize+f.size > cfMaxBucketSize || len(cur) >= cfMaxBucketFiles) {
			buckets = append(buckets, cur)
			cur, curSize = nil, 0
		}
		cur = append(cur, f)
		curSize += f.size
	}
	if len(cur) > 0 {
		buckets = append(buckets, cur)
	}
	return buckets
}

// cfUploader holds the credentials and the live (refreshable) upload JWT.
type cfUploader struct {
	token, accountID, project string
	mu                        sync.Mutex
	jwt                       string
}

// currentJWT returns a non-expired upload JWT, minting a fresh one when needed.
func (u *cfUploader) currentJWT() (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if cfJWTExpired(u.jwt) {
		j, err := cfUploadToken(u.token, u.accountID, u.project)
		if err != nil {
			return "", err
		}
		u.jwt = j
	}
	return u.jwt, nil
}

func (u *cfUploader) uploadBuckets(buckets [][]cfAsset) error {
	sem := make(chan struct{}, cfUploadConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, b := range buckets {
		bucket := b
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := u.uploadBucket(bucket); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// cfUploadItem is one asset in an /pages/assets/upload request body.
type cfUploadItem struct {
	Key      string `json:"key"`
	Value    string `json:"value"` // base64 of the raw file bytes
	Metadata struct {
		ContentType string `json:"contentType"`
	} `json:"metadata"`
	Base64 bool `json:"base64"`
}

func (u *cfUploader) uploadBucket(bucket []cfAsset) error {
	items := make([]cfUploadItem, 0, len(bucket))
	for _, a := range bucket {
		b, err := os.ReadFile(a.path)
		if err != nil {
			return err
		}
		var it cfUploadItem
		it.Key = a.hash
		it.Value = base64.StdEncoding.EncodeToString(b)
		it.Metadata.ContentType = a.contentType
		it.Base64 = true
		items = append(items, it)
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < cfMaxUploadAttempts; attempt++ {
		jwt, jerr := u.currentJWT()
		if jerr != nil {
			return jerr
		}
		body, status, derr := cfRequest(cfUploadHTTP(), http.MethodPost, "/pages/assets/upload", jwt, "application/json", bytes.NewReader(payload))
		if derr == nil && status >= 200 && status < 300 {
			return nil
		}
		lastErr = cfStatusErr(body, status, derr)
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			u.expireJWT() // force a refresh on the next attempt
		}
	}
	return lastErr
}

// expireJWT drops the cached JWT so currentJWT mints a new one.
func (u *cfUploader) expireJWT() {
	u.mu.Lock()
	u.jwt = ""
	u.mu.Unlock()
}

func (u *cfUploader) checkMissing(hashes []string) ([]string, error) {
	payload, _ := json.Marshal(map[string][]string{"hashes": hashes})
	var lastErr error
	for attempt := 0; attempt < cfMaxUploadAttempts; attempt++ {
		jwt, jerr := u.currentJWT()
		if jerr != nil {
			return nil, jerr
		}
		body, status, err := cfRequest(cfHTTP(), http.MethodPost, "/pages/assets/check-missing", jwt, "application/json", bytes.NewReader(payload))
		if err == nil && status >= 200 && status < 300 {
			var out struct {
				cfResponse
				Result []string `json:"result"`
			}
			if jerr := json.Unmarshal(body, &out); jerr != nil {
				return nil, jerr
			}
			return out.Result, nil
		}
		lastErr = cfStatusErr(body, status, err)
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			u.expireJWT()
		}
	}
	return nil, lastErr
}

// upsertHashes registers the full asset set so Cloudflare keeps the uploaded
// blobs alive for this deployment.
func (u *cfUploader) upsertHashes(hashes []string) error {
	payload, _ := json.Marshal(map[string][]string{"hashes": hashes})
	jwt, err := u.currentJWT()
	if err != nil {
		return err
	}
	body, status, err := cfRequest(cfHTTP(), http.MethodPost, "/pages/assets/upsert-hashes", jwt, "application/json", bytes.NewReader(payload))
	if err == nil && status >= 200 && status < 300 {
		return nil
	}
	return cfStatusErr(body, status, err)
}

// cfEnsureProject makes sure the Pages project exists, creating it (idempotently)
// on the first deploy.
func cfEnsureProject(token, accountID, project, branch string) error {
	body, status, err := cfGet(token, "/accounts/"+accountID+"/pages/projects/"+project)
	if err != nil {
		return err
	}
	if status >= 200 && status < 300 {
		return nil
	}
	if status != http.StatusNotFound {
		return cfStatusErr(body, status, nil)
	}
	payload, _ := json.Marshal(map[string]string{"name": project, "production_branch": branch})
	rb, st, err := cfRequest(cfHTTP(), http.MethodPost, "/accounts/"+accountID+"/pages/projects", token, "application/json", bytes.NewReader(payload))
	if err == nil && st >= 200 && st < 300 {
		return nil
	}
	return cfStatusErr(rb, st, err)
}

func cfUploadToken(token, accountID, project string) (string, error) {
	body, status, err := cfGet(token, "/accounts/"+accountID+"/pages/projects/"+project+"/upload-token")
	if err != nil {
		return "", err
	}
	var out struct {
		cfResponse
		Result struct {
			JWT string `json:"jwt"`
		} `json:"result"`
	}
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return "", jerr
	}
	if status < 200 || status >= 300 || !out.Success {
		return "", out.err(status)
	}
	if out.Result.JWT == "" {
		return "", fmt.Errorf("Cloudflare returned an empty upload token")
	}
	return out.Result.JWT, nil
}

// cfJWTExpired reports whether a JWT is missing or within 30s of its exp claim.
func cfJWTExpired(jwt string) bool {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return true
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false // can't read exp — assume valid and let the call decide
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return false
	}
	return time.Now().Unix() >= claims.Exp-30
}

// cfCreateDeployment posts the manifest (and any _headers/_redirects) as
// multipart form data and returns the deployment URL plus its aliases.
func cfCreateDeployment(token, accountID, project, branch string, manifest map[string]string, headersPath, redirectsPath string) (string, []string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	mj, _ := json.Marshal(manifest)
	_ = w.WriteField("manifest", string(mj))
	if branch != "" {
		_ = w.WriteField("branch", branch)
	}
	if err := cfAddFileField(w, "_headers", headersPath); err != nil {
		return "", nil, err
	}
	if err := cfAddFileField(w, "_redirects", redirectsPath); err != nil {
		return "", nil, err
	}
	if err := w.Close(); err != nil {
		return "", nil, err
	}

	body, status, err := cfRequest(cfUploadHTTP(), http.MethodPost, "/accounts/"+accountID+"/pages/projects/"+project+"/deployments", token, w.FormDataContentType(), &buf)
	if err != nil {
		return "", nil, err
	}
	var out struct {
		cfResponse
		Result struct {
			URL     string   `json:"url"`
			Aliases []string `json:"aliases"`
		} `json:"result"`
	}
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return "", nil, jerr
	}
	if status < 200 || status >= 300 || !out.Success {
		return "", nil, out.err(status)
	}
	return out.Result.URL, out.Result.Aliases, nil
}

// cfAddFileField attaches a file (e.g. _headers) as its own multipart field. A
// blank path means the file isn't present, so nothing is added.
func cfAddFileField(w *multipart.Writer, field, path string) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fw, err := w.CreateFormFile(field, field)
	if err != nil {
		return err
	}
	_, err = fw.Write(b)
	return err
}

// cfPickURL prefers the stable https://<project>.pages.dev alias over the
// per-deploy hashed URL, falling back to the hash-stripped deploy URL.
func cfPickURL(url string, aliases []string) string {
	for _, a := range aliases {
		if strings.HasSuffix(a, ".pages.dev") {
			return a
		}
	}
	return canonicalPagesURL(url)
}

// cfRequest performs an authenticated CF API request, returning the raw body,
// HTTP status, and any transport error.
func cfRequest(client *http.Client, method, path, bearer, contentType string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequest(method, cfAPIBase+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

// cfStatusErr turns a non-2xx CF reply (or transport error) into the clearest
// message available: the API's own error text, else the HTTP status.
func cfStatusErr(body []byte, status int, transport error) error {
	if transport != nil {
		return transport
	}
	var r cfResponse
	if json.Unmarshal(body, &r) == nil && len(r.Errors) > 0 {
		return r.err(status)
	}
	return fmt.Errorf("Cloudflare API error (HTTP %d)", status)
}
