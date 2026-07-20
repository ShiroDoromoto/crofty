package cli

// Carrying a finished worker to the edge.
//
// crofty runs no bundler and owns no routing convention: it carries a worker the
// author finished, it does not build one. So the worker it takes is a single
// self-contained ES module, and this file holds the two halves of that deal.
//
//   - workerImports reads the source and reports every place it reaches for
//     another module. Any hit stops the deploy — an unresolved import is a worker
//     that uploads cleanly and then fails in production.
//   - buildWorkerBundle wraps the module in the shape Pages takes it in: a
//     multipart form of its own, serialized into the outer deployment's
//     _worker.bundle field. It is the same form wrangler builds, and Go's
//     mime/multipart is all it needs — no dependency comes with this.
//
// The scan is deliberately crude, because crofty has no JS parser and the two
// ways of being wrong are not equal. Refusing a worker that would have worked
// leaves the author a way out (put it in one file, or deploy with wrangler);
// carrying one that doesn't leaves a broken endpoint in production. So it errs
// toward refusing.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"regexp"
	"sort"
	"strings"
)

// workerMainModule is the module's name inside the bundle. Pages requires it to
// match the file Pages itself expects at the site root.
const workerMainModule = "_worker.js"

// workerImport is one place a worker reaches outside itself, named the way the
// author will recognise it in their own source.
type workerImport struct {
	line int
	what string
}

// workerModuleRefs are the ways a module pulls in another one. Each pattern runs
// over source whose comments and string literals have been blanked out, so the
// word "import" inside an HTML template is not mistaken for one.
var workerModuleRefs = []struct {
	what string
	re   *regexp.Regexp
}{
	// Static `import x from …`, bare `import "…"`, and dynamic `import(…)` alike.
	// The trailing group is how import.meta — which reads the module's own URL and
	// pulls in nothing — is told apart from the rest.
	{"import", regexp.MustCompile(`\bimport\b\s*(\.?)`)},
	{"require(", regexp.MustCompile(`\brequire\s*\(`)},
	// `export { x } from "…"` and `export * from "…"` hand another module's
	// exports on, so they are imports wearing a different word. Only the two
	// forms that can take a source are matched — a plain `export default { …
	// from … }` says nothing about another module, and being crude there would
	// refuse the most ordinary worker there is.
	{"export … from", regexp.MustCompile(`\bexport\s*(\*(\s+as\s+[\w$]+)?|\{[^}]*\})\s*from\b`)},
}

// workerImports lists every module reference in src, in source order. An empty
// result is the only shape of worker crofty carries.
func workerImports(src []byte) []workerImport {
	code := blankJSLiterals(src)
	type hit struct {
		at   int
		what string
	}
	var hits []hit
	for _, ref := range workerModuleRefs {
		for _, m := range ref.re.FindAllStringSubmatchIndex(code, -1) {
			// `obj.import` is a property read, not a keyword.
			if precededByDot(code, m[0]) {
				continue
			}
			// import.meta: the one `import` that brings nothing with it.
			if len(m) > 3 && m[2] >= 0 && code[m[2]:m[3]] == "." {
				continue
			}
			hits = append(hits, hit{at: m[0], what: ref.what})
		}
	}
	// In source order, so the author reads them the way their file is written
	// rather than grouped by pattern.
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })

	var out []workerImport
	for _, h := range hits {
		out = append(out, workerImport{line: 1 + strings.Count(code[:h.at], "\n"), what: h.what})
	}
	return out
}

// precededByDot reports whether the token at i is a property access.
func precededByDot(code string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch code[j] {
		case ' ', '\t', '\r', '\n':
			continue
		case '.':
			return true
		default:
			return false
		}
	}
	return false
}

// blankJSLiterals replaces the bytes of comments and string literals with
// spaces, keeping every newline so a match still reports the right line. It is
// not a JS lexer: it is the smallest thing that keeps prose out of the scan
// without hiding code from it.
//
// A quoted string that never closes on its own line is left alone, because the
// quote was almost certainly a regular expression (`/"/`) rather than the start
// of a string — blanking from there would swallow real code, which is the one
// direction this scan must not fail in. Template literals do span lines, so
// those are followed properly, and the code inside `${…}` is left visible.
func blankJSLiterals(src []byte) string {
	out := append([]byte(nil), src...)
	blank := func(i int) {
		if out[i] != '\n' {
			out[i] = ' '
		}
	}
	// tmpl counts the `${…}` interpolations open inside the current template
	// literal; -1 means no template literal is open.
	tmpl := -1

	for i := 0; i < len(src); i++ {
		switch {
		case tmpl >= 0:
			switch {
			case src[i] == '\\':
				blank(i)
				if i+1 < len(src) {
					i++
					blank(i)
				}
			case src[i] == '$' && i+1 < len(src) && src[i+1] == '{':
				tmpl++
				i++ // the interpolation's code stays readable
			case src[i] == '}' && tmpl > 0:
				tmpl--
			case src[i] == '`' && tmpl == 0:
				blank(i)
				tmpl = -1
			case tmpl == 0:
				blank(i)
			}
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '/':
			for ; i < len(src) && src[i] != '\n'; i++ {
				blank(i)
			}
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '*':
			open := i
			blank(i)
			for i++; i < len(src); i++ {
				blank(i)
				// `/*/` is not a closed comment: the same star can't do both ends.
				if src[i] == '/' && i > open+2 && src[i-1] == '*' {
					break
				}
			}
		case src[i] == '`':
			blank(i)
			tmpl = 0
		case src[i] == '\'' || src[i] == '"':
			if end, ok := closingQuote(src, i); ok {
				for ; i <= end; i++ {
					blank(i)
				}
				i--
			}
		}
	}
	return string(out)
}

// closingQuote finds the matching quote for the one at i, on that line only.
func closingQuote(src []byte, i int) (int, bool) {
	q := src[i]
	for j := i + 1; j < len(src) && src[j] != '\n'; j++ {
		if src[j] == '\\' {
			j++
			continue
		}
		if src[j] == q {
			return j, true
		}
	}
	return 0, false
}

// workerOptions are the author's declarations about how their worker runs,
// read from .crofty/config.json. crofty carries them; it does not fill them in.
type workerOptions struct {
	// compatibilityDate pins the Workers runtime (YYYY-MM-DD). Empty is
	// undeclared, and stays undeclared on the wire.
	compatibilityDate string

	// requiredEnv names the environment variables the worker needs. Names only:
	// they are compared against the destination and never sent anywhere.
	requiredEnv []string
}

// workerDateFormat is how Cloudflare writes a compatibility date, and the only
// shape crofty will pass on.
var workerDateFormat = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// workerEnvName is the shape of an environment variable name, which is all
// crofty will accept in requiredEnv — anything else is a value that wandered in.
var workerEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validate checks the declarations crofty can check without the network, so a
// typo is caught before a build and a login rather than as an API error at the
// end of a deploy.
func (o workerOptions) validate() error {
	if o.compatibilityDate != "" && !workerDateFormat.MatchString(o.compatibilityDate) {
		return fmt.Errorf("deploy.worker.compatibilityDate is %q — it has to be a date like 2026-07-20", o.compatibilityDate)
	}
	for _, name := range o.requiredEnv {
		if workerEnvName.MatchString(name) {
			continue
		}
		// The likely mistake is writing NAME=value, and that one has to be
		// caught rather than trimmed: a value in a config file is a secret in a
		// file, which is the thing this field exists to avoid.
		return fmt.Errorf("deploy.worker.requiredEnv has %q — put the variable's name there and nothing else.\n"+
			"  crofty compares names with the destination; the value belongs where the destination keeps secrets", name)
	}
	return nil
}

// missingEnv returns the declared names that the destination does not have, in
// the order they were declared — the author's own order, which is the one they
// can scan against their config file.
func missingEnv(required, present []string) []string {
	if len(required) == 0 {
		return nil
	}
	have := make(map[string]bool, len(present))
	for _, name := range present {
		have[name] = true
	}
	var missing []string
	for _, name := range required {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// buildWorkerBundle wraps a module in the Workers script upload form and returns
// the serialized bytes plus the Content-Type that names its boundary. The caller
// must send that content type with the part, or the receiving end cannot find
// the form inside it.
//
// bindings is sent empty: a binding is a resource in the author's own Cloudflare
// account, and crofty does not create or name those (D-332).
func buildWorkerBundle(src []byte, opts workerOptions) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	meta := map[string]any{
		"main_module": workerMainModule,
		"bindings":    []any{},
	}
	// Absent, not empty: sending "" would be crofty answering a question the
	// author didn't, and Pages reads a missing key as "use the project setting".
	if opts.compatibilityDate != "" {
		meta["compatibility_date"] = opts.compatibilityDate
	}
	metadata, err := json.Marshal(meta)
	if err != nil {
		return nil, "", err
	}
	if err := w.WriteField("metadata", string(metadata)); err != nil {
		return nil, "", err
	}

	// The module itself: field name, file name and main_module are all the same
	// string, which is how the form says which module is the entry point.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, workerMainModule, workerMainModule))
	h.Set("Content-Type", "application/javascript+module")
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(src); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}
