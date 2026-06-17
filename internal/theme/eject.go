package theme

import (
	"io/fs"
	"os"
	"path/filepath"
)

// CustomCSSPath is where `crofty theme eject` writes the starter token override,
// relative to the project root. It lives under assets/ so Hugo's resources.Get
// picks it up (baseof links it only when present) and so it sits on the
// documented customization seam (assets/, layouts/).
const CustomCSSPath = "assets/css/custom.css"

// CustomCSS is the starter override `crofty theme eject` writes. It mirrors the
// :root tokens in the bundled crofty.css with their default values, so editing
// it is the safe, documented way to restyle: it loads after crofty.css (see
// baseof.html), so whatever is set here wins, and it can only touch design —
// `crofty doctor` still guarantees the site owns its content. The token NAMES
// here must stay in sync with crofty.css; eject_test.go guards against drift.
const CustomCSS = `/* Your theme's editable surface.
   These variables ARE the official way to restyle a crofty site. Edit freely —
   this file loads after the bundled theme, so whatever you set here wins. No
   matter how you change it, ` + "`crofty doctor`" + ` confirms your site still owns
   its content: a canonical link, a feed, and no third-party tracking.

   Want to change more than colour and type? ` + "`crofty theme eject --full`" + `
   writes the whole theme (layouts + CSS) into your project to edit directly. */

:root {
  --bg: #fcfbf7;        /* page background */
  --ink: #211e1a;       /* body text and headings */
  --muted: #6f6a61;     /* dates, captions, secondary text */
  --line: #e4e0d8;      /* rules, borders, link underlines */
  --accent: #5c4b37;    /* links on hover, active marks */
  --code-bg: #f2efe7;   /* inline code and code blocks */

  --measure: 46rem;     /* reading-column width */

  /* Point --font-body at var(--font-mono) for a terminal feel, or drop in any
     family. --font-chrome is the header/footer/meta face. */
  --font-body: Charter, "Iowan Old Style", "Palatino Linotype", Palatino, "Book Antiqua", Georgia, serif;
  --font-chrome: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
  --font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

/* Dark mode follows the reader's system setting. Delete this block to opt out,
   or set your own dark palette here. */
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #17150f;
    --ink: #e9e5dc;
    --muted: #9a9488;
    --line: #322e27;
    --accent: #c9b79a;
    --code-bg: #221f18;
  }
}
`

// EjectFull writes the whole bundled theme into dst (a project root) as the
// override layer: layouts/, static/, i18n/ land at the same paths Hugo overrides
// from, so they take effect immediately while the project stays a crofty project.
// theme.toml is skipped — it is theme metadata, unused at the project root.
// Existing files are left untouched unless force is set, so re-running never
// clobbers edits. Returns the relative paths written and skipped.
func EjectFull(dst string, force bool) (written, skipped []string, err error) {
	src := FS()
	err = fs.WalkDir(src, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || p == "theme.toml" {
			return nil
		}
		target := filepath.Join(dst, p)
		if !force {
			if _, statErr := os.Stat(target); statErr == nil {
				skipped = append(skipped, p)
				return nil
			}
		}
		b, readErr := fs.ReadFile(src, p)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(target), 0o755); mkErr != nil {
			return mkErr
		}
		if wErr := os.WriteFile(target, b, 0o644); wErr != nil {
			return wErr
		}
		written = append(written, p)
		return nil
	})
	return written, skipped, err
}
