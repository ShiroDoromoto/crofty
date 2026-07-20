package cli

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"reflect"
	"strings"
	"testing"
)

// A worker that reaches for another module can't be carried, and the scan has to
// see it however it is written — including through the words that don't look
// like an import at first glance.
func TestWorkerImportsFindsEveryWayOut(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"static import", `import { Router } from "./router.js"` + "\nexport default {}", "import"},
		{"bare import", `import "./polyfill.js"` + "\nexport default {}", "import"},
		{"dynamic import", "export default { async fetch() { const m = await import('./late.js') } }", "import"},
		{"require", `const fs = require("node:fs")`, "require("},
		{"re-export", `export { handler } from "./handler.js"`, "export … from"},
		{"star re-export", `export * from "./api.js"`, "export … from"},
		{"namespaced re-export", `export * as api from "./api.js"`, "export … from"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := workerImports([]byte(tc.src))
			if len(got) == 0 {
				t.Fatalf("workerImports(%q) found nothing — a worker that would break in production", tc.src)
			}
			if got[0].what != tc.want {
				t.Errorf("what = %q, want %q", got[0].what, tc.want)
			}
		})
	}
}

// The other half of the deal: a self-contained worker must pass. A scan that
// cries wolf sends authors to wrangler for no reason, so the everyday shapes —
// the word inside a string, a comment, `import.meta`, a property called import —
// are not module references.
func TestWorkerImportsPassesSelfContained(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"plain worker", "export default {\n  async fetch(req) { return new Response('ok') }\n}"},
		{"the word in a string", `export default { fetch: () => new Response("<script type=module>import x</script>") }`},
		{"the word in a comment", "// import the router later\n/* require() is not used here */\nexport default {}"},
		{"import.meta", "export default { fetch() { return new Response(import.meta.url) } }"},
		{"a property named import", "export default { fetch(o) { return o.import } }"},
		{"the word in a template", "export default { fetch: () => new Response(`import nothing`) }"},
		{"a from without an export", "const from = 1\nexport default { fetch: () => new Response(String(from)) }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workerImports([]byte(tc.src)); len(got) != 0 {
				t.Errorf("workerImports(%q) = %+v, want none", tc.src, got)
			}
		})
	}
}

// Code interpolated into a template literal is still code: blanking the whole
// template would hide an import from the scan, which is the one direction this
// must never fail in.
func TestWorkerImportsSeesInsideInterpolation(t *testing.T) {
	src := "export default { fetch: () => `${await import('./late.js')}` }"
	if got := workerImports([]byte(src)); len(got) == 0 {
		t.Error("an import inside ${…} was missed")
	}
}

// The line number is the whole value of the report: it is what the author uses
// to find the line to fold into the file.
func TestWorkerImportsReportsLines(t *testing.T) {
	src := "export default {}\n\nconst fs = require('node:fs')\n"
	got := workerImports([]byte(src))
	if len(got) != 1 || got[0].line != 3 {
		t.Fatalf("workerImports = %+v, want one hit on line 3", got)
	}
}

// The bundle is the shape Pages takes a worker in: a form of its own, whose
// Content-Type names the boundary the outer part hides. Get this wrong and the
// upload is accepted as an opaque file that never runs.
func TestBuildWorkerBundleShape(t *testing.T) {
	const src = "export default { fetch: () => new Response('ok') }"
	bundle, contentType, err := buildWorkerBundle([]byte(src), workerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		t.Fatalf("content type %q carries no boundary (err %v)", contentType, err)
	}

	r := multipart.NewReader(strings.NewReader(string(bundle)), params["boundary"])
	seen := map[string]string{}
	types := map[string]string{}
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		seen[p.FormName()] = string(b)
		types[p.FormName()] = p.Header.Get("Content-Type")
	}

	var meta struct {
		MainModule string `json:"main_module"`
		Bindings   []any  `json:"bindings"`
	}
	if err := json.Unmarshal([]byte(seen["metadata"]), &meta); err != nil {
		t.Fatalf("metadata is not JSON: %q", seen["metadata"])
	}
	if meta.MainModule != workerMainModule {
		t.Errorf("main_module = %q, want %q", meta.MainModule, workerMainModule)
	}
	if len(meta.Bindings) != 0 {
		t.Errorf("bindings = %v, want none — crofty names no resources of the author's", meta.Bindings)
	}
	if seen[workerMainModule] != src {
		t.Errorf("module body = %q, want the source verbatim", seen[workerMainModule])
	}
	if got := types[workerMainModule]; got != "application/javascript+module" {
		t.Errorf("module content type = %q, want application/javascript+module", got)
	}
	if strings.Contains(seen["metadata"], "compatibility_date") {
		t.Errorf("metadata = %q, want no compatibility_date — none was declared", seen["metadata"])
	}
}

// A declared runtime has to reach the metadata, or declaring it changed nothing.
// Undeclared has to stay absent rather than travel as an empty string: Pages
// reads a missing key as "use the project's own setting", which is the whole
// point of crofty not having a default of its own.
func TestBuildWorkerBundleCarriesTheCompatibilityDate(t *testing.T) {
	for _, tc := range []struct {
		name, date string
		want       bool
	}{
		{"declared", "2026-07-20", true},
		{"undeclared", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle, contentType, err := buildWorkerBundle([]byte("export default {}"), workerOptions{compatibilityDate: tc.date})
			if err != nil {
				t.Fatal(err)
			}
			meta := bundleMetadata(t, bundle, contentType)
			got, present := meta["compatibility_date"]
			if present != tc.want {
				t.Fatalf("compatibility_date present = %v, want %v (metadata %v)", present, tc.want, meta)
			}
			if tc.want && got != tc.date {
				t.Errorf("compatibility_date = %v, want %q", got, tc.date)
			}
		})
	}
}

// A date crofty can't pass on is caught before the build and the login, where
// the author is still watching — not as an API error at the end of a deploy.
func TestWorkerOptionsValidate(t *testing.T) {
	for _, tc := range []struct {
		date string
		ok   bool
	}{
		{"", true}, // undeclared is a valid answer
		{"2026-07-20", true},
		{"2026-7-20", false},
		{"july 2026", false},
		{"2026-07-20T00:00:00Z", false},
	} {
		err := workerOptions{compatibilityDate: tc.date}.validate()
		if tc.ok && err != nil {
			t.Errorf("validate(%q) = %v, want nil", tc.date, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("validate(%q) = nil, want an error naming the field", tc.date)
		}
	}
}

// requiredEnv holds names. The mistake worth catching is NAME=value, because
// that is a secret landing in a file crofty asks people to commit.
func TestWorkerOptionsValidateRequiredEnv(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		ok   bool
	}{
		{"none declared", nil, true},
		{"plain names", []string{"API_BASE", "_private", "K2"}, true},
		{"a value came along", []string{"API_KEY=sk-live-abc"}, false},
		{"empty entry", []string{""}, false},
		{"a leading digit is not a name", []string{"2FA_SECRET"}, false},
		{"whitespace", []string{"API KEY"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := workerOptions{requiredEnv: tc.env}.validate()
			if tc.ok && err != nil {
				t.Errorf("validate(%q) = %v, want nil", tc.env, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("validate(%q) = nil, want an error naming the field", tc.env)
			}
		})
	}
}

// A declared name the destination doesn't have is the whole point; a declared
// name it does have must stay quiet, or the warning becomes noise to scroll past.
func TestMissingEnv(t *testing.T) {
	for _, tc := range []struct {
		name              string
		required, present []string
		want              []string
	}{
		{"nothing declared", nil, []string{"API_BASE"}, nil},
		{"all present", []string{"API_BASE"}, []string{"OTHER", "API_BASE"}, nil},
		{"none present", []string{"API_BASE", "SIGNING_KEY"}, nil, []string{"API_BASE", "SIGNING_KEY"}},
		{"some present", []string{"API_BASE", "SIGNING_KEY"}, []string{"API_BASE"}, []string{"SIGNING_KEY"}},
		{"names are case-sensitive, as they are on the wire", []string{"API_BASE"}, []string{"api_base"}, []string{"API_BASE"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingEnv(tc.required, tc.present); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("missingEnv(%q, %q) = %q, want %q", tc.required, tc.present, got, tc.want)
			}
		})
	}
}

// bundleMetadata pulls the metadata field out of a worker bundle.
func bundleMetadata(t *testing.T, bundle []byte, contentType string) map[string]any {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	r := multipart.NewReader(strings.NewReader(string(bundle)), params["boundary"])
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if p.FormName() != "metadata" {
			continue
		}
		b, _ := io.ReadAll(p)
		var meta map[string]any
		if err := json.Unmarshal(b, &meta); err != nil {
			t.Fatalf("metadata is not JSON: %q", b)
		}
		return meta
	}
	t.Fatal("the bundle has no metadata field")
	return nil
}
