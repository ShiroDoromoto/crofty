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
```

`crofty validate [path ...]` (default `./content`) reports, in field order,
what does not match the spec and how to fix it — in plain language you can act
on by hand or hand to any assistant. It is side-effect-free; `--json` emits the
same findings as structured output for tools. It exits non-zero when any
blocking error is found.

`publish` and `eject` are planned for later milestones.

The bundled theme is static and ships no JavaScript or trackers. The build
output is a plain Hugo project, so you can take it and run `hugo` yourself
without this tool.

## License

See [LICENSE](./LICENSE).
