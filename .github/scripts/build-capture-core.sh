#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
case "${1:?expected an output path}" in
  /*) OUTPUT="$1" ;;
  *) OUTPUT="$(pwd)/$1" ;;
esac
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
SOURCE="$(cd "$ROOT/src/native/netproxy" && go mod download github.com/sagernet/sing-box && go list -m -f '{{.Dir}}' github.com/sagernet/sing-box)"
VERSION="$(jq -r '.singBox.currentVersion | ltrimstr("v")' "$ROOT/.github/resources.json")"
TAGS="$(jq -r '.singBox.buildTags | join(",")' "$ROOT/.github/resources.json")"
cp -R "$SOURCE/." "$WORK"
chmod -R u+w "$WORK"
patch -d "$WORK" -p1 < "$ROOT/.github/patches/sing-box-netproxy-capture.patch"
cd "$WORK"
go build -trimpath -tags "$TAGS" -ldflags "-s -w -X github.com/sagernet/sing-box/constant.Version=$VERSION $(cat release/LDFLAGS)" -o "$OUTPUT" ./cmd/sing-box
