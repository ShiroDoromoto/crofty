# packaging — unsigned click installers

The **primary** way a person installs crofty: download, double-click, done. It is
the one step we ask of someone who never opens a terminal — the assistant takes
over from `crofty init` on. The install scripts (`install.sh` / `install.ps1`)
remain, for people who do open one.

- **macOS**: `macos/build-pkg.sh` → a universal `crofty.pkg`. Its postinstall
  splits the install into an entry and a body (D-339): a link at
  `/usr/local/bin/crofty` (on the default PATH), and the body — crofty plus its
  bundled Hugo — in `~/Library/Application Support/crofty`, owned by the user so
  `crofty update` can replace it later with no root. Standard installer (asks for
  the password once, to write the entry).
- **Windows**: `windows/installer.nsi` + `windows/build-exe.sh` → `crofty-setup.exe`
  that installs per-user to `%LOCALAPPDATA%\crofty\bin` and adds it to the user
  PATH (no admin). Already user-writable, so D-339's entry/body split is a no-op
  here — nothing to change.

Both are **unsigned by choice** — no Apple Developer ID, no code-signing cert, no
P12. First open shows an OS warning (Gatekeeper / SmartScreen); the user picks
"Open Anyway" / "Run anyway". The agent guides them through it.

## The bundled Hugo

Both installers carry Hugo (extended), fetched and checksum-verified at build
time by `hugo.sh` — which also pins the version. Someone who double-clicks has no
prerequisite to install, and `crofty build` works the moment the installer
finishes. crofty runs *this* Hugo ahead of any on PATH (`internal/hugobin`): a
stray `hugo` says nothing about its version or flavor, and the frozen theme needs
the extended build.

Neither installer disturbs a hugo the author already has.

- macOS: in the body, at `~/Library/Application Support/crofty/libexec/crofty/hugo`
  — deliberately **off** PATH (shared ground: Intel Homebrew's hugo lives in
  `/usr/local/bin`). crofty finds it from its own body, following the entry link
  there first (`internal/hugobin`).
- Windows: beside `crofty.exe`, in a directory the installer owns outright.

Hugo's macOS build ships as a `.pkg` only, so `hugo.sh` unwraps it with `pkgutil`
to get at the universal binary. Hugo is Apache-2.0; `hugo/LICENSE-hugo.txt` rides
along next to the binary in both installers.

Bump `HUGO_VERSION` in `hugo.sh` deliberately. The weekly `hugo-compat.yml` run is
what tells us a newer Hugo still builds a contract-clean site.

## Distribution

Not via wharfy channels: declaring wharfy `bundle:` flips the release into BYO
mode and drops the Go cross-build (breaks the CLI channels), and `prebuilt` is
Pro-only. So the installers are attached to the GitHub Release as **extra assets**
(same place as `install.sh`), via `gh release upload`. Download URLs:

- `https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty.pkg`
- `https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty-setup.exe`

## Update payloads (for `crofty update`)

The installers are how a person gets crofty the first time. `crofty update`
(D-339) is how the body updates after that — it self-fetches and swaps only the
body in the user's home, never re-running the installer (that is what keeps root,
and Gatekeeper, out of the update). So the same body the installers drop is also
published as a machine-fetchable archive, one per bundled-Hugo route, at fixed
`latest/download` names so update needs no tag lookup:

- `…/releases/latest/download/crofty-body-darwin-universal.tar.gz` — a tree:
  `bin/crofty` beside `libexec/crofty/hugo` (what the .pkg lays in the home).
- `…/releases/latest/download/crofty-body-windows-amd64.zip` — flat: `crofty.exe`
  beside `hugo.exe` (what the .exe lays in `%LOCALAPPDATA%\crofty\bin`).
- `…/releases/latest/download/crofty-body-checksums.txt` — sha256 in `shasum -a
  256` format; update reads its route's hash by asset name and verifies before
  swapping.

The install-script route carries no Hugo, so it needs no body of its own: an
update there reuses wharfy's binary archive. This script only produces the
payloads; `crofty update` (internal/cli) consumes them — including on Windows,
where an in-use `.exe` can't overwrite itself, so update renames the running one
aside before dropping the new one in.

## Release procedure

These are built in public CI, not on a laptop: pushing a tag runs
`.github/workflows/release.yml` on a macOS runner (the `.pkg` needs
`pkgbuild`/`lipo`/`codesign`, which exist nowhere else), and that workflow calls
`release-installers.sh` right after `wharfy build`, since the script reads the
binaries wharfy just produced in `.wharfy/dist`. The tag only produces a
*prerelease*; `promote.yml`, dispatched by hand, is what ships it.

```sh
packaging/release-installers.sh X.Y.Z [outdir]   # from repo root, after `wharfy build`
```

It builds a universal `.pkg` (arm64+amd64, ad-hoc signed) and the amd64 `.exe`,
each with its Hugo, plus the `crofty update` payloads (the same bodies, as a
tar.gz / zip with a sha256 checksums file), then attaches them all to the release
with `--clobber`. Rare win/arm64 users fall back to the install script. The
`.pkg` runs about 47 MB and the `.exe` about 28 MB — Hugo is most of that.
Passing `outdir` keeps the files after the upload, which is how CI attests their
build provenance: wharfy never sees these assets, and provenance covering only
part of a release is rejected by `wharfy verify`.

Running it by hand needs a macOS host with `makensis` and `gh` — and is only for
when CI is down, because a laptop build carries no provenance at all.

## Known limitation

The NSIS uninstaller removes the files but not the PATH entry (a stale entry to a
deleted dir — harmless; Windows ignores missing PATH dirs).
