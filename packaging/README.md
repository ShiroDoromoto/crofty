# packaging — unsigned click installers

The double-click **fallback** for users whose AI agent can't install crofty over
the shell. The primary path stays the install script (`install.sh` / `install.ps1`);
these installers are for people who can only download-and-double-click.

- **macOS**: `macos/build-pkg.sh` → a universal `crofty.pkg` that installs the
  `crofty` binary to `/usr/local/bin` (on the default PATH). Standard installer
  (asks for the user's password once).
- **Windows**: `windows/installer.nsi` + `windows/build-exe.sh` → `crofty-setup.exe`
  that installs per-user to `%LOCALAPPDATA%\crofty\bin` and adds it to the user
  PATH (no admin).

## The bundled Hugo

Both installers carry Hugo (extended), fetched and checksum-verified at build
time by `hugo.sh` — which also pins the version. Someone who double-clicks has no
prerequisite to install, and `crofty build` works the moment the installer
finishes. crofty runs *this* Hugo ahead of any on PATH (`internal/hugobin`): a
stray `hugo` says nothing about its version or flavor, and the frozen theme needs
the extended build.

Neither installer disturbs a hugo the author already has.

- macOS: `/usr/local/libexec/crofty/hugo` — deliberately **not** `/usr/local/bin`,
  which is shared ground (Intel Homebrew's hugo lives exactly there). It stays off
  PATH; crofty finds it from its own location.
- Windows: beside `crofty.exe`, in a directory the installer owns outright.

Hugo's macOS build ships as a `.pkg` only, so `hugo.sh` unwraps it with `pkgutil`
to get at the universal binary. Hugo is Apache-2.0; `hugo/LICENSE-hugo.txt` rides
along next to the binary in both installers.

Bump `HUGO_VERSION` in `hugo.sh` deliberately. The weekly `hugo-compat.yml` run is
what tells us a newer Hugo still builds a contract-clean site.

Both are **unsigned by choice** — no Apple Developer ID, no code-signing cert, no
P12. First open shows an OS warning (Gatekeeper / SmartScreen); the user picks
"Open Anyway" / "Run anyway". The agent guides them through it.

## Distribution

Not via wharfy channels: declaring wharfy `bundle:` flips the release into BYO
mode and drops the Go cross-build (breaks the CLI channels), and `prebuilt` is
Pro-only. So the installers are attached to the GitHub Release as **extra assets**
(same place as `install.sh`), via `gh release upload`. Download URLs:

- `https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty.pkg`
- `https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty-setup.exe`

## Release procedure

Cut a release from a macOS host with `makensis` and `gh` installed:

```sh
git tag vX.Y.Z && git push origin vX.Y.Z
export GITHUB_TOKEN=…                      # release upload
wharfy build                              # cross-compile → .wharfy/dist
wharfy release --yes                      # GitHub release: archives, deb/rpm, install.sh/ps1, latest.json
packaging/release-installers.sh X.Y.Z     # build .pkg/.exe from .wharfy/dist and gh-upload them
export FURY_TOKEN=…                        # from your secret store, for apt/rpm publish
wharfy publish --yes                      # push owned channels
```

`release-installers.sh` builds a universal `.pkg` (arm64+amd64, ad-hoc signed) and
the amd64 `.exe`, each with its Hugo, then attaches both to the release with
`--clobber`. Rare win/arm64 users fall back to the install script. The `.pkg` runs
about 47 MB and the `.exe` about 28 MB — Hugo is most of that.

## Known limitation

The NSIS uninstaller removes the files but not the PATH entry (a stale entry to a
deleted dir — harmless; Windows ignores missing PATH dirs).
