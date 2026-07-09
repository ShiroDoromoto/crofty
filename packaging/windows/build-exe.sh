#!/bin/sh
# Build the UNSIGNED Windows installer (.exe) from a pre-built crofty.exe and the
# hugo.exe it bundles, using NSIS (makensis). Runs on any host makensis supports,
# including macOS/Linux.
#
# Usage: build-exe.sh <version> <crofty.exe path> <hugo.exe path> <out.exe>
set -eu

VERSION="${1:?usage: build-exe.sh <version> <crofty.exe path> <hugo.exe path> <out.exe>}"
EXE="${2:?windows binary path required}"
HUGO="${3:?hugo.exe path required}"
OUT="${4:?output installer path required}"

DIR="$(cd "$(dirname "$0")" && pwd)"

# Absolute paths: makensis resolves File/OutFile relative to the .nsi otherwise.
EXE="$(cd "$(dirname "$EXE")" && pwd)/$(basename "$EXE")"
HUGO="$(cd "$(dirname "$HUGO")" && pwd)/$(basename "$HUGO")"
case "$OUT" in /*) ;; *) OUT="$(pwd)/$OUT" ;; esac

makensis -V2 \
  -DVERSION="$VERSION" \
  -DCROFTY_EXE="$EXE" \
  -DHUGO_EXE="$HUGO" \
  -DHUGO_LICENSE="$DIR/../hugo/LICENSE-hugo.txt" \
  -DOUTFILE="$OUT" \
  "$DIR/installer.nsi"

echo "built $OUT"
