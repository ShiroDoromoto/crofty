#!/bin/sh
# Build the UNSIGNED click installers from the wharfy-built binaries and attach
# them to the GitHub release as extra assets. These are the double-click fallback
# for users whose AI agent can't install crofty over the shell; the agent hands
# them the download URL:
#   macOS:   https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty.pkg
#   Windows: https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty-setup.exe
#
# Both carry Hugo, so a person who double-clicks needs no prerequisite. The Hugo
# is downloaded and checksum-verified by hugo.sh at build time.
#
# Run AFTER `wharfy release` (the release must exist) and BEFORE/around publish.
# Requires: macOS host (lipo, pkgbuild, codesign, pkgutil) + makensis, and gh authed.
#
# Usage: packaging/release-installers.sh <version> [outdir]   (from repo root; reads .wharfy/dist)
#
# With an outdir the installers are left there for the caller to use — CI attests
# their build provenance, which needs the files it just built, not a re-download.
# Without one they are built in a temp dir and thrown away after the upload.
set -eu

VERSION="${1:?usage: release-installers.sh <version> [outdir]}"
DIST=".wharfy/dist"
HERE="$(cd "$(dirname "$0")" && pwd)"
if [ "${2-}" = "" ]; then
  OUT="$(mktemp -d)"
  trap 'rm -rf "$OUT"' EXIT
else
  OUT="$2"
  mkdir -p "$OUT"
fi

# --- macOS: one universal (arm64 + amd64) .pkg; entry on /usr/local/bin, body
#     in the user's home (D-339) ---
DARWIN_ARM="$(ls -d "$DIST"/crofty_darwin_arm64*/crofty 2>/dev/null | head -1)"
DARWIN_AMD="$(ls -d "$DIST"/crofty_darwin_amd64*/crofty 2>/dev/null | head -1)"
: "${DARWIN_ARM:?darwin arm64 binary not found in $DIST — run 'wharfy build' first}"
: "${DARWIN_AMD:?darwin amd64 binary not found in $DIST — run 'wharfy build' first}"
lipo -create "$DARWIN_ARM" "$DARWIN_AMD" -output "$OUT/crofty"
codesign --force --sign - "$OUT/crofty"   # ad-hoc (certificate-free) so it runs on Apple Silicon
HUGO_MAC="$("$HERE/hugo.sh" darwin-universal "$OUT")"
"$HERE/macos/build-pkg.sh" "$VERSION" "$OUT/crofty" "$HUGO_MAC" "$OUT/crofty.pkg"

# --- Windows: amd64 NSIS installer -> %LOCALAPPDATA%\crofty\bin (covers the vast
#     majority; rare win/arm64 users use the script) ---
WIN_AMD="$(ls -d "$DIST"/crofty_windows_amd64*/crofty.exe 2>/dev/null | head -1)"
: "${WIN_AMD:?windows amd64 binary not found in $DIST — run 'wharfy build' first}"
HUGO_WIN="$("$HERE/hugo.sh" windows-amd64 "$OUT")"
"$HERE/windows/build-exe.sh" "$VERSION" "$WIN_AMD" "$HUGO_WIN" "$OUT/crofty-setup.exe"

# --- update payloads (D-339): the body `crofty update` self-fetches to replace
#     itself, identical to what the installers drop. macOS is a tree (crofty under
#     bin/, Hugo under libexec/crofty/ — the same shape build-pkg.sh stages and
#     crofty resolves Hugo from); Windows is flat (crofty.exe beside hugo.exe, as
#     installer.nsi lays them). The install-script route carries no Hugo, so it
#     reuses wharfy's own binary archives — no extra payload is built here. ---
MACBODY="$OUT/body-darwin"
mkdir -p "$MACBODY/bin" "$MACBODY/libexec/crofty"
cp "$OUT/crofty" "$MACBODY/bin/crofty"
cp -p "$HUGO_MAC" "$MACBODY/libexec/crofty/hugo"
cp "$HERE/hugo/LICENSE-hugo.txt" "$MACBODY/libexec/crofty/LICENSE-hugo.txt"
tar -C "$MACBODY" -czf "$OUT/crofty-body-darwin-universal.tar.gz" bin libexec

WINBODY="$OUT/body-windows"
mkdir -p "$WINBODY"
cp "$WIN_AMD" "$WINBODY/crofty.exe"
cp "$HUGO_WIN" "$WINBODY/hugo.exe"
cp "$HERE/hugo/LICENSE-hugo.txt" "$WINBODY/LICENSE-hugo.txt"
# zip resolves its output relative to the cwd, so a cd into $WINBODY would send
# a relative $OUT astray (unlike tar -C above, which never leaves this dir). Stay
# put and flatten with -j, which drops the files at the archive root the way the
# installer lays them. rm first so a re-run rebuilds rather than appends (as tar
# does), keeping the bytes update verifies against reproducible.
rm -f "$OUT/crofty-body-windows-amd64.zip"
zip -qj "$OUT/crofty-body-windows-amd64.zip" \
  "$WINBODY/crofty.exe" "$WINBODY/hugo.exe" "$WINBODY/LICENSE-hugo.txt"

# Checksums, paired with the verification crofty update does after downloading
# (sha256, in `shasum -a 256` format so update reads the hash by asset name).
( cd "$OUT" && shasum -a 256 \
    crofty-body-darwin-universal.tar.gz \
    crofty-body-windows-amd64.zip \
    > crofty-body-checksums.txt )

# --- attach to the GitHub release (idempotent: --clobber replaces same-name) ---
gh release upload "v$VERSION" \
  "$OUT/crofty.pkg" "$OUT/crofty-setup.exe" \
  "$OUT/crofty-body-darwin-universal.tar.gz" \
  "$OUT/crofty-body-windows-amd64.zip" \
  "$OUT/crofty-body-checksums.txt" \
  --clobber

echo "attached installers + update payloads to release v$VERSION"
