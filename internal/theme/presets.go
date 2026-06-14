package theme

// A Preset is a named look `crofty theme set` writes to assets/css/custom.css.
// Each is just a different set of values for the same tokens the default theme
// declares (presets.go and crofty.css share a token set; eject_test.go guards
// it), so switching a preset can only change the look — never the output
// contract. Presets are the cheap proof that a crofty site is yours to reshape.
type Preset struct {
	Name    string
	Summary string
	CSS     string
}

// Presets are the looks crofty ships beyond its default editorial theme. They
// are token-only overrides, so applying one is instant and safe.
func Presets() []Preset {
	return []Preset{
		{Name: "quiet-paper", Summary: "literary serif on warm paper, muted moss accent", CSS: presetQuietPaper},
		{Name: "terminal", Summary: "sans reading column, monospace chrome, functional green", CSS: presetTerminal},
	}
}

// PresetByName returns the named preset, or ok=false if there is no such preset.
func PresetByName(name string) (Preset, bool) {
	for _, p := range Presets() {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// IsShippedCSS reports whether content is exactly one of the token files crofty
// itself writes — the eject default or a preset — i.e. not hand-edited. `theme
// set` uses it to know it can swap presets freely without clobbering real work.
func IsShippedCSS(content string) bool {
	if content == CustomCSS {
		return true
	}
	for _, p := range Presets() {
		if content == p.CSS {
			return true
		}
	}
	return false
}

const presetQuietPaper = `/* Preset: quiet-paper — a literary serif page, warm as paper, with a muted
   moss accent. Swap looks with ` + "`crofty theme set <name>`" + `; delete this file to
   return to the default theme. Restyling can't break the output contract —
   ` + "`crofty doctor`" + ` still guarantees your site owns its content. */

:root {
  --bg: #fcfbf7;        /* page background */
  --ink: #211e1a;       /* body text and headings */
  --muted: #6f6a61;     /* dates, captions, secondary text */
  --line: #e4e0d8;      /* rules, borders, link underlines */
  --accent: #4a6a52;    /* muted moss — links on hover, marks */
  --code-bg: #f2efe7;   /* inline code and code blocks */

  --measure: 34rem;     /* reading-column width */

  --font-body: Charter, "Iowan Old Style", "Palatino Linotype", Palatino, "Book Antiqua", Georgia, serif;
  --font-chrome: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
  --font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #17150f;
    --ink: #e9e5dc;
    --muted: #9a9488;
    --line: #322e27;
    --accent: #8fae93;
    --code-bg: #221f18;
  }
}
`

const presetTerminal = `/* Preset: terminal — a sans reading column with monospace chrome and a
   functional green, in the spirit of the CLI. Swap looks with ` + "`crofty theme set <name>`" + `;
   delete this file to return to the default theme. Restyling can't break the
   output contract — ` + "`crofty doctor`" + ` still guarantees ownership. */

:root {
  --bg: #fbfaf8;        /* page background — barely-warm near-white */
  --ink: #15181b;       /* body text and headings */
  --muted: #5b6168;     /* dates, captions, secondary text */
  --line: #e6e3dd;      /* rules, borders, link underlines */
  --accent: #157a5b;    /* functional green ("go") */
  --code-bg: #f1efe8;   /* inline code and code blocks */

  --measure: 34rem;     /* reading-column width */

  /* Body in system sans; header/footer/meta in monospace for the terminal feel. */
  --font-body: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
  --font-chrome: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  --font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0f1417;
    --ink: #e9efe9;
    --muted: #8a948c;
    --line: #273139;
    --accent: #4fd0a0;
    --code-bg: #161d21;
  }
}
`
