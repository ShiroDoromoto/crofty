#!/bin/sh
# Build an UNSIGNED macOS installer (.pkg) that installs the crofty CLI binary
# into /usr/local/bin — already on the default PATH, so `crofty` works right
# after install. This is the double-click fallback for when an AI agent can't
# install crofty over the shell itself; a human runs it instead.
#
# Unsigned by choice: macOS Gatekeeper warns on first open ("unidentified
# developer") and the user picks Open Anyway. Signing/notarization is not done.
#
# Usage: build-pkg.sh <version> <binary> <out.pkg>
set -eu

VERSION="${1:?usage: build-pkg.sh <version> <binary-path> <out.pkg>}"
BIN="${2:?binary path required}"
OUT="${3:?output .pkg path required}"

ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

mkdir -p "$ROOT/usr/local/bin"
cp "$BIN" "$ROOT/usr/local/bin/crofty"
chmod 0755 "$ROOT/usr/local/bin/crofty"

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
