// Package contract is the crofty output contract (08 §2) expressed as code: the
// invariants `crofty doctor` checks the built site (dist/) against, the
// counterpart to package spec's input (Markdown) contract. It guards the
// must-not-break elements of crofty's promise — canonical links, a feed, no
// phone-home — so an agent can freely restyle the theme and any customization
// that breaks ownership/portability is caught before deploy, not after.
//
// Only Error blocks deploy; Warn is advisory. The checks read the built HTML,
// not the templates: crofty constrains outputs, never how you wrote them.
package contract

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Severity ranks a finding. Only Error blocks deploy.
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
)

// Finding is one contract result about one file (File is "" for site-level
// findings). Check is the contract id (e.g. "C-E1") so reports are referenceable.
type Finding struct {
	File     string   `json:"file"`
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix"`
}

// Report is the outcome of checking a built site. OK is false if any Error.
type Report struct {
	OK       bool      `json:"ok"`
	Findings []Finding `json:"findings"`
}

// HasError reports whether the report contains a blocking (Error) finding.
func (r Report) HasError() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

var ulidRe = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`) // Crockford base32, 26 chars

// croftyManagedDomains are hosts crofty itself would phone home to. crofty runs
// no servers and sends no telemetry (A7); this list is the guard that keeps it
// that way — if a build ever beacons to a crofty-controlled host, C-E4 fails.
// Empty by design. (Destinations the author configures — GA, their Stripe page —
// are theirs, not crofty's, and are never checked here.)
var croftyManagedDomains = []string{}

// Check walks a built site at distDir and returns its contract findings.
func Check(distDir string) (Report, error) {
	fi, err := os.Stat(distDir)
	if err != nil || !fi.IsDir() {
		return Report{}, fmt.Errorf("no build output at %s — run 'crofty build' first", distDir)
	}

	var findings []Finding

	// C-E2 (site level): a feed must exist. Hugo emits dist/index.xml by default;
	// its absence means RSS output was turned off — a real syndication break.
	if _, err := os.Stat(filepath.Join(distDir, "index.xml")); err != nil {
		findings = append(findings, Finding{
			Check: "C-E2", Severity: SeverityError,
			Message: "no RSS feed (index.xml) in the build",
			Fix:     "Re-enable RSS output (Hugo emits index.xml by default) so readers and crawlers can follow the site.",
		})
	}

	err = filepath.WalkDir(distDir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			return nil
		}
		rel, _ := filepath.Rel(distDir, p)
		rel = filepath.ToSlash(rel)
		facts, perr := parsePage(p)
		if perr != nil {
			findings = append(findings, Finding{
				File: rel, Check: "parse", Severity: SeverityWarn,
				Message: "could not parse HTML: " + perr.Error(),
			})
			return nil
		}
		findings = append(findings, checkPage(rel, facts)...)
		return nil
	})
	if err != nil {
		return Report{}, err
	}

	return Report{OK: !hasError(findings), Findings: findings}, nil
}

func hasError(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// pageFacts is what a single built page tells us, gathered in one HTML pass.
type pageFacts struct {
	htmlLang      string
	title         string
	canonical     string
	feedLink      bool
	viewport      bool
	croftyID      string
	haveID        bool
	resourceHosts []string
}

// checkPage turns one page's facts into findings.
func checkPage(rel string, f pageFacts) []Finding {
	var out []Finding
	add := func(check string, sev Severity, msg, fix string) {
		out = append(out, Finding{File: rel, Check: check, Severity: sev, Message: msg, Fix: fix})
	}

	// C-E6: language, title, viewport.
	if strings.TrimSpace(f.htmlLang) == "" {
		add("C-E6", SeverityError, "<html> has no lang attribute",
			"Set the site language (crofty init --lang, or 'locale:' in hugo.yaml).")
	}
	if strings.TrimSpace(f.title) == "" {
		add("C-E6", SeverityError, "page has no <title>",
			"Ensure the layout emits <title> (the default theme does).")
	}
	if !f.viewport {
		add("C-E6", SeverityWarn, "no viewport meta",
			`Add <meta name="viewport" content="width=device-width, initial-scale=1">.`)
	}

	// C-E1: canonical link on every page but the 404.
	if rel != "404.html" && strings.TrimSpace(f.canonical) == "" {
		add("C-E1", SeverityError, "no <link rel=\"canonical\">",
			"The default theme emits it via partial crofty/head.html — restore it if a custom layout dropped it.")
	}

	// C-E2: the home page should advertise the feed (discoverability).
	if rel == "index.html" && !f.feedLink {
		add("C-E2", SeverityWarn, "home page does not link the feed",
			`Add <link rel="alternate" type="application/rss+xml" href="/index.xml">.`)
	}

	// C-E3: a crofty:id that is present must be a valid ULID. (Presence isn't
	// required — ids are assigned at publish time; see doctor's deferred note.)
	if f.haveID && !ulidRe.MatchString(strings.ToUpper(strings.TrimSpace(f.croftyID))) {
		add("C-E3", SeverityError, "crofty:id is not a valid ULID",
			"Remove the meta or restore the original id (crofty assigns it on first publish).")
	}

	// C-E4: no resource may load from a crofty-controlled host (no phone-home).
	for _, h := range f.resourceHosts {
		for _, m := range croftyManagedDomains {
			if h == m || strings.HasSuffix(h, "."+m) {
				add("C-E4", SeverityError, "a resource loads from a crofty-controlled host ("+h+")",
					"crofty must not phone home — remove this resource.")
			}
		}
	}

	return out
}

// parsePage reads one built HTML file and collects the facts checkPage needs.
func parsePage(path string) (pageFacts, error) {
	f, err := os.Open(path)
	if err != nil {
		return pageFacts{}, err
	}
	defer f.Close()
	doc, err := html.Parse(f)
	if err != nil {
		return pageFacts{}, err
	}

	var pf pageFacts
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "html":
				pf.htmlLang = attr(n, "lang")
			case "title":
				if pf.title == "" {
					pf.title = strings.TrimSpace(textOf(n))
				}
			case "link":
				rel, typ, href := attr(n, "rel"), attr(n, "type"), attr(n, "href")
				if strings.EqualFold(rel, "canonical") && pf.canonical == "" {
					pf.canonical = href
				}
				if strings.Contains(strings.ToLower(typ), "rss") {
					pf.feedLink = true
				}
				if h := host(href); h != "" {
					pf.resourceHosts = append(pf.resourceHosts, h)
				}
			case "meta":
				switch strings.ToLower(attr(n, "name")) {
				case "viewport":
					pf.viewport = true
				case "crofty:id":
					pf.croftyID, pf.haveID = attr(n, "content"), true
				}
			case "script", "img", "iframe", "source":
				if h := host(attr(n, "src")); h != "" {
					pf.resourceHosts = append(pf.resourceHosts, h)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return pf, nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

// host returns the host of an absolute URL, or "" for a relative URL (which is
// same-origin and never a phone-home).
func host(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
