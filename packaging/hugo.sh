#!/bin/sh
# Fetch the Hugo that the click installers bundle, verify it against the release
# checksums, and print the path to the extracted binary.
#
# The installers exist for authors who have no Hugo at all, so they carry one.
# crofty then runs *this* Hugo rather than whatever is on PATH: the bundled theme
# is frozen against the extended build, and a stray `hugo` proves nothing about
# version or flavor. See internal/hugobin.
#
# Usage: hugo.sh <darwin-universal|windows-amd64> <workdir>   (prints the binary path)
set -eu

# Bump deliberately, not on a schedule. hugo-compat.yml is what tells us a newer
# Hugo still builds a contract-clean site; this is what we then ship.
HUGO_VERSION="0.164.0"

TARGET="${1:?usage: hugo.sh <darwin-universal|windows-amd64> <workdir>}"
WORK="${2:?workdir required}"

case "$TARGET" in
  darwin-universal) ASSET="hugo_extended_${HUGO_VERSION}_darwin-universal.pkg" ;;
  windows-amd64)    ASSET="hugo_extended_${HUGO_VERSION}_windows-amd64.zip" ;;
  *) echo "hugo.sh: unknown target '$TARGET'" >&2; exit 2 ;;
esac

BASE="https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}"
SUMS="hugo_${HUGO_VERSION}_checksums.txt"
DL="$WORK/dl"
mkdir -p "$DL"

curl -fsSL -o "$DL/$ASSET" "$BASE/$ASSET"
curl -fsSL -o "$DL/$SUMS" "$BASE/$SUMS"

# Trust the download only after it matches the checksum Hugo published for it.
WANT="$(awk -v a="$ASSET" '$2 == a { print $1 }' "$DL/$SUMS")"
: "${WANT:?$ASSET is absent from $SUMS — did the asset naming change?}"
GOT="$(shasum -a 256 "$DL/$ASSET" | awk '{print $1}')"
if [ "$WANT" != "$GOT" ]; then
  echo "hugo.sh: checksum mismatch for $ASSET" >&2
  echo "  expected $WANT" >&2
  echo "  got      $GOT" >&2
  exit 1
fi

OUT="$WORK/hugo-$TARGET"
mkdir -p "$OUT"

case "$TARGET" in
  darwin-universal)
    # Hugo ships macOS as a .pkg only (no tarball), so unwrap it. The payload is
    # the universal binary, alone. Requires a macOS host for pkgutil.
    rm -rf "$WORK/hugo-pkg"
    pkgutil --expand-full "$DL/$ASSET" "$WORK/hugo-pkg" >/dev/null
    cp "$WORK/hugo-pkg/Payload/hugo" "$OUT/hugo"
    chmod 0755 "$OUT/hugo"
    echo "$OUT/hugo"
    ;;
  windows-amd64)
    unzip -oq "$DL/$ASSET" hugo.exe -d "$OUT"
    echo "$OUT/hugo.exe"
    ;;
esac
