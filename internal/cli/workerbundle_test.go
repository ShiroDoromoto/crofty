package cli

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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
	bundle, contentType, err := buildWorkerBundle([]byte(src))
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
}
