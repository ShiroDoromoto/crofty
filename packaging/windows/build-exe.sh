#!/bin/sh
# Build the UNSIGNED Windows installer (.exe) from a pre-built crofty.exe using
# NSIS (makensis). Runs on any host makensis supports, including macOS/Linux.
#
# Usage: build-exe.sh <version> <crofty.exe path> <out.exe>
set -eu

VERSION="${1:?usage: build-exe.sh <version> <crofty.exe path> <out.exe>}"
EXE="${2:?windows binary path required}"
OUT="${3:?output installer path required}"

DIR="$(cd "$(dirname "$0")" && pwd)"

# Absolute paths: makensis resolves File/OutFile relative to the .nsi otherwise.
EXE="$(cd "$(dirname "$EXE")" && pwd)/$(basename "$EXE")"
case "$OUT" in /*) ;; *) OUT="$(pwd)/$OUT" ;; esac

makensis -V2 \
  -DVERSION="$VERSION" \
  -DCROFTY_EXE="$EXE" \
  -DOUTFILE="$OUT" \
  "$DIR/installer.nsi"

echo "built $OUT"
