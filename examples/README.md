# Examples

Focused, single-purpose sites that show the *range* of what crofty builds. Where
[`demo/`](../demo/) is one all-in site that exercises every feature, each example
here is a believable site of one kind — so someone weighing crofty for *their*
use can see it, not infer it.

Each is its own crofty project (own `hugo.yaml`, content, layouts), English-only,
on the frozen theme with a different look. Build/deploy state (`.crofty/`,
`dist/`) is gitignored, same as `demo/`.

| Example | Kind | Look | What it shows |
|---|---|---|---|
| [`portfolio/`](portfolio/) | Photographer | `quiet-paper` preset | Home **is** a landing (a work grid, not a post list); work collection; About; Contact |
| [`band/`](band/) | Band / musician | `terminal` preset | Release grid landing; abc.js sheet music; a Shows table; About; Contact |
| [`shop/`](shop/) | Small shop / maker | default serif | Product cards linking **out** to external checkout (no cart); Legal (incl. 特定商取引法); About; Contact |
| [`studio/`](studio/) | Design & build studio | **ejected tokens** (custom teal/sans) | A designed landing (hero + services + selected work + CTA); `theme eject` token restyle; Work collection |

Across the set: both home shapes (blog-style post list in `demo/` vs designed
landings here), all three section patterns (grid, cards, list), the two restyle
paths (presets and ejected tokens), and the static-edge answers for contact
(mailto / external form) and commerce (external checkout link-out).

## Build or deploy one

```sh
cd examples/portfolio
crofty preview     # see it locally
crofty build       # render to ./dist
crofty deploy      # publish to its own Cloudflare Pages project
```

Each deploys to its own Pages project (`crofty-example-*`); link them from an
"Examples" page on the product site once they're live.
