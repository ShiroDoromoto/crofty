package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// runAdd turns on a capability the bundled theme doesn't ship on by default.
// Two kinds: ones crofty can fully own (a code-block render hook → a file under
// layouts/), and ones that are a hugo.yaml setting (raw HTML, class-based code
// colour) — for those crofty shows the line to paste rather than rewriting the
// author's config, the same stance as `crofty init`/`configure`.
func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite an existing render hook")
	fs.Usage = addUsage
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		addUsage()
		return nil
	}

	switch rest[0] {
	case "mermaid":
		return addRenderHook("mermaid", mermaidHook, *force, mermaidNote)
	case "abc":
		return addRenderHook("abc", abcHook, *force, abcNote)
	case "raw-html":
		printRawHTMLGuidance()
		return nil
	case "highlight":
		printHighlightGuidance()
		return nil
	case "-h", "--help", "help":
		addUsage()
		return nil
	default:
		return fmt.Errorf("unknown feature %q (try: mermaid | abc | highlight | raw-html; or 'crofty features')", rest[0])
	}
}

func addUsage() {
	fmt.Println("crofty add — turn on a capability the default theme doesn't ship on")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crofty add mermaid     # render ```mermaid blocks as diagrams")
	fmt.Println("  crofty add abc         # render ```abc blocks as sheet music")
	fmt.Println("  crofty add highlight   # theme-following code colour (older projects)")
	fmt.Println("  crofty add raw-html    # let raw HTML in Markdown through")
	fmt.Println()
	fmt.Println("See 'crofty features' for the full list of capabilities.")
}

// addRenderHook writes a code-block render hook into the project's layouts. The
// crofty theme stays frozen — Hugo looks the hook up by language, so it overrides
// nothing in the theme. note prints the trade-off (these use client JS).
func addRenderHook(lang, body string, force bool, note func()) error {
	proj, err := findProject()
	if err != nil {
		return err
	}
	rel := filepath.Join("layouts", "_default", "_markup", "render-codeblock-"+lang+".html")
	target := filepath.Join(proj.Root, rel)
	if !force {
		if fileExists(target) {
			fmt.Printf("%s already exists — edit it, or pass --force to reset it.\n", rel)
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		return err
	}

	fmt.Printf("✓ wrote %s\n", rel)
	fmt.Println()
	note()
	fmt.Println()
	fmt.Println("next:")
	fmt.Printf("  write a ```%s block in a post, then 'crofty preview'.\n", lang)
	return nil
}

func mermaidNote() {
	fmt.Println("Heads up: native Mermaid renders in the browser — it loads mermaid.js from a")
	fmt.Println("CDN (client-side JS + an external request). That's at odds with crofty's")
	fmt.Println("static, no-JS, no-trackers default. If you want to keep the page JS-free,")
	fmt.Println("build the diagram to an SVG and embed it with <img> instead of using this hook.")
}

func abcNote() {
	fmt.Println("Heads up: this renders sheet music in the browser — it loads abcjs from a CDN")
	fmt.Println("(client-side JS + an external request), which is at odds with crofty's static,")
	fmt.Println("no-JS, no-trackers default. Drop the hook if you'd rather keep the page JS-free.")
}

func printRawHTMLGuidance() {
	fmt.Println("Raw HTML in Markdown (e.g. <figure>, <video>) is dropped unless you turn it")
	fmt.Println("on. Add this to hugo.yaml:")
	fmt.Println()
	fmt.Println("    markup:")
	fmt.Println("      goldmark:")
	fmt.Println("        renderer:")
	fmt.Println("          unsafe: true")
	fmt.Println()
	fmt.Println("It's \"unsafe\" only in the sense that Hugo passes your HTML through verbatim —")
	fmt.Println("fine for your own content. Then 'crofty preview' to see it.")
}

func printHighlightGuidance() {
	fmt.Println("Theme-following code colour is on by default for new projects. For an older")
	fmt.Println("one, add this to hugo.yaml (the theme already ships the stylesheet):")
	fmt.Println()
	fmt.Println("    markup:")
	fmt.Println("      highlight:")
	fmt.Println("        noClasses: false")
	fmt.Println()
	fmt.Println("Then 'crofty build' (or restart 'crofty preview') and code follows light/dark.")
}

// The render hooks below are project layouts, written verbatim into the user's
// project. They mirror the demo's hooks; the theme never has to know about them.

const mermaidHook = `{{- /*
  Render ` + "```mermaid" + ` fenced blocks as live diagrams.

  Project-level render hook: it overrides nothing in the crofty theme (which
  stays frozen), it only adds a code-block hook Hugo looks up by language.

  Mermaid reads the diagram source from the element's text, so we escape it and
  drop the highlighter. The loader is emitted once per page (guarded by
  Page.Store) and follows the reader's light/dark setting.
*/ -}}
<pre class="mermaid" aria-label="diagram">{{ .Inner | htmlEscape | safeHTML }}</pre>
{{- if not (.Page.Store.Get "hasMermaid") }}
{{- .Page.Store.Set "hasMermaid" true }}
<script type="module">
  import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs";
  const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  mermaid.initialize({ startOnLoad: true, theme: dark ? "dark" : "neutral" });
</script>
{{- end }}
`

const abcHook = `{{- /*
  Render ` + "```abc" + ` fenced blocks as engraved sheet music with a play button,
  using abcjs (https://www.abcjs.net/). Project-level render hook — the crofty
  theme stays untouched.

  The ABC source is stashed in a <script type="text/vnd.abc"> element so the
  browser keeps it verbatim (no HTML parsing, no execution). Each block renders
  into its own paper + audio-controls element. The abcjs library is loaded once
  per page; until it arrives, blocks queue their render and run on load.
*/ -}}
{{- $id := printf "abc-%d" .Ordinal -}}
<figure class="abc-block">
  <div id="{{ $id }}-paper" class="abc-paper"></div>
  <div id="{{ $id }}-audio" class="abc-audio"></div>
  <script type="text/vnd.abc" id="{{ $id }}-src">{{ .Inner | safeHTML }}</script>
  <script>
  (function () {
    function render() {
      var src = document.getElementById("{{ $id }}-src").textContent;
      var visual = ABCJS.renderAbc("{{ $id }}-paper", src, { responsive: "resize" })[0];
      if (ABCJS.synth && ABCJS.synth.supportsAudio()) {
        var ctl = new ABCJS.synth.SynthController();
        ctl.load("#{{ $id }}-audio", null, { displayPlay: true, displayProgress: true });
        ctl.setTune(visual, false);
      } else {
        document.getElementById("{{ $id }}-audio").textContent =
          "Audio playback is not supported in this browser.";
      }
    }
    if (window.ABCJS) { render(); }
    else { (window.__abcQueue = window.__abcQueue || []).push(render); }
  })();
  </script>
</figure>
{{- if not (.Page.Store.Get "hasAbc") }}
{{- .Page.Store.Set "hasAbc" true }}
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/abcjs@6/abcjs-audio.css">
<script
  src="https://cdn.jsdelivr.net/npm/abcjs@6/dist/abcjs-basic-min.js"
  onload="(window.__abcQueue || []).forEach(function (f) { f(); });"></script>
{{- end }}
`
