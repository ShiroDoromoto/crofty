# crofty

Write Markdown; crofty builds it with Hugo and deploys it to a website you own
on your own domain and accounts. It never talks to a server of ours — there
isn't one. Deploys go straight to your Cloudflare account over its API, with no
Node or Wrangler in the loop.

crofty is a plain CLI you can run yourself, but it's built so your AI can run it
for you: a person installs it and does the first setup, then an assistant takes
over. If you work that way, point your assistant at `crofty agent` first — it
prints crofty's whole command surface (every command with its flags and
examples, the usual workflow, and the machine-readable state to read — `config`,
`features`, `validate` and `doctor` all take `--json`) in one shot, with no
project needed. From there it can drive everything below.

## Install

**macOS** ([Homebrew](https://brew.sh)):

```sh
brew install ShiroDoromoto/crofty/crofty
```

**Windows** ([Scoop](https://scoop.sh)):

```sh
scoop bucket add crofty https://github.com/ShiroDoromoto/scoop-crofty
scoop install crofty
```

**Linux** (`.deb` / `.rpm` from the [releases page](https://github.com/ShiroDoromoto/crofty/releases)):

```sh
# Debian/Ubuntu
sudo dpkg -i crofty_*_linux_amd64.deb
# Fedora/RHEL
sudo rpm -i crofty_*_linux_amd64.rpm
```

crofty wraps [Hugo](https://gohugo.io) at runtime for `build` and `preview`.
On macOS and Windows both installers pull it in for you — Homebrew as a formula
dependency, Scoop as a manifest dependency (`hugo-extended` from the main
bucket). On Linux the `.deb`/`.rpm` only *recommends* hugo (distro packages are
often outdated or not the extended build), so install
[hugo-extended](https://gohugo.io/installation/linux/) yourself —
`crofty build` / `crofty preview` will tell you if it's missing from your PATH.

## Quick start

First, start a project (a website you own) — pick where it goes:

```sh
crofty init          # asks for a name, then creates it under ~/Documents/Crofty/
crofty init <name>   # a bare name lands in ~/Documents/Crofty/<name>
crofty init .        # use the current folder (a path or absolute dir works too)
```

Then build and publish it:

```sh
cd <the path it prints>   # crofty init prints where it created the site
crofty preview     # see it in a browser — no account needed
crofty connect     # save your Cloudflare API token (to your keychain)
crofty deploy      # build the current site and publish it to your own Cloudflare Pages
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

To change settings later, run `crofty init` **inside the project** (or point it
at one). It prompts for an optional support link; analytics and the site title
are settings you (or your AI) edit directly in `hugo.yaml` / `data/profile.yml`
— it shows where, and never rewrites them for you. The build output is a plain
Hugo project, so you can take it and run `hugo` yourself without this tool.

## More than a blog

The sample project starts as a blog (`content/posts/`), but a crofty site is a
whole Hugo site — the homepage bits an author rarely escapes are pages too. Two
kinds, both drawn by the same frozen theme:

- **Fixed pages** you maintain — about, contact, access, legal — are a Markdown
  file at `content/<slug>/index.md`.
- **Collections** that grow like the blog — a gallery, a shop, a discography —
  are a section folder: an `_index.md` plus one folder per item.

Put any of them in the top navigation through `hugo.yaml`'s `menu.main` (crofty
prints the lines to paste; it never rewrites `hugo.yaml`). Contact and commerce
stay external on a static site — embed a form (Formspree, Tally) or link out to
a checkout (Stripe, BOOTH). `crofty agent` prints these recipes in full: the two
page kinds, the menu snippet, and where the external pieces go.

## Commands

```sh
crofty init       # create a new project (or re-run to configure an existing one)
crofty agent      # print the whole command surface for an AI to read first
crofty features   # list what crofty can do and how to turn each on
crofty config     # show this project's current configuration
crofty add        # turn on a capability (mermaid, abc, highlight, raw-html, analytics)
crofty lang       # add or list the languages your site is written in
crofty preview    # see your site in a browser (local, no account)
crofty build      # render the site to ./dist with Hugo (to inspect it)
crofty connect    # save the Cloudflare API token used to deploy
crofty deploy     # build the current site and publish it to Cloudflare Pages
crofty analytics  # read your traffic (GA4) and search performance (Search Console)
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

`crofty analytics` reads your own traffic from the command line: GA4 (who
visited, what they read) and Search Console (search queries, pages, sitemaps),
as a plain table or `--json` for an assistant. It uses your own Google
service-account key, kept in your OS keychain, and talks to Google's APIs
directly — there is no server of ours in between. Set the property in
`hugo.yaml` (`params.crofty.analytics`); `crofty analytics status` walks you
through the one-time setup and tells you the next missing step.

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
