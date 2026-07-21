#!/bin/sh
# Build an UNSIGNED macOS installer (.pkg) for the crofty CLI. This is the
# double-click fallback for when an AI agent can't install crofty over the shell
# itself; a human runs it instead.
#
# The .pkg splits into an entry and a body (D-339):
#   - the entry — a link at /usr/local/bin/crofty (on the default PATH), created
#     by the postinstall as root. Written once at install; never touched again.
#   - the body — crofty and its bundled Hugo, placed in the installing user's
#     home (~/Library/Application Support/crofty), user-writable. This is the
#     part `crofty update` later replaces, with no root and no Gatekeeper prompt.
#
# So the payload here is only staged in system space (/usr/local/libexec/
# crofty-stage); the postinstall moves it into the user's home, chowns it to
# them, links the entry, and clears the staging tree. crofty runs *this* Hugo
# ahead of any on PATH, found from its own body — see internal/hugobin. Hugo
# stays beside crofty in the body, never on PATH, so it can't shadow a hugo the
# author installed themselves.
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

# Stage the body under a system dir; the postinstall relocates it into the user's
# home. Its shape (bin/crofty next to libexec/crofty/hugo) is the tree crofty
# resolves Hugo from, kept identical wherever the postinstall lands it.
STAGE="$ROOT/usr/local/libexec/crofty-stage"
mkdir -p "$STAGE/bin" "$STAGE/libexec/crofty"
cp "$BIN" "$STAGE/bin/crofty"
chmod 0755 "$STAGE/bin/crofty"
# -p keeps Hugo's own signature intact; it ships notarized and we do not re-sign it.
cp -p "$HUGO" "$STAGE/libexec/crofty/hugo"
chmod 0755 "$STAGE/libexec/crofty/hugo"
cp "$HERE/../hugo/LICENSE-hugo.txt" "$STAGE/libexec/crofty/LICENSE-hugo.txt"

# Best-effort clear of extended attributes. Note: macOS adds a system-managed
# com.apple.provenance xattr to executables that cannot be removed here; it shows
# up as a ._crofty AppleDouble entry in `pkgutil --payload-files` but is harmless
# — on install it is reconstituted as an xattr on the file, not a stray ._ file.
xattr -cr "$ROOT" 2>/dev/null || true

pkgbuild \
  --root "$ROOT" \
  --scripts "$HERE/scripts" \
  --identifier site.crofty.cli \
  --version "$VERSION" \
  --install-location / \
  "$OUT"

echo "built $OUT"
