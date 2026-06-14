# crofty

Write Markdown; crofty builds it with Hugo and deploys it to a website you own
on your own domain and accounts. It never talks to a server of ours — there
isn't one.

## Build from source

Requires Go 1.25+, [Hugo](https://gohugo.io), and Node.js (for Wrangler).

```sh
go build -o crofty .
```

## Create a project

A crofty project is a directory with a standard Hugo config, a `.crofty/`
folder, and your Markdown:

```
your-site/
├── hugo.yaml            # standard Hugo config (baseURL, title, …)
├── .crofty/
│   └── config.json      # crofty settings (deploy target)
└── content/
    └── _index.md        # your home page, in Markdown
```

Minimal `.crofty/config.json`:

```json
{
  "deploy": { "provider": "cloudflare", "project": "your-pages-project" }
}
```

(A `crofty init` that scaffolds this for you is planned.)

## Usage

Run inside a crofty project:

```sh
crofty validate  # check your Markdown against the crofty spec (v0)
crofty build     # render the site to ./dist
crofty deploy    # publish ./dist to your Cloudflare Pages project
crofty share     # print a ready-to-post fragment (text + link) for any SNS
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

`eject` is planned for a later milestone.

The bundled theme is static and ships no JavaScript or trackers. The build
output is a plain Hugo project, so you can take it and run `hugo` yourself
without this tool.

## License

See [LICENSE](./LICENSE).
