# crofty

Write Markdown; crofty builds it with Hugo and deploys it to a website you own
on your own domain and accounts. It never talks to a server of ours — there
isn't one. Deploys go straight to your Cloudflare account over its API, with no
Node or Wrangler in the loop.

## Install

**macOS** ([Homebrew](https://brew.sh)):

```sh
brew install shirodoromoto/tap/crofty
```

**Windows** ([Scoop](https://scoop.sh)):

```sh
scoop bucket add crofty https://github.com/shirodoromoto/scoop-crofty
scoop install crofty
```

crofty wraps [Hugo](https://gohugo.io) at runtime for `build` and `preview`.
Homebrew installs it as a dependency; on Windows, install it alongside crofty
with `scoop install hugo-extended`.

## Quick start

```sh
crofty init        # create a project (a website you own)
cd <the path it prints>
crofty preview     # see it in a browser — no account needed
crofty connect     # save your Cloudflare API token (to your keychain)
crofty deploy      # publish ./dist to your own Cloudflare Pages
```

`crofty init` scaffolds a standard Hugo project plus a `.crofty/` folder:

```
your-site/
├── hugo.yaml            # standard Hugo config (baseURL, title, …)
├── .crofty/
│   └── config.json      # crofty settings (deploy target) — never your content, no secrets
└── content/
    ├── _index.md        # your home page, in Markdown
    └── posts/
        └── welcome/
            └── index.md # a sample post to edit or delete
```

Run `crofty init` again inside a project to add optional settings (a support
link, analytics). The build output is a plain Hugo project, so you can take it
and run `hugo` yourself without this tool.

## Commands

```sh
crofty init       # create a new project (or re-run to configure an existing one)
crofty preview    # see your site in a browser (local, no account)
crofty build      # render the site to ./dist with Hugo
crofty connect    # save the Cloudflare API token used to deploy
crofty deploy     # publish ./dist to your Cloudflare Pages project
crofty validate   # check content against the crofty spec (v0)
crofty doctor     # check the built site against the output contract
crofty share      # print a ready-to-post fragment (text + link) for any SNS
crofty theme      # bring the theme onto disk to customize (eject tokens or full)
crofty reset      # remove saved credentials (keychain) and state
```

`crofty validate [path ...]` (default `./content`) reports, in field order,
what does not match the spec and how to fix it — in plain language you can act
on by hand or hand to any assistant. It is side-effect-free; `--json` emits the
same findings as structured output for tools. It exits non-zero when any
blocking error is found.

`crofty share <post>` composes a ready-to-post fragment — the title, a summary,
and a link back to your site — for each channel in the post's `crofty.targets`
(or `--to`). The body is never included: the summary and link are all you give
away, so readers come to your site for the rest. It touches no credentials and
posts nothing — it prints the text (and, where a platform has one, a pre-filled
compose link) for you or your agent to paste or open. `--json` emits the same
fragments as structured output; `--plain` prints just the text and link.

A draft stays off your published site: add `draft: true` to a post's
frontmatter, or give it a future `date` to schedule it ahead. `crofty build`
lists any posts it leaves out, so nothing disappears silently.

`crofty eject` (convert to a plain Hugo project) is planned for a later
milestone.

The bundled theme is static and ships no JavaScript or trackers.

## Build from source

Requires Go 1.25+ (and [Hugo](https://gohugo.io) to run it).

```sh
go build -o crofty .
```

## License

See [LICENSE](./LICENSE).
