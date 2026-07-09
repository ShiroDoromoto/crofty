#!/bin/sh
# Build an UNSIGNED macOS installer (.pkg) that installs the crofty CLI binary
# into /usr/local/bin — already on the default PATH, so `crofty` works right
# after install. This is the double-click fallback for when an AI agent can't
# install crofty over the shell itself; a human runs it instead.
#
# It also carries Hugo, which crofty wraps, so the author needs no prerequisite.
# Hugo goes to /usr/local/libexec/crofty/ and stays OFF PATH: /usr/local/bin is
# shared with every other program, and an installer has no business replacing a
# hugo the author already put there (Intel Homebrew's lives exactly there). crofty
# finds this copy from its own location instead — see internal/hugobin.
#
# Unsigned by choice: macOS Gatekeeper warns on first open ("unidentified
# developer") and the user picks Open Anyway. Signing/notarization is not done.
#
# Usage: build-pkg.sh <version> <crofty-binary> <hugo-binary> <out.pkg>
set -eu

VERSION="${1:?usage: build-pkg.sh <version> <crofty-binary> <hugo-binary> <out.pkg>}"
BIN="${2:?crofty binary path required}"
HUGO="${3:?hugo binary path required}"
OUT="${4:?output .pkg path required}"

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

mkdir -p "$ROOT/usr/local/bin" "$ROOT/usr/local/libexec/crofty"
cp "$BIN" "$ROOT/usr/local/bin/crofty"
chmod 0755 "$ROOT/usr/local/bin/crofty"
# -p keeps Hugo's own signature intact; it ships notarized and we do not re-sign it.
cp -p "$HUGO" "$ROOT/usr/local/libexec/crofty/hugo"
chmod 0755 "$ROOT/usr/local/libexec/crofty/hugo"
cp "$HERE/../hugo/LICENSE-hugo.txt" "$ROOT/usr/local/libexec/crofty/LICENSE-hugo.txt"

# Best-effort clear of extended attributes. Note: macOS adds a system-managed
# com.apple.provenance xattr to executables that cannot be removed here; it shows
# up as a ._crofty AppleDouble entry in `pkgutil --payload-files` but is harmless
# — on install it is reconstituted as an xattr on the file, not a stray ._ file.
xattr -cr "$ROOT" 2>/dev/null || true

pkgbuild \
  --root "$ROOT" \
  --identifier site.crofty.cli \
  --version "$VERSION" \
  --install-location / \
  "$OUT"

echo "built $OUT"
