#!/bin/sh
# Build the UNSIGNED click installers from the wharfy-built binaries and attach
# them to the GitHub release as extra assets. These are the double-click fallback
# for users whose AI agent can't install crofty over the shell; the agent hands
# them the download URL:
#   macOS:   https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty.pkg
#   Windows: https://github.com/ShiroDoromoto/crofty/releases/latest/download/crofty-setup.exe
#
# Run AFTER `wharfy release` (the release must exist) and BEFORE/around publish.
# Requires: macOS host (lipo, pkgbuild, codesign) + makensis, and gh authed.
#
# Usage: packaging/release-installers.sh <version>   (from repo root; reads .wharfy/dist)
set -eu

VERSION="${1:?usage: release-installers.sh <version>}"
DIST=".wharfy/dist"
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT

# --- macOS: one universal (arm64 + amd64) .pkg installing to /usr/local/bin ---
DARWIN_ARM="$(ls -d "$DIST"/crofty_darwin_arm64*/crofty 2>/dev/null | head -1)"
DARWIN_AMD="$(ls -d "$DIST"/crofty_darwin_amd64*/crofty 2>/dev/null | head -1)"
: "${DARWIN_ARM:?darwin arm64 binary not found in $DIST — run 'wharfy build' first}"
: "${DARWIN_AMD:?darwin amd64 binary not found in $DIST — run 'wharfy build' first}"
lipo -create "$DARWIN_ARM" "$DARWIN_AMD" -output "$OUT/crofty"
codesign --force --sign - "$OUT/crofty"   # ad-hoc (certificate-free) so it runs on Apple Silicon
"$HERE/macos/build-pkg.sh" "$VERSION" "$OUT/crofty" "$OUT/crofty.pkg"

# --- Windows: amd64 NSIS installer -> %LOCALAPPDATA%\crofty\bin (covers the vast
#     majority; rare win/arm64 users use the script or scoop) ---
WIN_AMD="$(ls -d "$DIST"/crofty_windows_amd64*/crofty.exe 2>/dev/null | head -1)"
: "${WIN_AMD:?windows amd64 binary not found in $DIST — run 'wharfy build' first}"
"$HERE/windows/build-exe.sh" "$VERSION" "$WIN_AMD" "$OUT/crofty-setup.exe"

# --- attach to the GitHub release (idempotent: --clobber replaces same-name) ---
gh release upload "v$VERSION" "$OUT/crofty.pkg" "$OUT/crofty-setup.exe" --clobber

echo "attached crofty.pkg + crofty-setup.exe to release v$VERSION"
