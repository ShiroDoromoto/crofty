# Examples

A gallery of focused, single-purpose sites showing the *range* of what crofty
builds — so someone weighing crofty for *their* use can see it, not infer it.
Each is its own crofty project (own `hugo.yaml`, content, layouts) on the frozen
theme with a different look. Build/deploy state (`.crofty/`, `dist/`) is
gitignored.

| Example | Kind | Look | What it shows |
|---|---|---|---|
| [`blog/`](blog/) | Writer's blog | default serif, en/ja | The flagship — a rich blog: images, video, code, Mermaid, abc.js, embeds, RSS, tags, i18n, plus About / Links / Contact / Legal pages. Live at **demo.crofty.site** |
| [`portfolio/`](portfolio/) | Photographer | `quiet-paper` preset | Home **is** a landing (a work grid, not a post list); work collection; About; Contact |
| [`band/`](band/) | Band / musician | `terminal` preset | Release-grid landing; abc.js sheet music; a Shows table; About; Contact |
| [`shop/`](shop/) | Small shop / maker | default serif | Product cards linking **out** to external checkout (no cart); Legal (incl. 特定商取引法); About; Contact |
| [`studio/`](studio/) | Design & build studio | **ejected tokens** (teal/sans) | A designed landing (hero + services + selected work + CTA); `theme eject` token restyle; Work collection |

Across the set: both home shapes (a post-list front page in `blog/` vs designed
landings), all three section patterns (grid, cards, list), both restyle paths
(presets and ejected tokens), bilingual (`blog/`) and English-only (the rest),
and the static-edge answers for contact (mailto / external form) and commerce
(external checkout link-out).

## Build or deploy one

```sh
cd examples/portfolio
crofty preview     # see it locally
crofty build       # render to ./dist
crofty deploy      # publish to its own Cloudflare Pages project
```

`blog/` deploys to the `crofty-demo` project (→ demo.crofty.site); each other
example to its own `crofty-example-*` project. They're linked from the Examples
page on the product site.
