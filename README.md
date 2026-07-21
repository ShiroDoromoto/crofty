# crofty

Write Markdown; crofty builds it with Hugo and deploys it to a website you own
on your own domain and accounts. It never talks to a server of ours — there
isn't one. Deploys go straight to your Cloudflare account over its API, with no
Node or Wrangler in the loop.

crofty is a plain CLI you can run yourself, but it's built so your AI can run it
for you: a person double-clicks the installer, and an assistant takes it from
there — `crofty init` included. If you work that way, point your assistant at
`crofty agent` first — it
prints crofty's whole command surface (every command with its flags and
examples, the usual workflow, and the machine-readable state to read — `config`,
`features`, `validate` and `doctor` all take `--json`) in one shot, with no
project needed. From there it can drive everything below.

## Install

Download the installer and double-click it. That is the whole installation, and
it is the only part meant for you rather than your assistant.

- **macOS** — [crofty.pkg](https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty.pkg)
- **Windows** — [crofty-setup.exe](https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty-setup.exe)

Both carry [Hugo](https://gohugo.io) with them, so there is nothing to install
first: when the installer finishes, `crofty build` works.

**Your OS will warn you the first time. That is expected** — the installers are
unsigned. Nothing you have to do here needs a terminal:

- **macOS** — the first open is refused: "crofty.pkg cannot be opened because it
  is from an unidentified developer." Open **System Settings → Privacy &
  Security**, scroll to Security, and next to the message about crofty.pkg press
  **Open Anyway**. The button only appears *after* the refusal, so let it happen
  first. Confirm once more, and the installer runs.
- **Windows** — SmartScreen says "Windows protected your PC". Press **More
  info**, then **Run anyway**.

Linux has no click installer. Take the archive below.

### The archive, on any OS

crofty is one file. The
[releases page](https://github.com/ShiroDoromoto/crofty/releases) carries it for
every OS, and your browser is enough to get it:

1. Download the archive that matches your machine —
   `crofty_<version>_windows_amd64.zip`, `crofty_<version>_linux_arm64.tar.gz`,
   and so on.
2. Unpack it. Inside is a single `crofty` binary.
3. Move it wherever you keep such programs. Putting that directory on your PATH
   saves you typing; it is not required, since a full path runs just as well.

Don't unpack it into a `.crofty/` directory. That name belongs to crofty's own
per-site state, and `crofty init` writes there.

### If you'd rather use a terminal

```sh
curl -fsSL https://crofty.site/install.sh | sh                                          # macOS / Linux
irm https://github.com/ShiroDoromoto/crofty/releases/latest/download/install.ps1 | iex  # Windows
```

These are shortcuts for when they work. Both fetch through your system's TLS
stack, and on Windows that stack sometimes declines to supply a credential at
all (`SEC_E_NO_CREDENTIALS`) — the download dies before crofty is anywhere in
the picture. Nothing here can fix that machine; the archive above simply goes
through your browser instead, and arrives.

The script drops crofty in `~/.local/bin` and touches nothing else. For a
system-wide install, name the prefix: `curl -fsSL https://crofty.site/install.sh
| sudo PREFIX=/usr/local sh`.

Each release ships a `crofty_<version>_checksums.txt` if you want to verify what
you downloaded.

**To uninstall**, delete the binary — `rm ~/.local/bin/crofty` (or
`/usr/local/bin/crofty`). crofty keeps no state inside its own install
directory, and your sites are yours: they live where `crofty init` put them, and
nothing removes them but you.

### Hugo

crofty wraps Hugo at runtime for `build` and `preview`, and needs the
**extended** build (the theme's stylesheets are SCSS).

Only the click installers bring their own. They don't touch a `hugo` you already
have: the bundled copy sits next to crofty, off your PATH, and crofty runs it in
preference to whatever PATH happens to name.

Every other route expects a hugo on your PATH, and none of them installs one for
you — a distro's hugo is often outdated or not the extended build, and one that
fails the build is worse than none. Install
[hugo-extended](https://gohugo.io/installation/) yourself; `crofty build` /
`crofty preview` will tell you if it's missing. To point crofty at a particular
one, set `CROFTY_HUGO=/path/to/hugo`.

### Updating

Once crofty is installed, it updates itself:

```sh
crofty update
```

It fetches the latest release and swaps the binary in place — no reinstall, no
admin, and no OS warning (a file crofty downloads itself carries no quarantine
flag). This works for the click installers (`.pkg` / `.exe`) and the per-user
install script. There is no automatic update: it only runs when you (or the AI
driving crofty) ask it to, and it points you at the release notes so the upgrade
stays your call. crofty still tells you when a release is out.

A system-wide install script (`PREFIX=/usr/local`) is root-owned, so `crofty
update` sends you back to re-run the script with `sudo` instead. Either way,
`crofty doctor` shows whether an update will work from your install.

### If you installed with Homebrew, Scoop, or a `.deb` / `.rpm`

Those routes no longer receive updates. Leave, then come back by one of the
routes above:

```sh
brew uninstall crofty && brew untap ShiroDoromoto/crofty   # macOS
scoop uninstall crofty && scoop bucket rm crofty           # Windows
sudo apt remove crofty                                     # Debian/Ubuntu
sudo dnf remove crofty                                     # Fedora/RHEL
```

Then install the `.pkg` / `.exe`, or run the install script. Nothing you wrote
is touched — crofty keeps no state inside its own install directory.

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

Your sites hold everything of yours. Apart from them, crofty keeps one small
state folder — a list of where your projects are, so it can find them from any
directory. It lives in your OS's config folder; set `CROFTY_HOME` to keep it
somewhere else, which is what to do if that folder is locked down. If crofty
cannot write it, your site is still complete; only the finding-it-from-anywhere
part is off. `crofty config` says where that folder is and whether crofty may
write there, so you never have to find out by running something that writes.

## Build from source

Requires Go 1.25+ (and [Hugo](https://gohugo.io) to run it).

```sh
go build -o crofty .
```

## License

See [LICENSE](./LICENSE).
