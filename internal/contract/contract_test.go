package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDist builds a throwaway dist/ from a map of relative path → file contents.
func writeDist(t *testing.T, files map[string]string) string {
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

// findingsFor returns the findings whose Check id matches.
func findingsFor(r Report, check string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

const goodPage = `<!DOCTYPE html><html lang="en"><head>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Hello</title>
<link rel="canonical" href="https://example.com/posts/hello/">
<link rel="alternate" type="application/rss+xml" href="https://example.com/index.xml">
</head><body><article>Hi.</article></body></html>`

func TestCheckCleanSitePasses(t *testing.T) {
	dir := writeDist(t, map[string]string{
		"index.html":             goodPage,
		"posts/hello/index.html": goodPage,
		"index.xml":              `<rss></rss>`,
	})
	r, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || r.HasError() {
		t.Fatalf("expected a clean pass, got findings: %+v", r.Findings)
	}
}

func TestRedirectAliasIsSkipped(t *testing.T) {
	// Hugo emits /page/1/ and taxonomy aliases as <meta http-equiv="refresh">
	// stubs with no viewport, lang, or body. They are redirects, not content
	// pages, so the contract must skip them rather than flag them.
	alias := `<!DOCTYPE html><html><head>
<title>https://example.com/</title>
<link rel="canonical" href="https://example.com/">
<meta http-equiv="refresh" content="0; url=https://example.com/">
</head></html>`
	dir := writeDist(t, map[string]string{
		"index.html":        goodPage,
		"page/1/index.html": alias,
		"index.xml":         `<rss></rss>`,
	})
	r, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || r.HasError() {
		t.Fatalf("expected redirect alias to be skipped, got findings: %+v", r.Findings)
	}
	for _, f := range r.Findings {
		if f.File == "page/1/index.html" {
			t.Fatalf("alias stub should yield no findings, got: %+v", f)
		}
	}
}

func TestMissingCanonicalIsError(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head>
<meta name="viewport" content="x"><title>T</title></head><body>x</body></html>`
	dir := writeDist(t, map[string]string{"posts/x/index.html": page, "index.html": goodPage, "index.xml": "<rss/>"})
	r, _ := Check(dir)
	if !r.HasError() {
		t.Fatal("expected an error for a missing canonical link")
	}
	got := findingsFor(r, "C-E1")
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("expected one C-E1 error, got %+v", got)
	}
}

func TestMissingFeedIsError(t *testing.T) {
	dir := writeDist(t, map[string]string{"index.html": goodPage}) // no index.xml
	r, _ := Check(dir)
	if len(findingsFor(r, "C-E2")) == 0 || !r.HasError() {
		t.Fatalf("expected a C-E2 error for the missing feed, got %+v", r.Findings)
	}
}

func TestMissingLangAndTitleAreErrors(t *testing.T) {
	page := `<!DOCTYPE html><html><head><link rel="canonical" href="https://e.com/"></head><body>x</body></html>`
	dir := writeDist(t, map[string]string{"index.html": page, "index.xml": "<rss/>"})
	r, _ := Check(dir)
	c6 := findingsFor(r, "C-E6")
	var lang, title bool
	for _, f := range c6 {
		if f.Severity != SeverityError {
			continue
		}
		switch {
		case strings.Contains(f.Message, "lang"):
			lang = true
		case strings.Contains(f.Message, "title"):
			title = true
		}
	}
	if !lang || !title {
		t.Fatalf("expected lang+title errors, got %+v", c6)
	}
}

func TestMalformedIDIsError(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head><meta name="viewport" content="x"><title>T</title>
<link rel="canonical" href="https://e.com/p/"><meta name="crofty:id" content="not-a-ulid"></head><body>x</body></html>`
	dir := writeDist(t, map[string]string{"posts/p/index.html": page, "index.html": goodPage, "index.xml": "<rss/>"})
	r, _ := Check(dir)
	got := findingsFor(r, "C-E3")
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("expected one C-E3 error for a malformed id, got %+v", got)
	}
}

func TestValidIDPasses(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head><meta name="viewport" content="x"><title>T</title>
<link rel="canonical" href="https://e.com/p/"><meta name="crofty:id" content="01J9Z3M8XK7Q2N4P6R8T0V2W4Y"></head><body>x</body></html>`
	dir := writeDist(t, map[string]string{"posts/p/index.html": page, "index.html": goodPage, "index.xml": "<rss/>"})
	r, _ := Check(dir)
	if len(findingsFor(r, "C-E3")) != 0 {
		t.Fatalf("a valid ULID should not produce a C-E3 finding: %+v", findingsFor(r, "C-E3"))
	}
}

func Test404IsExemptFromCanonical(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head><meta name="viewport" content="x"><title>Not found</title></head><body>x</body></html>`
	dir := writeDist(t, map[string]string{"404.html": page, "index.html": goodPage, "index.xml": "<rss/>"})
	r, _ := Check(dir)
	for _, f := range findingsFor(r, "C-E1") {
		if f.File == "404.html" {
			t.Fatalf("404.html should be exempt from the canonical check, got %+v", f)
		}
	}
}

func TestMissingViewportIsWarnNotError(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head><title>T</title>
<link rel="canonical" href="https://e.com/"></head><body>x</body></html>`
	dir := writeDist(t, map[string]string{"index.html": page, "index.xml": "<rss/>"})
	r, _ := Check(dir)
	got := findingsFor(r, "C-E6")
	var vp *Finding
	for i := range got {
		if strings.Contains(got[i].Message, "viewport") {
			vp = &got[i]
		}
	}
	if vp == nil || vp.Severity != SeverityWarn {
		t.Fatalf("missing viewport should be a warning, got %+v", got)
	}
	if r.HasError() {
		t.Fatalf("viewport alone must not block: %+v", r.Findings)
	}
}

func TestMissingDistErrors(t *testing.T) {
	if _, err := Check(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error when dist/ is absent")
	}
}
